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

// parseTree reads `ls-tree -z` output: <mode> SP <type> SP <oid> TAB <path>,
// NUL-terminated.
func parseTree(out string) ([]TreeEntry, error) {
	var entries []TreeEntry

	for _, record := range splitNUL(out) {
		head, path, ok := strings.Cut(record, "\t")
		if !ok {
			return nil, fmt.Errorf("git ls-tree: %q is not a tree entry", record)
		}

		fields := strings.Fields(head)
		if len(fields) != 3 {
			return nil, fmt.Errorf("git ls-tree: %q is not a tree entry", record)
		}

		entries = append(entries, TreeEntry{
			Mode: fields[0], Type: fields[1], OID: fields[2], Path: path,
		})
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
}

// ForEachRef lists the refs matching the given patterns.
//
// It is how the board finds its inputs: every branch, and every claim under
// refs/claims/. One process lists them all, which is what keeps M2-S5's process
// count linear in refs rather than quadratic.
func (g *Git) ForEachRef(ctx context.Context, patterns ...string) ([]Ref, error) {
	const format = "--format=%(refname)%00%(objectname)%00%(objecttype)%00%(*objectname)"

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
		if len(fields) != 4 {
			return nil, fmt.Errorf("git for-each-ref: %q is not a ref", line)
		}

		refs = append(refs, Ref{
			Name: fields[0], OID: fields[1], Type: fields[2], Target: fields[3],
		})
	}

	return refs, nil
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
