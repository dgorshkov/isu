// The content half of a GitHub import: the comments, the field values, the
// attachment links, and the pull request that closed each issue.
//
// It is its own file because it is its own story — PLAN.md M7-S5 — and because
// the two halves fail differently. Everything beside it maps an issue onto
// isu's schema and cannot lose anything; this is the part that would quietly
// drop a comment, mangle an attachment link, or claim a link to a commit it
// cannot show you.

package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/dgorshkov/isu/internal/importer"
)

// attachComments reads every comment in the repository and files each under the
// issue it belongs to.
func (c *Client) attachComments(ctx context.Context, repository string, issues []Issue) error {
	raw, err := pages[json.RawMessage](ctx, c, "/repos/"+repository+"/issues/comments")
	if err != nil {
		return err
	}

	byIssue := map[int][][]byte{}

	for _, one := range raw {
		var where struct {
			IssueURL string `json:"issue_url"`
		}

		if err := json.Unmarshal(one, &where); err != nil {
			return fmt.Errorf("reading a comment: %w", err)
		}

		if number, ok := issueNumber(where.IssueURL); ok {
			byIssue[number] = append(byIssue[number], one)
		}
	}

	for i := range issues {
		if list, ok := byIssue[issues[i].Number]; ok {
			// Assembled rather than re-marshalled: every element came out of a
			// decoded array, so joining them is the whole of it and there is no
			// failure here to invent an error path for.
			issues[i].Comments = append(append([]byte{'['},
				bytes.Join(list, []byte(","))...), ']')
		}
	}

	return nil
}

// attachFields reads each issue's field values, which is the one per-issue
// request this importer makes and the only thing no repository-wide list
// carries.
func (c *Client) attachFields(ctx context.Context, repository string, issues []Issue) error {
	for i := range issues {
		if issues[i].isPullRequest() {
			continue
		}

		body, err := c.get(ctx, fmt.Sprintf("/repos/%s/issues/%d/issue-field-values",
			repository, issues[i].Number))
		if err != nil {
			return err
		}

		var values []FieldValue
		if err := json.Unmarshal(body, &values); err != nil {
			return fmt.Errorf("reading the field values of #%d: %w", issues[i].Number, err)
		}

		issues[i].Fields = values
	}

	return nil
}

func fieldValues(values []FieldValue) map[string]any {
	out := map[string]any{}
	for _, v := range values {
		out[v.name()] = v.value()
	}

	return out
}

// comments maps GitHub's comments onto the importer's.
func comments(in Issue) []importer.Comment {
	out := make([]importer.Comment, 0, len(in.comments))

	for _, c := range in.comments {
		author := ""
		if c.User != nil {
			author = c.User.Login
		}

		out = append(out, importer.Comment{
			Author: author,
			When:   c.CreatedAt,
			Body:   c.Body,
		})
	}

	return out
}

// attachmentHost is where GitHub puts a file somebody dragged into an issue.
const attachmentHost = "https://github.com/user-attachments/assets/"

// attachments finds the attachment links in a body.
//
// They are recorded and never rewritten, and that is a finding rather than a
// preference: on a private repository the asset cannot be fetched with a
// personal access token or a GitHub App token at all — it wants a browser
// session — so an importer whose completeness depends on winning that race is
// one that half-works on exactly the repositories people most want migrated.
func attachments(body string) []string {
	var out []string

	rest := body

	for {
		at := strings.Index(rest, attachmentHost)
		if at < 0 {
			return out
		}

		rest = rest[at:]

		end := strings.IndexFunc(rest, func(r rune) bool {
			return r == ')' || r == ']' || r == '"' || r == '\'' || r == ' ' ||
				r == '\n' || r == '\r' || r == '\t' || r == '<' || r == '>'
		})
		if end < 0 {
			end = len(rest)
		}

		out = appendOnce(out, rest[:end])
		rest = rest[end:]
	}
}

// closing is the pull request or commit GitHub itself says closed the issue.
//
// It is the strongest evidence tier there is and here it is free: GitHub
// already stores it, and it arrives with the issue rather than through an
// integration somebody had to install.
func closing(in Issue) string {
	for _, ref := range in.ClosedBy {
		if ref.URL != "" {
			return ref.URL
		}

		if ref.Number > 0 {
			return "#" + strconv.Itoa(ref.Number)
		}
	}

	return in.ClosedByCommit
}
