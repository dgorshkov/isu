package gittest

import (
	"fmt"
	"strings"
	"time"
)

// issuesDir is where issues live in every repository isu understands.
const issuesDir = "issues"

// IssueOption changes what Issue writes. The zero set of options produces a
// valid open chore, which is the smallest issue the schema in PLAN.md accepts:
// every other type carries a required field of its own, and an option that
// silently produced an invalid fixture would make every validation test in M1
// pass for the wrong reason.
type IssueOption func(*issueSpec)

// issueSpec is the frontmatter of one issue, in the order it will be written,
// plus its body and the other files in its folder.
type issueSpec struct {
	keys   []string
	values map[string]string
	body   string
	files  []issueFile
}

type issueFile struct {
	path    string
	content string
}

// set writes a key, keeping its position if it is already there. Order is
// stable so that a fixture's frontmatter reads the way the schema documents it
// rather than the way a map happened to iterate.
func (s *issueSpec) set(key, value string) {
	if _, ok := s.values[key]; !ok {
		s.keys = append(s.keys, key)
	}
	s.values[key] = value
}

func (s *issueSpec) unset(key string) {
	if _, ok := s.values[key]; !ok {
		return
	}

	delete(s.values, key)
	for i, k := range s.keys {
		if k == key {
			s.keys = append(s.keys[:i], s.keys[i+1:]...)
			break
		}
	}
}

func (s *issueSpec) render() string {
	var b strings.Builder

	b.WriteString("---\n")
	for _, key := range s.keys {
		fmt.Fprintf(&b, "%s: %s\n", key, s.values[key])
	}
	b.WriteString("---\n")

	if s.body != "" {
		b.WriteString(s.body)
		if !strings.HasSuffix(s.body, "\n") {
			b.WriteString("\n")
		}
	}

	return b.String()
}

// Issue writes issues/<id>/README.md and stages it, along with any comments and
// attachments the options asked for. It does not commit: what is on disk, what
// is in the index and what is in a commit are three different questions, and
// M2-S3 tests all three.
//
// Calling it again with the same id rewrites the file, which is how a fixture
// flips an issue's state on a branch.
func (r *Repo) Issue(id string, opts ...IssueOption) *Repo {
	r.t.Helper()

	spec := &issueSpec{values: map[string]string{}}
	spec.set("schema", "1")
	spec.set("id", id)
	spec.set("title", id)
	spec.set("type", "chore")
	spec.set("state", "open")
	spec.set("owner", "tester")
	spec.set("created", time.Now().Add(-r.offset).UTC().Format(time.DateOnly))

	for _, opt := range opts {
		opt(spec)
	}

	dir := issuesDir + "/" + id
	readme := dir + "/README.md"
	r.write(readme, spec.render())

	paths := []string{readme}
	for _, f := range spec.files {
		path := dir + "/" + f.path
		r.write(path, f.content)
		paths = append(paths, path)
	}

	r.Git(append([]string{"add", "--"}, paths...)...)

	return r
}

// Title sets the issue's one-line title.
func Title(title string) IssueOption { return Field("title", title) }

// Type sets the issue type: bug, story, chore, spike or epic. An epic must not
// declare a state, so building one is Type("epic") with Without("state").
func Type(issueType string) IssueOption { return Field("type", issueType) }

// State sets open, resolved or dropped.
func State(state string) IssueOption { return Field("state", state) }

// Owner sets the human answerable for the issue.
func Owner(owner string) IssueOption { return Field("owner", owner) }

// Created sets the creation date, which is what issue age is computed from.
func Created(date string) IssueOption { return Field("created", date) }

// Priority sets p0 to p3.
func Priority(priority string) IssueOption { return Field("priority", priority) }

// Parent points the issue at the epic it belongs to.
func Parent(id string) IssueOption { return Field("parent", id) }

// BlockedBy lists the issues this one waits on.
func BlockedBy(ids ...string) IssueOption {
	return Field("blocked_by", strings.Join(ids, ", "))
}

// Field sets any frontmatter key, including one the schema does not define —
// unknown keys survive a round trip by M1-S1, and something has to write one.
func Field(key, value string) IssueOption {
	return func(s *issueSpec) { s.set(key, value) }
}

// Without removes a frontmatter key. It is how a fixture writes an epic, which
// must not carry a state, and how it writes the invalid files the validator in
// M1-S4 has to reject.
func Without(key string) IssueOption {
	return func(s *issueSpec) { s.unset(key) }
}

// Body sets the markdown below the frontmatter.
func Body(body string) IssueOption {
	return func(s *issueSpec) { s.body = body }
}

// Comment adds a file under the issue's comments/ directory. Names are
// <date>-<author>-<nn>.md; the .md is added if it is missing.
func Comment(name, body string) IssueOption {
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}

	return func(s *issueSpec) {
		s.files = append(s.files, issueFile{path: "comments/" + name, content: body})
	}
}

// Attachment adds a file beside the issue's README, which is where attachments
// live: with the issue, in the same commit as the issue.
func Attachment(name, content string) IssueOption {
	return func(s *issueSpec) {
		s.files = append(s.files, issueFile{path: name, content: content})
	}
}
