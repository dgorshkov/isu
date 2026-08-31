package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// The branch namespaces. `isu/<ID>` is the claim — a branch there carrying
// `state: resolved` is what makes an issue read `in progress`, and nothing else
// does. The other two are deliberately outside it: a report and a triage edit
// are somebody's work on a branch, and until they claim, the board does not
// speak for them.
const (
	claimBranchPrefix  = "isu/"
	reportBranchPrefix = "report/"
	triageBranchPrefix = "triage/"
)

// change is one file a command writes.
type change struct {
	// path is relative to the repository root, slash-separated.
	path string
	blob []byte
}

// commitSpec is one commit a command wants written.
type commitSpec struct {
	// branch is the short branch name to write onto.
	branch string
	// base is where to start the branch when it does not exist yet.
	base string
	// message is the whole commit message, subject and body.
	message string
	changes []change
}

// commitOn writes a commit onto a branch.
//
// When the branch is the one HEAD is on, the write goes through the working
// tree, because moving a ref out from under a checkout leaves the user looking
// at a file that is no longer what their branch says. Every other case is built
// with plumbing and never goes near their tree — which is what lets `isu claim`
// work while somebody is mid-edit, and what lets a claim that loses its race
// leave nothing behind.
func (s *session) commitOn(ctx context.Context, spec commitSpec) (string, error) {
	current, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}

	if current == spec.branch {
		return s.commitHere(ctx, spec.message, spec.changes)
	}

	ref := "refs/heads/" + spec.branch

	old, base, parents, err := s.startFrom(ctx, ref, spec.base)
	if err != nil {
		return "", err
	}

	edits := make([]gitx.TreeEdit, 0, len(spec.changes))
	for _, c := range spec.changes {
		edits = append(edits, gitx.TreeEdit{Path: c.path, Blob: c.blob})
	}

	tree, err := s.git.BuildTree(ctx, base, edits)
	if err != nil {
		return "", err
	}

	commit, err := s.git.CommitTree(ctx, tree, parents, spec.message)
	if err != nil {
		return "", err
	}

	if err := s.git.UpdateRef(ctx, ref, commit, old); err != nil {
		return "", err
	}

	return commit, nil
}

// startFrom works out what a commit on ref hangs off: the value the ref must
// still have for the update to be safe, the tree to build over, and the parents.
//
// A branch that is already there is all three answers at once — it is where the
// new commit starts, what it must not have moved from, and its parent — so it
// is looked up once. A branch that is not there starts where the caller said,
// and a caller's base that does not resolve either is an empty repository,
// whose first commit has no parent at all.
func (s *session) startFrom(
	ctx context.Context, ref, wanted string,
) (old, base string, parents []string, err error) {
	tip, err := s.git.RevParse(ctx, ref)

	switch {
	case err == nil:
		return tip, tip, []string{tip}, nil
	case !errors.Is(err, gitx.ErrUnknownRevision):
		return "", "", nil, err
	}

	start, err := s.git.RevParse(ctx, wanted)

	switch {
	case err == nil:
		return gitx.ZeroOID, wanted, []string{start}, nil
	case !errors.Is(err, gitx.ErrUnknownRevision):
		return "", "", nil, err
	}

	return gitx.ZeroOID, "", nil, nil
}

// commitHere stages changes in the working tree and commits them where HEAD
// already is.
func (s *session) commitHere(
	ctx context.Context, message string, changes []change,
) (string, error) {
	if _, err := s.stage(ctx, changes); err != nil {
		return "", err
	}

	return s.git.Commit(ctx, message)
}

// stage writes changes into the working tree and adds them to the index.
//
// Staging rather than only writing is what makes `isu new --no-branch` usable
// for the bulk conversion it exists for: a hundred issue folders that then take
// one commit, rather than a hundred folders and a `git add` somebody has to
// remember.
func (s *session) stage(ctx context.Context, changes []change) ([]string, error) {
	paths := make([]string, 0, len(changes))

	for _, c := range changes {
		full := filepath.Join(s.root, filepath.FromSlash(c.path))

		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return nil, fmt.Errorf("making %s: %w", filepath.Dir(c.path), err)
		}

		if err := os.WriteFile(full, c.blob, 0o644); err != nil { //nolint:gosec // an issue file is not a secret
			return nil, fmt.Errorf("writing %s: %w", c.path, err)
		}

		paths = append(paths, c.path)
	}

	if err := s.git.Add(ctx, paths...); err != nil {
		return nil, err
	}

	return paths, nil
}

