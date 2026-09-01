package gitx

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// TreeEntry is one row of `git ls-tree -r`: what an object is, and where it
// sits in the tree.
type TreeEntry struct {
	// Mode is the file mode, as git writes it: 100644, 100755, 120000, 160000.
	Mode string
	// Type is blob, tree or commit. `-r` flattens trees away, so a tree here is
	// a submodule's gitlink rather than a directory.
	Type string
	// OID is the object id. It is the point of this whole call: PLAN.md §0
	// measures `cat-file --batch` fed object ids at 0.6 s where the same batch
	// fed `ref:path` takes 4.9 s, because a path costs a tree walk per lookup.
	OID string
	// Path is relative to the repository root, whatever directory git ran in.
	Path string
	// Size is the blob's size in bytes, and is set only by LsTreeLong. It is
	// zero everywhere else, and -1 for an object that has no size — a tree, or
	// a submodule's gitlink, both of which git reports as `-`.
	Size int64
}

// LsTree lists every object under paths at ref, recursively.
//
// It is step one of the read path PLAN.md mandates: take the object id of every
// issues/*/README.md here, then read them all in one batch.
func (g *Git) LsTree(ctx context.Context, ref string, paths ...string) ([]TreeEntry, error) {
	args := []string{"ls-tree", "-r", "-z", "--full-tree", ref}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	out, err := g.output(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseTree(out)
}

// LsTreeLong is LsTree with the size of every blob.
//
// It is a separate call rather than a flag on the one above because `-l` makes
// git look up the size of every object it lists, and the read path in PLAN.md
// §0 lists five thousand issues without wanting one of them. The attachment cap
// in M5-S2 is the opposite case: it is asking about sizes and nothing else.
func (g *Git) LsTreeLong(ctx context.Context, ref string, paths ...string) ([]TreeEntry, error) {
	args := []string{"ls-tree", "-r", "-l", "-z", "--full-tree", ref}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	out, err := g.output(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseTree(out)
}

// MergeBase is where two revisions diverged.
//
// The branch checks in M5 are about what a branch proposes, which is its own
// difference from trunk and not trunk's difference from it. `git diff trunk
// head` answers the second question as well as the first: a trunk that has
// moved on since the branch left shows up as the branch reverting work it never
// touched. Diffing from the merge base is the fix, and this is the one process
// it costs.
func (g *Git) MergeBase(ctx context.Context, a, b string) (string, error) {
	out, err := g.output(ctx, "merge-base", a, b)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// parseTree reads `ls-tree -z` output: <mode> SP <type> SP <oid> TAB <path>,
// NUL-terminated. Under `-l` a size sits between the object id and the tab, and
// is `-` for anything that is not a blob.
func parseTree(out string) ([]TreeEntry, error) {
	var entries []TreeEntry

	for _, record := range splitNUL(out) {
		head, path, ok := strings.Cut(record, "\t")
		if !ok {
			return nil, fmt.Errorf("git ls-tree: %q is not a tree entry", record)
		}

		fields := strings.Fields(head)
		if len(fields) != 3 && len(fields) != 4 {
			return nil, fmt.Errorf("git ls-tree: %q is not a tree entry", record)
		}

		entry := TreeEntry{
			Mode: fields[0], Type: fields[1], OID: fields[2], Path: path,
		}

		if len(fields) == 4 {
			size, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil {
				// `-`, which is what git prints for an object whose size it
				// does not report: a tree, or a submodule's gitlink.
				size = -1
			}

			entry.Size = size
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// Object is one object out of a `cat-file --batch` stream.
type Object struct {
	// OID is the object id, echoed back by git.
	OID string
	// Type is blob, tree, commit or tag.
	Type string
	// Size is what the header declared, which is what was read.
	Size int64
	// Data is the object's contents, verbatim.
	Data []byte
}

// CatFileBatch reads every object in one git process, calling fn for each as it
// arrives.
//
// Objects are fed in as object ids rather than as ref:path, which is the whole
// performance requirement in PLAN.md §0: a path costs a tree walk per lookup
// and an object id does not. Feeding nothing runs nothing.
//
// fn is called in stream order. An error from fn stops the walk, kills git and
// comes back unwrapped, so a caller can test for its own sentinel.
func (g *Git) CatFileBatch(ctx context.Context, oids []string, fn func(Object) error) error {
	if len(oids) == 0 {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var stdin strings.Builder

	stdin.Grow(len(oids) * 41)
	for _, oid := range oids {
		stdin.WriteString(oid)
		stdin.WriteByte('\n')
	}

	reader, writer := io.Pipe()

	done := make(chan error, 1)

	go func() {
		err := g.run(ctx, invocation{
			args:   []string{"cat-file", "--batch"},
			stdin:  strings.NewReader(stdin.String()),
			stdout: writer,
		})
		_ = writer.CloseWithError(err)
		done <- err
	}()

	parsed := parseBatch(reader, fn)
	if parsed != nil {
		// Stop git rather than let it fill a pipe nobody is reading.
		_ = reader.CloseWithError(parsed)
		cancel()
	} else {
		_, _ = io.Copy(io.Discard, reader)
	}

	ran := <-done

	if parsed != nil {
		return parsed
	}

	return ran
}

// parseBatch reads the `cat-file --batch` stream: a header line, then exactly
// the number of bytes it declared, then the newline git puts after them.
//
// Counting bytes is the whole of it. A parser that instead scanned for the next
// header-shaped line would split any object whose own body contains one — and
// an issue file quoting a git error, a build log or another issue's frontmatter
// does exactly that.
func parseBatch(r io.Reader, fn func(Object) error) error {
	br := bufio.NewReaderSize(r, 64*1024)

	for {
		header, err := br.ReadString('\n')
		if errors.Is(err, io.EOF) && header == "" {
			return nil
		}
		if err != nil {
			return fmt.Errorf("reading the cat-file stream: %w", err)
		}

		fields := strings.Fields(strings.TrimSuffix(header, "\n"))
		if len(fields) == 2 && fields[1] == "missing" {
			return fmt.Errorf("git cat-file: object %s is missing from this repository", fields[0])
		}
		if len(fields) != 3 {
			return fmt.Errorf("git cat-file: %q is not a batch header",
				strings.TrimSuffix(header, "\n"))
		}

		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			return fmt.Errorf("git cat-file: %q is not an object size: %w", fields[2], err)
		}

		data := make([]byte, size)
		if _, err := io.ReadFull(br, data); err != nil {
			return fmt.Errorf("reading object %s: %w", fields[0], err)
		}
		if _, err := br.Discard(1); err != nil {
			return fmt.Errorf("reading object %s: %w", fields[0], err)
		}

		if err := fn(Object{OID: fields[0], Type: fields[1], Size: size, Data: data}); err != nil {
			return err
		}
	}
}

// RevParse resolves a revision to an object id.
func (g *Git) RevParse(ctx context.Context, rev string) (string, error) {
	return g.output(ctx, "rev-parse", "--verify", rev)
}

// Ref is one row of `git for-each-ref`.
type Ref struct {
	// Name is the full ref name: refs/heads/main, refs/claims/AR-7f3akq.
	Name string
	// OID is what the ref points at.
	OID string
	// Type is the object type at OID: commit for a branch and for a claim,
	// tag for an annotated tag.
	Type string
	// Target is what an annotated tag dereferences to, and empty otherwise.
	Target string
	// Created is the date of what the ref points at: a commit's committer date,
	// an annotated tag's tagger date. It is `creatordate` rather than
	// `committerdate` because the second is empty on a tag, and a freshness
	// line that silently reads zero for half a repository's refs is worse than
	// no freshness line.
	//
	// M4-S2 is the caller: `isu board` says how old the newest remote ref is,
	// because every derived status in this product is a statement about refs
	// and two engineers with different fetch ages see different contention.
	Created time.Time
}

// ForEachRef lists the refs matching the given patterns.
//
// It is how the board finds its inputs: every branch, and every claim under
// refs/claims/. One process lists them all, which is what keeps M2-S5's process
// count linear in refs rather than quadratic.
func (g *Git) ForEachRef(ctx context.Context, patterns ...string) ([]Ref, error) {
	const format = "--format=%(refname)%00%(objectname)%00%(objecttype)%00" +
		"%(*objectname)%00%(creatordate:unix)"

	out, err := g.output(ctx, append([]string{"for-each-ref", format}, patterns...)...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	var refs []Ref

	// A ref name cannot contain a newline, so the records are lines and only
	// the fields inside one need a separator git will not produce itself.
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Split(line, "\x00")
		if len(fields) != 5 {
			return nil, fmt.Errorf("git for-each-ref: %q is not a ref", line)
		}

		ref := Ref{
			Name: fields[0], OID: fields[1], Type: fields[2], Target: fields[3],
		}

		// A ref pointing at something with no date of its own leaves the field
		// empty. That is a ref with no age rather than a ref from 1970, so it
		// stays the zero time and the caller decides what to say about it.
		if seconds, err := strconv.ParseInt(fields[4], 10, 64); err == nil {
			ref.Created = time.Unix(seconds, 0).UTC()
		}

		refs = append(refs, ref)
	}

	return refs, nil
}

// Toplevel is the root of the working tree the bound directory sits inside.
//
// Every command takes a path or the working directory and has to turn it into
// the repository root, because `issues/` is anchored there and not wherever the
// user happened to be standing.
func (g *Git) Toplevel(ctx context.Context) (string, error) {
	return g.output(ctx, "rev-parse", "--show-toplevel")
}

// DiffNameOnly lists the paths that differ between two revisions.
//
// M5-S3's evidence check is the caller that matters: resolving a bug requires a
// change outside issues/, and this is the question that answers it.
func (g *Git) DiffNameOnly(ctx context.Context, from, to string, paths ...string) ([]string, error) {
	args := []string{"diff", "--name-only", "-z", from, to}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	out, err := g.output(ctx, args...)
	if err != nil {
		return nil, err
	}

	return splitNUL(out), nil
}

// DiffTree lists what changed between two revisions, with the object id on
// either side of each change.
//
// It is DiffNameOnly's answer plus the object ids, which is the difference
// between knowing that a branch touched an issue and being able to read what it
// says — and it costs no more, because git compares trees by object id and
// skips the subtrees that match. That is what lets the board read two hundred
// branches without listing five thousand issues two hundred times.
//
// Renames are not detected, for the same reason as in Log: a moved issue folder
// is one issue ending and another beginning.
func (g *Git) DiffTree(ctx context.Context, from, to string, paths ...string) ([]Change, error) {
	args := []string{"diff-tree", "-r", "--no-abbrev", "--no-renames", "-z", from, to}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}

	out, err := g.output(ctx, args...)
	if err != nil {
		return nil, err
	}

	return parseChanges(splitNUL(out))
}

// Show returns an object's bytes.
//
// Nothing on the fast read path uses it — that is LsTree plus CatFileBatch —
// but reading one issue at one ref is a whole command in M4, and doing it with
// a batch of one would be theatre.
func (g *Git) Show(ctx context.Context, object string) ([]byte, error) {
	var buf bytes.Buffer

	if err := g.run(ctx, invocation{args: []string{"show", object}, stdout: &buf}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// Push sends refspecs to a remote.
//
// It is the one command here that may prompt: a human whose credential helper
// has expired should be asked for a password rather than told the push failed.
// The reads never prompt, because a board that blocks on a hidden prompt looks
// exactly like a board that hung.
func (g *Git) Push(ctx context.Context, remote string, refspecs ...string) error {
	args := append([]string{"push", remote}, refspecs...)

	return g.run(ctx, invocation{args: args, stdout: io.Discard, interactive: true})
}

// splitNUL cuts a NUL-separated list, dropping the empty tail a trailing
// separator leaves behind.
func splitNUL(out string) []string {
	out = strings.TrimSuffix(out, "\x00")
	if out == "" {
		return nil
	}

	return strings.Split(out, "\x00")
}
