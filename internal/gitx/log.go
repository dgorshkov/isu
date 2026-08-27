package gitx

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Signature is who did something and when.
type Signature struct {
	Name  string
	Email string
	When  time.Time
}

// Change is one row of `git log --raw`: a path whose blob changed, with the
// object ids on either side of the change.
//
// The new object id is what makes M2-S4 possible in two processes. Reading the
// state an issue held at a trunk commit means reading the blob, and the blob is
// named right here — without a second tree walk, and without ever asking git
// who authored the change, which a squash merge would have rewritten anyway.
type Change struct {
	// Status is git's letter: A, M, D, T. Renames are not detected, so R and C
	// do not appear — see LogSpec.
	Status string
	// OldOID is the blob before the change, or forty zeroes on an addition.
	OldOID string
	// NewOID is the blob after it, or forty zeroes on a deletion.
	NewOID string
	// Path is relative to the repository root.
	Path string
}

// Deleted reports whether the change removed the path.
func (c Change) Deleted() bool { return strings.HasPrefix(c.Status, "D") }

// Commit is one commit as `git log` reported it.
type Commit struct {
	OID       string
	Parents   []string
	Author    Signature
	Committer Signature
	// Subject is the first line of the message. A squash merge composed by the
	// forge puts the pull request's title here, which is the third tier of the
	// resolving-commit recovery in PLAN.md.
	Subject string
	// Body is everything below the subject, which is where the Isu-Resolves
	// trailer lives.
	Body string
	// Changes is populated when LogSpec.Raw is set.
	Changes []Change
}

// LogSpec is one call to `git log`.
type LogSpec struct {
	// Rev is the revision to walk.
	Rev string
	// Paths limits the walk to commits that touched them.
	Paths []string
	// FirstParent walks only the first-parent chain — trunk's own timeline —
	// and, crucially, makes a merge commit report the change it brought in.
	// Without it a merge shows no diff at all, so an issue resolved on a branch
	// and merged would look, at trunk, as though nothing ever happened.
	FirstParent bool
	// Raw asks for the changed paths and their object ids.
	Raw bool
	// Reverse walks oldest first.
	Reverse bool
	// Limit caps how many commits come back. Zero means all of them.
	Limit int
}

// logFormat is one record per commit, NUL-separated within the record and
// preceded by a NUL and a SOH that mark where a record starts.
//
// The marker is needed because --raw appends a variable number of entries after
// the format, so there is nothing structural to split records on. NUL cannot
// appear in a commit message or a path, so the two-byte sequence can only be a
// record boundary.
const logFormat = "%x00%x01" +
	"%H%x00%P%x00" +
	"%an%x00%ae%x00%aI%x00" +
	"%cn%x00%ce%x00%cI%x00" +
	"%s%x00%b"

// logRecordSeparator opens every record, the first one included.
const logRecordSeparator = "\x00\x01"

// logFields is how many format fields precede the raw entries.
const logFields = 10

// Log walks a revision.
func (g *Git) Log(ctx context.Context, spec LogSpec) ([]Commit, error) {
	args := []string{"log", "--format=" + logFormat, "-z"}

	if spec.FirstParent {
		args = append(args, "--first-parent")
	}
	if spec.Reverse {
		args = append(args, "--reverse")
	}
	if spec.Limit > 0 {
		args = append(args, "--max-count="+strconv.Itoa(spec.Limit))
	}
	if spec.Raw {
		// --no-abbrev because an abbreviated object id cannot be fed to
		// cat-file --batch without git resolving it again, and --no-renames
		// because M2-S4 documents renames as not followed: a moved issue folder
		// is a delete and an add, which is a limitation a test can assert
		// rather than a heuristic that changes with git's version.
		args = append(args, "--raw", "--no-abbrev", "--no-renames")
	}

	args = append(args, spec.Rev)

	if len(spec.Paths) > 0 {
		args = append(args, "--")
		args = append(args, spec.Paths...)
	}

	out, err := g.output(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseLog(out)
}

func parseLog(out string) ([]Commit, error) {
	var commits []Commit

	for _, record := range strings.Split(out, logRecordSeparator) {
		if record == "" {
			continue
		}

		commit, err := parseLogRecord(record)
		if err != nil {
			return nil, err
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

func parseLogRecord(record string) (Commit, error) {
	fields := strings.Split(record, "\x00")
	if len(fields) < logFields {
		return Commit{}, fmt.Errorf("git log: %q is not a commit", record)
	}

	author, err := signature(fields[2], fields[3], fields[4])
	if err != nil {
		return Commit{}, err
	}

	committer, err := signature(fields[5], fields[6], fields[7])
	if err != nil {
		return Commit{}, err
	}

	commit := Commit{
		OID:       fields[0],
		Parents:   strings.Fields(fields[1]),
		Author:    author,
		Committer: committer,
		Subject:   fields[8],
		Body:      fields[9],
	}

	commit.Changes, err = parseChanges(fields[logFields:])
	if err != nil {
		return Commit{}, err
	}

	return commit, nil
}

// parseChanges reads the --raw entries that follow the format.
//
// Each is `:<oldmode> <newmode> <oldoid> <newoid> <status>` followed by its
// path, both NUL-terminated. The first one is preceded by the newline git puts
// between the format and the raw block.
func parseChanges(fields []string) ([]Change, error) {
	var changes []Change

	for i := 0; i < len(fields); {
		head := strings.TrimLeft(fields[i], "\n")
		if head == "" {
			i++
			continue
		}
		if !strings.HasPrefix(head, ":") {
			return nil, fmt.Errorf("git log --raw: %q is not a change", head)
		}
		if i+1 >= len(fields) {
			return nil, fmt.Errorf("git log --raw: %q has no path", head)
		}

		parts := strings.Fields(strings.TrimPrefix(head, ":"))
		if len(parts) != 5 {
			return nil, fmt.Errorf("git log --raw: %q is not a change", head)
		}

		changes = append(changes, Change{
			Status: parts[4],
			OldOID: parts[2],
			NewOID: parts[3],
			Path:   fields[i+1],
		})

		i += 2
	}

	return changes, nil
}

func signature(name, email, when string) (Signature, error) {
	at, err := time.Parse(time.RFC3339, when)
	if err != nil {
		return Signature{}, fmt.Errorf("git log: %q is not a date: %w", when, err)
	}

	return Signature{Name: name, Email: email, When: at}, nil
}