// readIssueAt reads one issue's file as a revision holds it.
//
// It is `git show` rather than the batch read path: that path exists because
// five thousand issues cost five thousand lookups, and one issue costs one.
func (s *session) readIssueAt(ctx context.Context, rev, id string) (*issue.Issue, error) {
	blob, err := s.git.Show(ctx, rev+":"+readmePath(id))
	if err != nil {
		return nil, fmt.Errorf("reading %s at %s: %w", id, rev, err)
	}

	return decodeIssue(blob, id)
}

// readIssueHere reads one issue's file as the working tree holds it, which is
// what the commands that write on the current branch are editing.
func (s *session) readIssueHere(id string) (*issue.Issue, error) {
	path := filepath.Join(s.root, filepath.FromSlash(readmePath(id)))

	blob, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", readmePath(id), err)
	}

	return decodeIssue(blob, id)
}

func decodeIssue(blob []byte, id string) (*issue.Issue, error) {
	doc, err := issue.Parse(blob)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}

	decoded, err := issue.Decode(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", id, err)
	}

	decoded.Folder = id

	return decoded, nil
}

// push sends a branch to the remote it belongs on.
//
// There is one remote question worth asking and it is which one, not whether:
// a repository with no remote cannot claim, because the push is the
// compare-and-swap that makes a claim mean anything.
func (s *session) push(ctx context.Context, branch string) error {
	remotes, err := s.git.Remotes(ctx)
	if err != nil {
		return err
	}

	if len(remotes) == 0 {
		return errors.New(
			"this repository has no remote: a claim is a branch push, and a push " +
				"nobody else can see settles nothing")
	}

	return s.git.Push(ctx, remotes[0], "refs/heads/"+branch+":refs/heads/"+branch)
}

// mustNotBeTrunk refuses a command that would write where state is true.
//
// `resolve` and `drop` leave a branch for a pull request. Running one on trunk
// would make the only way to reach `done` a person typing a command, where the
// whole point of the model is that it is a merged pull request.
func (s *session) mustNotBeTrunk(ctx context.Context, cmd *cobra.Command) error {
	current, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	trunk, err := s.trunkBranch(ctx)
	if err != nil {
		return err
	}

	if s.trunk == "HEAD" {
		// isu is reading whatever HEAD is as trunk, because this repository's
		// trunk is called neither main nor master and origin never said. It
		// therefore cannot tell whether this branch is trunk, and guessing
		// wrong in either direction is worse than asking.
		return usagef(cmd,
			"isu cannot tell what trunk is called here — it is neither main nor "+
				"master, and there is no origin/HEAD to ask — so it cannot tell "+
				"whether %s writes onto it. Name trunk with --ref", cmd.Name())
	}

	if current != "" && current == trunk {
		return usagef(cmd,
			"%s writes a commit for a pull request to carry, and you are on %s: "+
				"branch first, or the only way to reach done is somebody typing a command",
			cmd.Name(), current)
	}

	return nil
}

// trunkBranch is the branch name trunk resolves to, or empty when the ref this
// invocation calls trunk is not a local branch at all.
//
// `--ref` takes any revision — a tag, a raw commit id, a remote-tracking ref —
// and none of those is somewhere a commit can be written. Answering with the
// string it was given would have `isu triage --push` create a branch named
// after a sha, which is a branch nobody asked for and nothing reads.
func (s *session) trunkBranch(ctx context.Context) (string, error) {
	if s.trunk == "HEAD" {
		return s.git.CurrentBranch(ctx)
	}

	name := strings.TrimPrefix(s.trunk, "refs/heads/")

	if _, err := s.git.RevParse(ctx, "refs/heads/"+name); err != nil {
		if errors.Is(err, gitx.ErrUnknownRevision) {
			return "", nil
		}

		return "", err
	}

	return name, nil
}

// requireID is the argument validator every command that names an issue uses.
func requireID(cmd *cobra.Command, args []string) error {
	if len(args) != 1 {
		return usagef(cmd, "%s needs exactly one issue id", cmd.Name())
	}

	if !issue.ValidID(args[0]) {
		return usagef(cmd, "%q is not an issue id", args[0])
	}

	return nil
}

// noArgs is the validator for the commands that read the whole repository.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usagef(cmd, "%s takes no arguments, and got %q", cmd.Name(), args[0])
	}

	return nil
}
