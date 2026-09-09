package github_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/importer/github"
)

// transcript is the recorded API M7-S5 asks these tests to read: a
// canned answer per path, and a count of every round trip, which is what makes
// "no request per issue" something this suite asserts rather than hopes.
type transcript struct {
	answers map[string]*github.Response
	miss    *github.Response
	err     error
	paths   []string
	tokens  []importer.Secret
}

func (t *transcript) Get(
	_ context.Context, url string, token importer.Secret,
) (*github.Response, error) {
	path := url
	if _, rest, found := strings.Cut(url, "https://api.invalid"); found {
		path = rest
	}

	t.paths = append(t.paths, path)
	t.tokens = append(t.tokens, token)

	if t.err != nil {
		return nil, t.err
	}

	if answer, ok := t.answers[path]; ok {
		return answer, nil
	}

	if t.miss != nil {
		return t.miss, nil
	}

	return &github.Response{Status: http.StatusOK, Body: []byte("[]")}, nil
}

func (t *transcript) requests() int { return len(t.paths) }

func ok(body string) *github.Response {
	return &github.Response{Status: http.StatusOK, Body: []byte(body)}
}

// client is one wired to a transcript, with a clock and a wait that record
// rather than pass time.
func client(t *transcript, waited *[]time.Duration) *github.Client {
	return &github.Client{
		Base:      "https://api.invalid",
		Transport: t,
		Now:       func() time.Time { return time.Unix(1_000_000, 0) },
		Sleep: func(_ context.Context, d time.Duration) error {
			*waited = append(*waited, d)

			return nil
		},
	}
}

// listOf renders n issues as the REST list would, from the given number on.
func listOf(from, n int) string {
	rows := make([]string, 0, n)
	for i := range n {
		rows = append(rows, fmt.Sprintf(
			`{"number":%d,"title":"issue %d","state":"open","created_at":"2026-03-04T09:00:00Z",`+
				`"assignees":[{"login":"dmitry"}],"comments":0}`, from+i, from+i))
	}

	return "[" + strings.Join(rows, ",") + "]"
}

// M7-S4: "5,000 issues are read in pages of a hundred with no request
// per issue" — a process count in the spirit of M2-S5, against a transport that
// counts.
func TestTheListIsReadInPagesAndCostsNoRequestPerIssue(t *testing.T) {
	t.Parallel()

	const issues = 250

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 100)),
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=2": ok(listOf(101, 100)),
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=3": ok(listOf(201, 50)),
	}}

	var waited []time.Duration

	c := client(tape, &waited)

	got, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.NoError(t, err)
	require.Len(t, got, issues)

	require.Equal(t, 4, tape.requests(),
		"three pages of issues and one of comments: nothing the list already "+
			"carried is asked for again, per issue or otherwise")
	require.Equal(t, 4, c.Requests())
	require.Zero(t, c.Waited())

	require.Equal(t, "/repos/acme/widgets/issues/comments?per_page=100&page=1", tape.paths[3],
		"comments are listed for the repository, which is what keeps them pages "+
			"rather than issues")
}

// The one per-issue request there is, and it is one because there is no
// repository-wide list of field values.
func TestFieldValuesAreTheOnlyPerIssueRequest(t *testing.T) {
	t.Parallel()

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 3)),
		"/repos/acme/widgets/issues/1/issue-field-values":                                     ok(`[{"name":"Severity","value":"high"}]`),
	}}

	var waited []time.Duration

	got, err := client(tape, &waited).Issues(t.Context(), "acme/widgets", github.StateAll, true)
	require.NoError(t, err)
	require.Len(t, got, 3)

	require.Equal(t, 5, tape.requests(), "one page, one comments page, three issues")
	require.Contains(t, tape.paths, "/repos/acme/widgets/issues/3/issue-field-values")
}

func TestCommentsAreFiledUnderTheIssueTheyBelongTo(t *testing.T) {
	t.Parallel()

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 2)),
		"/repos/acme/widgets/issues/comments?per_page=100&page=1": ok(`[
			{"issue_url":"https://api.github.com/repos/acme/widgets/issues/1","user":{"login":"alice"},"created_at":"2026-03-05T10:00:00Z","body":"first"},
			{"issue_url":"https://api.github.com/repos/acme/widgets/issues/1","user":{"login":"alice"},"created_at":"2026-03-05T11:00:00Z","body":"second"},
			{"issue_url":"https://api.github.com/repos/acme/widgets/issues/9","body":"an issue that is not here"},
			{"issue_url":"nonsense","body":"not an issue url at all"}
		]`),
	}}

	var waited []time.Duration

	body, err := client(tape, &waited).Fetch(t.Context(), "acme/widgets", github.StateAll, false)
	require.NoError(t, err)

	source, err := github.New(github.Options{Dump: body})
	require.NoError(t, err)

	batch, err := source.Load(t.Context())
	require.NoError(t, err)

	require.Len(t, batch.Items, 2)
	require.Len(t, item(t, batch, "#1").Comments, 2)
	require.Equal(t, "second", item(t, batch, "#1").Comments[1].Body)
	require.Empty(t, item(t, batch, "#2").Comments)
}

