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
	// Commits is how many commits trunk's history holds, the one that writes
	// the issues included. Zero and one both mean that single commit.
	//
	// Every other dimension here is bounded by what a repository holds today.
	// This one is bounded by how long the repository has existed, and it only
	// ever grows — which makes it the dimension a fixture is most likely to
	// leave untested and a real repository most likely to hit.
	Commits int
	// TouchesIssues is how many of those commits change an issue file. Zero
	// means all of them. Trunk in a real repository is mostly code, and a
	// commit that touches nothing under issues/ still has to be walked, so the
	// two costs are separable and worth separating.
	TouchesIssues int
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
func Generate(t testing.TB, spec Spec) *Repo {
	t.Helper()

	r := New(t)
	prefix := spec.prefix()

	for n := range spec.Issues {
		id := GeneratedID(prefix, n)
		r.write(issuesDir+"/"+id+"/"+"README.md", generated(prefix, n, spec.Issues))
	}

	r.Git("add", "--all")
	r.Commit(fmt.Sprintf("%d issues", spec.Issues))

	if spec.Commits > 1 {
		r.importHistory(prefix, spec)
	}

	if spec.Branches > 0 {
		r.importBranches(prefix, spec)
	}

	return r
}

// importHistory lengthens trunk to spec.Commits, in one git process.
//
// The commits that touch an issue flip its state, taking issues in turn and
// wrapping; the rest write a file outside issues/, which is what most of a real
// trunk is. Both have to be walked to answer a question about history, and only
// one of them costs a blob read, so a fixture that made every commit an issue
// commit would measure the wrong half.
//
// fast-import moves the ref without touching the index or the working tree, so
// the checkout is brought back to the new tip afterwards — otherwise every
// later `git add` in the same repository would try to revert the history this
// just wrote.
func (r *Repo) importHistory(prefix string, spec Spec) {
	r.t.Helper()

	extra := spec.Commits - 1
	touching := spec.TouchesIssues
	if touching <= 0 || touching > extra {
		touching = extra
	}

	when := time.Now().Add(-r.offset).Unix()
	head := r.Head()

	var stream strings.Builder

	for n := range extra {
		// A commit record is the ref, who wrote it, the message, and only then
		// its parent and its files. Only the first names a parent; the rest
		// continue the branch fast-import is already holding.
		fmt.Fprintf(&stream, "commit refs/heads/%s\n", DefaultBranch)
		fmt.Fprintf(&stream,
			"committer isu tester <tester@example.invalid> %d +0000\n", when+int64(n))

		// The issue commits go first, so that a fixture asking for a few of
		// them among many gets a trunk whose recent history is ordinary code —
		// the shape that makes a walk look cheap until it is not.
		touchesIssue := n < touching

		which := n % spec.Issues
		id := GeneratedID(prefix, which)

		if touchesIssue {
			data(&stream, "resolve "+id)
		} else {
			data(&stream, fmt.Sprintf("commit %d", n))
		}

		if n == 0 {
			fmt.Fprintf(&stream, "from %s\n", head)
		}

		if touchesIssue {
			fmt.Fprintf(&stream, "M 100644 inline %s/%s/README.md\n", issuesDir, id)
			data(&stream, touched(prefix, which, spec.Issues, n/spec.Issues))

			continue
		}

		fmt.Fprintf(&stream, "M 100644 inline src/file%04d.go\n", n%512)
		data(&stream, fmt.Sprintf("package src\n\n// revision %d\n", n))
	}

	stream.WriteString("done\n")

	if _, err := r.git.Feed(
		context.Background(), strings.NewReader(stream.String()),
		"fast-import", "--quiet", "--done",
	); err != nil {
		r.t.Fatalf("gittest: importing history: %v", err)
	}

	r.Git("reset", "--hard", "--quiet", DefaultBranch)
}

// importBranches builds one branch per issue in a single git process.
func (r *Repo) importBranches(prefix string, spec Spec) {
	r.t.Helper()

	trunk := r.Head()
	when := time.Now().Add(-r.offset).Unix()

	var stream strings.Builder

	// An epic has no state and cannot be resolved, so branches take the issues
	// that can be, in turn. Mapping a branch number straight onto an issue
	// number and nudging past the epics would hand two branch numbers the same
	// issue, and a fixture asked for two hundred branches would quietly build
	// a hundred and sixty.
	which := -1

	for range spec.Branches {
		for {
			which++
			if which >= spec.Issues {
				break
			}
			if generatedType(which) != "epic" {
				break
			}
		}

		if which >= spec.Issues {
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

// touched is the nth issue after the revisionth trunk commit to change it: the
// state flipped back and forth, and the revision written into the body.
//
// The body is what makes this honest. Writing the same bytes twice records no
// diff the second time, because git compares blobs by object id — so a deep
// history built out of repeated content is a deep history git can walk almost
// for free, and a measurement taken on one is a measurement of deduplication
// rather than of depth. Every change here is a blob nothing else in the
// repository shares, which is what an issue edited over years actually looks
// like.
func touched(prefix string, n, issues, revision int) string {
	state := "resolved"
	if revision%2 == 1 {
		state = "open"
	}

	return strings.Replace(
		generated(prefix, n, issues), "\nstate: open\n", "\nstate: "+state+"\n", 1) +
		fmt.Sprintf("\nRevised %d times.\n", revision+1)
}
