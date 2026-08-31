package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ZeroOID is the object id git spells "nothing was here". Passed to UpdateRef
// as the expected old value it means the ref must not already exist, which is
// how `isu claim` discovers locally that it lost a race before it ever reaches
// the network.
const ZeroOID = "0000000000000000000000000000000000000000"

// defaultMode is the file mode a TreeEdit that does not name one is written
// with: an ordinary, non-executable file, which is what every issue file is.
const defaultMode = "100644"

// TreeEdit is one file to write into a tree.
//
// There is no removal here because nothing in M4 removes a file this way. An
// unused capability is an untested one, and a tree builder that can silently
// delete is not the thing to leave lying around untested.
type TreeEdit struct {
	// Path is relative to the repository root, slash-separated.
	Path string
	// Blob is the file's contents.
	Blob []byte
	// Mode is git's file mode. Empty means an ordinary file.
	Mode string
}

// BuildTree writes edits over the tree at base and returns the new tree's
// object id. An empty base builds from nothing, which is the first commit of an
// empty repository.
//
// It goes through a temporary index rather than the repository's own, so that
// building a commit never touches the user's working tree, their index or their
// HEAD. That is not tidiness: `isu claim` has to work while somebody is in the
// middle of something, and it has to leave nothing behind when its push loses
// the race. A checkout-and-commit implementation can promise neither.
//
// GIT_INDEX_FILE is one of the variables environ scrubs, and setting it here is
// not a hole in that. What the scrub prevents is isu *inheriting* a pointer at
// somebody else's index from the process that ran it; this sets it deliberately,
// after the scrub, at a path this function made and removes.
func (g *Git) BuildTree(ctx context.Context, base string, edits []TreeEdit) (string, error) {
	dir, err := os.MkdirTemp("", "isu-index-")
	if err != nil {
		return "", fmt.Errorf("making room for a temporary index: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	index := filepath.Join(dir, "index")

	if base != "" {
		read := invocation{args: []string{"read-tree", base}, stdout: io.Discard, index: index}
		if err = g.run(ctx, read); err != nil {
			return "", err
		}
	}

	args := []string{"update-index", "--add"}

	for _, edit := range edits {
		var oid string

		oid, err = g.HashObject(ctx, edit.Blob)
		if err != nil {
			return "", err
		}

		mode := edit.Mode
		if mode == "" {
			mode = defaultMode
		}

		args = append(args, "--cacheinfo", mode+","+oid+","+edit.Path)
	}

	if err = g.run(ctx, invocation{args: args, stdout: io.Discard, index: index}); err != nil {
		return "", err
	}

	var out bytes.Buffer

	if err = g.run(ctx, invocation{
		args: []string{"write-tree"}, stdout: &out, index: index,
	}); err != nil {
		return "", err
	}

	return strings.TrimRight(out.String(), "\n"), nil
}

// HashObject writes data into the object database and returns its object id.
func (g *Git) HashObject(ctx context.Context, data []byte) (string, error) {
	return g.Feed(ctx, bytes.NewReader(data), "hash-object", "-w", "--stdin")
}

// CommitTree commits a tree under the given parents and returns the commit's
// object id. The message is fed on standard input, so a subject and a trailer
// go in exactly as written rather than through an argument list.
func (g *Git) CommitTree(
	ctx context.Context, tree string, parents []string, message string,
) (string, error) {
	args := []string{"commit-tree", tree}
	for _, parent := range parents {
		args = append(args, "-p", parent)
	}

	return g.Feed(ctx, strings.NewReader(message), append(args, "-F", "-")...)
}

// UpdateRef points ref at oid. When old is not empty the update is refused
// unless the ref is already there — ZeroOID for a ref that must not exist yet,
// which makes creating a branch a compare-and-swap rather than a clobber.
func (g *Git) UpdateRef(ctx context.Context, ref, oid, old string) error {
	args := []string{"update-ref", ref, oid}
	if old != "" {
		args = append(args, old)
	}

	return g.Run(ctx, args...)
}

// DeleteRef removes a ref. It is what a command that failed after creating a
// branch uses to leave nothing behind.
func (g *Git) DeleteRef(ctx context.Context, ref string) error {
	return g.Run(ctx, "update-ref", "-d", ref)
}

// Add stages paths from the working tree.
func (g *Git) Add(ctx context.Context, paths ...string) error {
	return g.Run(ctx, append([]string{"add", "--"}, paths...)...)
}

// Commit commits what is staged and returns the new commit's object id.
//
// An empty index is a failure rather than an empty commit: a command that meant
// to change a file and did not is a bug, and an empty commit hides it behind a
// success somebody has to explain later.
func (g *Git) Commit(ctx context.Context, message string) (string, error) {
	_, err := g.Feed(ctx, strings.NewReader(message), "commit", "--quiet", "-F", "-")
	if err != nil {
		return "", err
	}

	return g.RevParse(ctx, "HEAD")
}

// CurrentBranch is the branch HEAD points at, or the empty string on a detached
// HEAD.
//
// Detached is not a failure here. It is a state a person can be in, and the
// commands that refuse to write from it say so themselves, in their own words,
// rather than passing git's on.
func (g *Git) CurrentBranch(ctx context.Context) (string, error) {
	out, err := g.output(ctx, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		return out, nil
	}

	// --quiet makes "HEAD is not a symbolic ref" exit 1 and say nothing, which
	// is the one failure that is an answer rather than a problem.
	var gitErr *Error
	if errors.As(err, &gitErr) && gitErr.ExitCode == 1 && gitErr.Stderr == "" {
		return "", nil
	}

	return "", err
}

// Switch moves HEAD to a branch, creating it at the current HEAD when create is
// set. Creating one that is already there is a failure rather than a switch.
func (g *Git) Switch(ctx context.Context, branch string, create bool) error {
	args := []string{"switch", "--quiet"}
	if create {
		args = append(args, "--create")
	}

	return g.Run(ctx, append(args, branch)...)
}

// Remotes lists the configured remotes, in git's order.
func (g *Git) Remotes(ctx context.Context) ([]string, error) {
	out, err := g.output(ctx, "remote")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}

	return strings.Split(out, "\n"), nil
}

// Fetch updates remote-tracking refs.
//
// Like Push it may prompt: a human whose credential helper has expired should
// be asked for a password rather than told the fetch failed. It is the one read
// in this package that reaches the network, and the reason `isu board --fetch`
// exists at all — every derived status in this product is a statement about
// refs, so a board computed from a week-old fetch is a board about last week.
func (g *Git) Fetch(ctx context.Context, remote string, args ...string) error {
	all := append([]string{"fetch", "--quiet", remote}, args...)

	return g.run(ctx, invocation{args: all, stdout: io.Discard, interactive: true})
}
