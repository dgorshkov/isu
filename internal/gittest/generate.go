package gittest

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

// DefaultPrefix is the id prefix generated fixtures use.
const DefaultPrefix = "ISU"

// Spec describes a fixture repository too large to script a line at a time.
//
// M2-S3 has to prove that the working tree and a ref agree on 5,000 issues, and
// M2-S5 has to hold the board to a budget over 200 branches. Neither is a
// repository anybody writes out by hand.
type Spec struct {
	// Issues is how many issues trunk carries.
	Issues int
	// Branches is how many branches to build over trunk. Each resolves one
	// issue, taking them in turn from the start of the set.
	Branches int
	// Prefix is the id prefix. Empty means DefaultPrefix.
	Prefix string
}

func (s Spec) prefix() string {
	if s.Prefix == "" {
		return DefaultPrefix
	}

	return s.Prefix
}

// GeneratedID is the id Generate gives the nth issue, counting from zero.
func GeneratedID(prefix string, n int) string {
	if prefix == "" {
		prefix = DefaultPrefix
	}

	return fmt.Sprintf("%s-%06d", prefix, n)
}

// Generate builds a repository of the given size.
//
// Trunk is written straight to disk and staged in one `git add`, because a
// fixture that spent a git process per issue would cost more to build than the
// thing it exists to measure. The branches are built by one `git fast-import`
// for the same reason and one more: a branch per issue built by checking out
// and committing rewrites the index 200 times over, which on 5,000 issues is
// half a minute of a test doing nothing anybody asked about.
func Generate(t *testing.T, spec Spec) *Repo {
	t.Helper()

	r := New(t)
	prefix := spec.prefix()

	for n := range spec.Issues {
		id := GeneratedID(prefix, n)
		r.write(issuesDir+"/"+id+"/"+"README.md", generated(prefix, n, spec.Issues))
	}

	r.Git("add", "--all")
	r.Commit(fmt.Sprintf("%d issues", spec.Issues))

	if spec.Branches > 0 {
		r.importBranches(prefix, spec)
	}

	return r
}

// importBranches builds one branch per issue in a single git process.
func (r *Repo) importBranches(prefix string, spec Spec) {
	r.t.Helper()

	trunk := r.Head()
	when := time.Now().Add(-r.offset).Unix()

	var stream strings.Builder

	for n := range spec.Branches {
		// An epic has no state and cannot be resolved, so branches take the
		// issues that can be.
		which := resolvable(n, spec.Issues)
		if which < 0 {
			break
		}

		id := GeneratedID(prefix, which)
		path := issuesDir + "/" + id + "/README.md"

		fmt.Fprintf(&stream, "commit refs/heads/isu/%s\n", id)
		fmt.Fprintf(&stream,
			"committer isu tester <tester@example.invalid> %d +0000\n", when)
		data(&stream, "resolve "+id)
		fmt.Fprintf(&stream, "from %s\n", trunk)
		fmt.Fprintf(&stream, "M 100644 inline %s\n", path)
		data(&stream, resolvedOnBranch(prefix, which, spec.Issues))
	}

	stream.WriteString("done\n")

	if _, err := r.git.Feed(
		context.Background(), strings.NewReader(stream.String()),
		"fast-import", "--quiet", "--done",
	); err != nil {
		r.t.Fatalf("gittest: importing branches: %v", err)
	}
}

// resolvable maps a branch number to an issue that carries a state, skipping
// the epics. It returns -1 when there is no such issue left.
func resolvable(n, issues int) int {
	if generatedType(n) == "epic" {
		n++
	}
	if n >= issues {
		return -1
	}

	return n
}

// data writes a fast-import data block, which is length-prefixed rather than
// delimited so that a payload may contain anything at all.
func data(b *strings.Builder, payload string) {
	fmt.Fprintf(b, "data %d\n%s\n", len(payload), payload)
}

// generatedType cycles the five types, so that a fixture exercises the required
// fields rather than being five thousand identical chores.
func generatedType(n int) string {
	return []string{"chore", "story", "bug", "spike", "epic"}[n%5]
}

// generated renders the nth issue of a set that holds issues of them.
func generated(prefix string, n, issues int) string {
	spec := &issueSpec{values: map[string]string{}}
	spec.set("schema", "1")
	spec.set("id", GeneratedID(prefix, n))
	spec.set("title", fmt.Sprintf("Generated issue %d", n))
	spec.set("type", generatedType(n))

	if generatedType(n) != "epic" {
		spec.set("state", "open")
	}

	spec.set("owner", []string{"dmitry", "sam", "alex"}[n%3])
	spec.set("created", "2026-08-24")
	spec.set("priority", fmt.Sprintf("p%d", n%4))

	switch generatedType(n) {
	case "bug":
		spec.set("repro", "run it twice")
	case "story":
		spec.set("acceptance", "the board renders it")
	case "spike":
		spec.set("question", "which way round is this")
	}

	// Everything that is not an epic belongs to the epic that closes its block
	// of five, so the fixture has a hierarchy rather than five thousand
	// orphans. A parent that is not in the set would be a dangling reference,
	// which is M5-S2's to report and not a fixture's to write.
	if epic := n - n%5 + 4; generatedType(n) != "epic" && epic < issues {
		spec.set("parent", GeneratedID(prefix, epic))
	}

	spec.body = fmt.Sprintf("Generated for a fixture.\n\nThis is issue %d.\n", n)

	return spec.render()
}

// resolvedOnBranch is the nth issue with its state flipped, which is what a
// branch that did the work looks like.
func resolvedOnBranch(prefix string, n, issues int) string {
	return strings.Replace(
		generated(prefix, n, issues), "\nstate: open\n", "\nstate: resolved\n", 1)
}