func TestARateLimitIsWaitedOutRatherThanFailedOn(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name   string
		status int
		header http.Header
		want   time.Duration
	}{
		{"retry-after", http.StatusForbidden, http.Header{"Retry-After": {"30"}}, 30 * time.Second},
		{"the reset", http.StatusTooManyRequests, http.Header{"X-Ratelimit-Reset": {"1000090"}}, 90 * time.Second},
		// A primary rate limit spends the budget and says so, which is what
		// tells it from a 403 that means "you may not read this".
		{"a reset already past", http.StatusForbidden, http.Header{
			"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"999000"},
		}, 0},
		{"an hour away", http.StatusForbidden, http.Header{
			"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {"1003600"},
		}, github.MaxWait},
		{"nothing said", http.StatusTooManyRequests, nil, time.Second},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			answered := false
			tape := &transcript{}
			tape.answers = map[string]*github.Response{}
			tape.miss = &github.Response{Status: tt.status, Header: tt.header}

			var waited []time.Duration

			c := client(tape, &waited)
			c.Sleep = func(context.Context, time.Duration) error {
				answered = true
				tape.miss = ok("[]")

				return nil
			}

			_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
			require.NoError(t, err)
			require.True(t, answered)

			c2 := client(&transcript{miss: &github.Response{Status: tt.status, Header: tt.header}}, &waited)
			_, _ = c2.Issues(t.Context(), "acme/widgets", github.StateAll, false)
			require.Equal(t, tt.want, waited[0])
		})
	}
}

func TestARateLimitThatNeverLiftsIsReported(t *testing.T) {
	t.Parallel()

	tape := &transcript{miss: &github.Response{
		Status: http.StatusForbidden,
		Body:   []byte("API rate limit exceeded"),
	}}

	var waited []time.Duration

	c := client(tape, &waited)

	_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "403")
	require.Contains(t, err.Error(), "API rate limit exceeded")

	require.Equal(t, github.MaxAttempts, tape.requests())
	require.Len(t, waited, github.MaxAttempts-1)
	require.Positive(t, c.Waited())
}

func TestAResponseThatIsNotARateLimitIsNotRetried(t *testing.T) {
	t.Parallel()

	tape := &transcript{miss: &github.Response{
		Status: http.StatusNotFound,
		Body:   []byte(strings.Repeat("x", 400)),
	}}

	var waited []time.Duration

	_, err := client(tape, &waited).Issues(t.Context(), "acme/nothing", github.StateAll, false)
	require.Error(t, err)
	require.Contains(t, err.Error(), "404")
	require.Contains(t, err.Error(), "…", "a failing body is summarised, not reprinted")
	require.Equal(t, 1, tape.requests())
}

func TestATokenIsSentAndNeverPrinted(t *testing.T) {
	t.Parallel()

	const value = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"

	tape := &transcript{err: fmt.Errorf(
		"Get \"https://api.invalid/x?access_token=%s\": connection refused", value)}

	var waited []time.Duration

	c := client(tape, &waited)
	c.Token = importer.Secret(value)

	_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
	require.NotContains(t, err.Error(), value)
	require.Contains(t, err.Error(), importer.Redacted)

	require.Equal(t, importer.Secret(value), tape.tokens[0],
		"the transport is given the token; nothing else ever sees it")
}

func TestABodyThatIsNotWhatItShouldBeIsReported(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		tape *transcript
		want string
	}{
		{
			"the list", &transcript{miss: ok("{}")},
			"reading /repos/acme/widgets/issues",
		},
		{
			"a comment", &transcript{answers: map[string]*github.Response{
				"/repos/acme/widgets/issues/comments?per_page=100&page=1": ok("[3]"),
			}},
			"reading a comment",
		},
		{
			"the field values", &transcript{answers: map[string]*github.Response{
				"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 1)),
				"/repos/acme/widgets/issues/1/issue-field-values":                                     ok("{}"),
			}},
			"the field values of #1",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var waited []time.Duration

			_, err := client(tt.tape, &waited).Issues(
				t.Context(), "acme/widgets", github.StateAll, true)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestFetchWritesADumpThatReadsBack(t *testing.T) {
	t.Parallel()

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=open&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 2)),
	}}

	var waited []time.Duration

	body, err := client(tape, &waited).Fetch(t.Context(), "acme/widgets", github.StateOpen, false)
	require.NoError(t, err)
	require.Contains(t, string(body), `"repository": "acme/widgets"`)

	source, err := github.New(github.Options{Dump: body})
	require.NoError(t, err)

	batch, err := source.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"#1", "#2"}, keys(batch))
}

func TestFetchReportsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	var waited []time.Duration

	_, err := client(&transcript{miss: &github.Response{Status: http.StatusNotFound}}, &waited).
		Fetch(t.Context(), "acme/nothing", github.StateAll, false)
	require.Error(t, err)
}

func TestTheSourceReadsTheAPIWhenThereIsNoDump(t *testing.T) {
	t.Parallel()

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(listOf(1, 2)),
	}}

	var waited []time.Duration

	source, err := github.New(github.Options{Repository: "acme/widgets", Client: client(tape, &waited)})
	require.NoError(t, err)

	batch, err := source.Load(t.Context())
	require.NoError(t, err)
	require.Equal(t, []string{"#1", "#2"}, keys(batch))
	require.Equal(t, 2, batch.Requests, "the dry run reports what the budget cost")
}

func TestTheSourceReportsWhatTheAPIWouldNotAnswer(t *testing.T) {
	t.Parallel()

	var waited []time.Duration

	source, err := github.New(github.Options{
		Repository: "acme/nothing",
		Client:     client(&transcript{miss: &github.Response{Status: http.StatusNotFound}}, &waited),
	})
	require.NoError(t, err)

	_, err = source.Load(t.Context())
	require.Error(t, err)
}

// The real transport, against a server this test runs. Nothing here reaches
// github.com.
func TestTheRealTransportSendsWhatGitHubExpects(t *testing.T) {
	t.Parallel()

	var got *http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r

		w.Header().Set("X-Ratelimit-Remaining", "4999")
		_, _ = w.Write([]byte(listOf(1, 1)))
	}))
	defer server.Close()

	c := &github.Client{Base: server.URL, Token: importer.Secret("ghp_secret")}

	issues, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.NoError(t, err)
	require.Len(t, issues, 1)

	require.Equal(t, "application/vnd.github+json", got.Header.Get("Accept"))
	require.Equal(t, "2022-11-28", got.Header.Get("X-GitHub-Api-Version"))
	require.Equal(t, "Bearer ghp_secret", got.Header.Get("Authorization"),
		"the token goes in a header and nowhere else: a URL is logged by every "+
			"proxy between here and GitHub")
	require.NotContains(t, got.URL.String(), "ghp_secret")
}

func TestTheRealTransportNeedsNoToken(t *testing.T) {
	t.Parallel()

	var got *http.Request

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	_, err := (&github.Client{Base: server.URL}).
		Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.NoError(t, err)
	require.Empty(t, got.Header.Get("Authorization"),
		"a public repository answers without one, which is what lets these "+
			"stories be tested against real repositories")
}

func TestTheRealTransportReportsARequestItCannotEvenBuild(t *testing.T) {
	t.Parallel()

	_, err := github.HTTPTransport{}.Get(t.Context(), "http://\x7f/", "")
	require.Error(t, err)

	_, err = (&github.Client{Base: "http://\x7f"}).
		Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
}

func TestTheDefaultBaseIsGitHub(t *testing.T) {
	t.Parallel()

	tape := &transcript{err: errors.New("no")}

	c := &github.Client{Transport: tape}

	_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
	require.Equal(t, "/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1",
		strings.TrimPrefix(tape.paths[0], github.DefaultBase))
}

// The wait is a real one when nothing injected a fake, and it is cancellable —
// an import somebody interrupted should stop rather than sit out an hour.
func TestARealWaitEndsWhenTheContextDoes(t *testing.T) {
	t.Parallel()

	tape := &transcript{miss: &github.Response{
		Status: http.StatusForbidden,
		Header: http.Header{
			"X-Ratelimit-Remaining": {"0"},
			"X-Ratelimit-Reset":     {strconv.FormatInt(time.Now().Add(time.Hour).Unix(), 10)},
		},
	}}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := (&github.Client{Base: "https://api.invalid", Transport: tape}).
		Issues(ctx, "acme/widgets", github.StateAll, false)
	require.ErrorIs(t, err, context.Canceled)
}

func TestARealWaitReturnsWhenTheTimerFires(t *testing.T) {
	t.Parallel()

	attempts := 0
	tape := &transcript{}
	tape.miss = &github.Response{
		Status: http.StatusTooManyRequests,
		Header: http.Header{"Retry-After": {"0"}},
	}

	c := &github.Client{Base: "https://api.invalid", Transport: &counting{tape, &attempts}}

	_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
	require.Equal(t, github.MaxAttempts, attempts)
}

