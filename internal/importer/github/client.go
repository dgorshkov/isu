package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/dgorshkov/isu/internal/importer"
)

// Rate limits are handled, not hoped for.
//
// Five thousand points an hour, a hundred concurrent requests, and a cap per
// minute besides. A 5,000-issue import fits comfortably inside that and only if
// it batches, which is why every list below is read a hundred rows at a time
// and why **nothing here makes a request per issue for anything the list
// already carried**. Comments are the case that looks like it needs one and
// does not: `/repos/{owner}/{repo}/issues/comments` lists every comment in the
// repository, so they cost pages rather than issues.
//
// Issue field values are the one exception, and they are one because there is
// no repository-wide list of them. `--fields=false` turns them off, and the dry
// run reports what the budget cost either way.
const (
	// PerPage is the largest page GitHub serves.
	PerPage = 100
	// MaxAttempts is how many times one request waits out a rate limit before
	// the import gives up and says so.
	MaxAttempts = 5
	// MaxWait is the longest this will sit on a single rate limit. A reset an
	// hour away is a reason to stop and tell somebody, not to hold a terminal
	// open.
	MaxWait = 10 * time.Minute
	// DefaultBase is the API root.
	DefaultBase = "https://api.github.com"
)

// Response is one round trip's answer.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Transport is one HTTP round trip.
//
// It is an interface so that the tests read a recorded transcript rather than
// the live API — an importer tested against a service that changes underneath
// it is one whose test suite fails for reasons nobody controls — and so that
// the process-count assertion PLAN.md M7-S4 asks for has something to count.
type Transport interface {
	Get(ctx context.Context, url string, token importer.Secret) (*Response, error)
}

// Client reads the GitHub API.
type Client struct {
	// Base is the API root. Empty means DefaultBase.
	Base string
	// Token is the credential, read from the environment and never from a
	// flag. It may be empty: a public repository answers without one, which is
	// what lets these stories be tested against real repositories.
	Token importer.Secret
	// Transport is how a request is made. Nil means net/http.
	Transport Transport
	// Sleep is how the client waits out a rate limit. Nil means a real wait.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now is the clock a rate-limit reset is measured against. Nil means the
	// real one.
	Now func() time.Time

	requests int
	waited   time.Duration
}

// Requests is how many round trips this client has made, which is what the dry
// run reports as the budget cost.
func (c *Client) Requests() int { return c.requests }

// Waited is how long it spent sitting on rate limits.
func (c *Client) Waited() time.Duration { return c.waited }

// Issues reads every issue in a repository, with its comments and, when asked,
// its field values.
func (c *Client) Issues(
	ctx context.Context, repository, state string, fields bool,
) ([]Issue, error) {
	issues, err := pages[Issue](ctx, c,
		fmt.Sprintf("/repos/%s/issues?state=%s&sort=created&direction=asc", repository, state))
	if err != nil {
		return nil, err
	}

	if err := c.attachComments(ctx, repository, issues); err != nil {
		return nil, err
	}

	if !fields {
		return issues, nil
	}

	return issues, c.attachFields(ctx, repository, issues)
}

// Dump is what --fetch-only writes: the repository's name and the list as it
// was read, so that an import is reproducible without a network and reviewable
// as a diff.
type Dump struct {
	Repository string  `json:"repository"`
	Issues     []Issue `json:"issues"`
}

// Fetch reads the repository and returns the dump bytes.
func (c *Client) Fetch(
	ctx context.Context, repository, state string, fields bool,
) ([]byte, error) {
	issues, err := c.Issues(ctx, repository, state, fields)
	if err != nil {
		return nil, err
	}

	return encodeDump(repository, issues)
}

// encodeDump renders the dump. It can fail — a raw message that is not JSON
// would make it — so it says so rather than handing back half a file.
func encodeDump(repository string, issues []Issue) ([]byte, error) {
	out, err := json.MarshalIndent(Dump{Repository: repository, Issues: issues}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("writing the dump: %w", err)
	}

	return append(out, '\n'), nil
}

// issueNumber reads the issue number out of an API URL.
func issueNumber(url string) (int, bool) {
	_, rest, ok := strings.Cut(url, "/issues/")
	if !ok {
		return 0, false
	}

	number, err := strconv.Atoi(rest)

	return number, err == nil
}

// pages reads a paginated list a hundred rows at a time, stopping at the first
// short page — which is what makes 5,000 issues fifty requests and not five
// thousand.
func pages[T any](ctx context.Context, c *Client, path string) ([]T, error) {
	var out []T

	join := "?"
	if strings.Contains(path, "?") {
		join = "&"
	}

	for page := 1; ; page++ {
		body, err := c.get(ctx, fmt.Sprintf("%s%sper_page=%d&page=%d", path, join, PerPage, page))
		if err != nil {
			return nil, err
		}

		var batch []T
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}

		out = append(out, batch...)

		if len(batch) < PerPage {
			return out, nil
		}
	}
}

// get makes one request, waiting out a rate limit rather than failing on one.
func (c *Client) get(ctx context.Context, path string) ([]byte, error) {
	url := c.base() + path

	for attempt := 1; ; attempt++ {
		c.requests++

		res, err := c.transport().Get(ctx, url, c.Token)
		if err != nil {
			return nil, importer.Redact(fmt.Errorf("GET %s: %w", path, err), c.Token)
		}

		if res.Status == http.StatusOK {
			return res.Body, nil
		}

		if !throttled(res.Status) || attempt == MaxAttempts {
			return nil, importer.Redact(fmt.Errorf("GET %s: %d %s: %s",
				path, res.Status, http.StatusText(res.Status), summary(res.Body)), c.Token)
		}

		wait := c.retryAfter(res.Header, attempt)
		c.waited += wait

		if err := c.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// throttled reports whether a status is GitHub saying "not so fast". Both
// answers mean it: 429 is the documented one and 403 is what a secondary rate
// limit still returns.
func throttled(status int) bool {
	return status == http.StatusTooManyRequests || status == http.StatusForbidden
}

// retryAfter is how long to wait: what the response said, and a widening
// backoff when it said nothing.
func (c *Client) retryAfter(h http.Header, attempt int) time.Duration {
	wait := time.Duration(attempt) * time.Second

	if seconds, err := strconv.Atoi(h.Get("retry-after")); err == nil {
		wait = time.Duration(seconds) * time.Second
	} else if reset, err := strconv.ParseInt(h.Get("x-ratelimit-reset"), 10, 64); err == nil {
		wait = time.Unix(reset, 0).Sub(c.now())
	}

	switch {
	case wait < 0:
		return 0
	case wait > MaxWait:
		return MaxWait
	default:
		return wait
	}
}

// summary is as much of a failing response as belongs in an error message.
func summary(body []byte) string {
	const most = 200

	text := strings.TrimSpace(string(body))
	if len(text) > most {
		return text[:most] + "…"
	}

	return text
}

func (c *Client) base() string {
	if c.Base == "" {
		return DefaultBase
	}

	return c.Base
}

func (c *Client) now() time.Time {
	if c.Now == nil {
		return time.Now()
	}

	return c.Now()
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}

	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) transport() Transport {
	if c.Transport == nil {
		return HTTPTransport{}
	}

	return c.Transport
}

// HTTPTransport is the real one.
type HTTPTransport struct {
	// Client is the http.Client to use. Nil means http.DefaultClient.
	Client *http.Client
}

// MaxResponseBytes is the most of one response this will read. A response
// larger than this is not a page of a hundred issues.
const MaxResponseBytes = 32 << 20

// Get makes one request.
func (t HTTPTransport) Get(
	ctx context.Context, url string, token importer.Secret,
) (*Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	// The token goes in a header and nowhere else: a URL is logged by every
	// proxy between here and GitHub, and a token in one is a token on a disk
	// somebody else owns.
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token.Reveal())
	}

	client := t.Client
	if client == nil {
		client = http.DefaultClient
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	// A truncated body is a failing response we describe imperfectly, which is
	// better than an import that stops because a connection was cut while it
	// was reading an error message.
	body, _ := io.ReadAll(io.LimitReader(res.Body, MaxResponseBytes))

	return &Response{Status: res.StatusCode, Header: res.Header, Body: body}, nil
}