// counting is a transport that answers through another and counts.
type counting struct {
	inner *transcript
	n     *int
}

func (c *counting) Get(
	ctx context.Context, url string, token importer.Secret,
) (*github.Response, error) {
	*c.n++

	return c.inner.Get(ctx, url, token)
}

func TestWhatTheCommentsAndFieldPagesCouldNotAnswer(t *testing.T) {
	t.Parallel()

	list := ok(listOf(1, 1))

	for _, tt := range []struct {
		name string
		tape *transcript
	}{
		{"the comments", &transcript{
			answers: map[string]*github.Response{
				"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": list,
			},
			miss: &github.Response{Status: http.StatusNotFound},
		}},
		{"the field values", &transcript{
			answers: map[string]*github.Response{
				"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": list,
				"/repos/acme/widgets/issues/comments?per_page=100&page=1":                             ok("[]"),
			},
			miss: &github.Response{Status: http.StatusNotFound},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var waited []time.Duration

			_, err := client(tt.tape, &waited).Issues(
				t.Context(), "acme/widgets", github.StateAll, true)
			require.Error(t, err)
			require.Contains(t, err.Error(), "404")
		})
	}
}

func TestAPullRequestIsNotAskedForItsFieldValues(t *testing.T) {
	t.Parallel()

	tape := &transcript{answers: map[string]*github.Response{
		"/repos/acme/widgets/issues?state=all&sort=created&direction=asc&per_page=100&page=1": ok(
			`[{"number":1,"title":"an issue","state":"open","created_at":"2026-03-04T09:00:00Z"},
			  {"number":2,"title":"a pull request","state":"open","created_at":"2026-03-04T09:00:00Z",
			   "pull_request":{"url":"https://api.github.com/repos/acme/widgets/pulls/2"}}]`),
	}}

	var waited []time.Duration

	_, err := client(tape, &waited).Issues(t.Context(), "acme/widgets", github.StateAll, true)
	require.NoError(t, err)

	require.NotContains(t, tape.paths, "/repos/acme/widgets/issues/2/issue-field-values",
		"a pull request is not an issue, and it has no field values to ask for")
	require.Contains(t, tape.paths, "/repos/acme/widgets/issues/1/issue-field-values")
}

func TestTheRealClockIsWhatARateLimitResetIsMeasuredAgainst(t *testing.T) {
	t.Parallel()

	// No Now, so the reset is measured against the real one — a reset one
	// second from now is a wait of about a second, which is what this asserts
	// without spending it.
	reset := time.Now().Add(time.Second).Unix()

	tape := &transcript{miss: &github.Response{
		Status: http.StatusTooManyRequests,
		Header: http.Header{"X-Ratelimit-Reset": {strconv.FormatInt(reset, 10)}},
	}}

	var waits []time.Duration

	c := &github.Client{
		Base:      "https://api.invalid",
		Transport: tape,
		Sleep: func(_ context.Context, d time.Duration) error {
			waits = append(waits, d)
			tape.miss = ok("[]")

			return nil
		},
	}

	_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.NoError(t, err)
	require.Len(t, waits, 1)
	require.Positive(t, waits[0])
	require.LessOrEqual(t, waits[0], time.Second)
}

func TestATransportThatCannotReachAnythingSaysSo(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	base := server.URL

	server.Close()

	_, err := (&github.Client{Base: base}).
		Issues(t.Context(), "acme/widgets", github.StateAll, false)
	require.Error(t, err)
}

// GitHub answers 403 both for a secondary rate limit and for "you may not read
// this", and the two want opposite handling. Waiting out the second is five
// requests and eleven seconds of backoff before a message that was already
// correct on the first one — measured against a real repository, which is how
// this was found.
func TestAPermissionRefusalIsNotARateLimit(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		res  *github.Response
	}{
		{"nothing said at all", &github.Response{Status: http.StatusForbidden}},
		{"a body about permission", &github.Response{
			Status: http.StatusForbidden,
			Body:   []byte(`{"message":"GitHub access to this repository is not enabled"}`),
		}},
		{"budget left over", &github.Response{
			Status: http.StatusForbidden,
			Header: http.Header{"X-Ratelimit-Remaining": {"4999"}},
			Body:   []byte(`{"message":"Resource not accessible by personal access token"}`),
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tape := &transcript{miss: tt.res}

			var waited []time.Duration

			c := client(tape, &waited)

			_, err := c.Issues(t.Context(), "acme/widgets", github.StateAll, false)
			require.Error(t, err)
			require.Contains(t, err.Error(), "403")

			require.Equal(t, 1, tape.requests(),
				"a refusal that will never improve is reported on the first request")
			require.Empty(t, waited)
			require.Zero(t, c.Waited())
		})
	}
}
