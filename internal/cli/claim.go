package cli

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
)

// claimTrailer carries the one thing that makes a claim commit unique.
//
// **PLAN.md's claim design has a hole, and the stress test in this milestone
// found it.** §1 argues that the push is the compare-and-swap because "two
// claimants produce two different commits — different author, different
// timestamp, therefore different object ids". Two claimants under one identity,
// in the same second, produce the *same* commit: same tree, same parent, same
// author, same second, same message. Git then answers the second push
// `Everything up-to-date`, exit 0, and both of them believe they won.
//
// Measured, with a hundred clones racing for one issue: **thirty winners.**
// Two agents sharing a bot identity is not an exotic case for a tracker built
// for agents, and neither is one person in two clones.
//
// `--force-with-lease=refs/heads/isu/<ID>:` — PLAN.md's own third mechanism —
// does not close it, because git short-circuits on "up to date" before the
// lease is ever evaluated; that was measured too. What does close it is making
// the sentence §1 already relies on true: atomicity comes from committing
// something nobody else can have committed, so the claim commits something
// nobody else can have. With the nonce, the second push is rejected as a
// non-fast-forward, which is exactly the mechanism the section describes.
const claimTrailer = "Isu-Claim"

func (a *app) claimCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "claim <id>",
		Short: "say you are working on an issue, before you start",
		Long: `claim creates isu/<id> from trunk, writes state: resolved into the issue and
pushes — in that order, and before any work, so that whoever loses the race
wastes nothing.

The push is the compare-and-swap and the state flip is what makes it one. Two
claimants write two different commits, so the second push is not a fast-forward
and git rejects it. Pushing a bare branch at the trunk tip would tell both of
them they had won.

The state says resolved before the work is done on purpose. A branch is a
proposal, not a fact: it says this branch intends to resolve this issue, and it
becomes true about the repository when it merges. It does not get you past the
evidence check — a branch that claimed and did nothing changed one line of one
README, which is exactly what M5-S3 rejects.

Nothing is checked out. The commit is built with plumbing, so claiming works
mid-edit and a claim that loses its race leaves nothing behind.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.claim(cmd, args[0])
		},
	}
}

func (a *app) claim(cmd *cobra.Command, id string) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	v, err := s.view(ctx)
	if err != nil {
		return err
	}

	item, ok := v.board.Get(id)
	if !ok {
		return fmt.Errorf("no issue %s on %s or any branch beside it", id, v.trunkName)
	}

	if item.Status.Terminal() {
		return fmt.Errorf(
			"%s is already %s: claiming it would say a finished issue is being worked on",
			id, item.Status)
	}

	branch := claimBranchPrefix + id

	if err := s.branchIsFree(ctx, branch, item); err != nil {
		return err
	}

	flipped, err := s.readIssueAt(ctx, s.trunk, id)
	if err != nil {
		return err
	}

	flipped.State = issue.StateResolved

	commit, err := s.commitOn(ctx, commitSpec{
		branch:  branch,
		base:    s.trunk,
		message: "claim " + id + "\n\n" + claimTrailer + ": " + rand.Text() + "\n",
		changes: []change{{path: readmePath(id), blob: flipped.Encode()}},
	})
	if err != nil {
		return err
	}

	if err := s.push(ctx, branch); err != nil {
		// The claim is the push. Losing it leaves nothing behind, which is the
		// whole reason the commit was built without a checkout.
		_ = s.git.DeleteRef(ctx, "refs/heads/"+branch)

		return s.lostRace(ctx, id, branch, err)
	}

	return a.reportWrite(Write{
		ID: id, Branch: branch, Commit: commit,
		Paths: []string{readmePath(id)}, Pushed: true,
	})
}

// branchIsFree refuses a claim whose branch is already here, naming whoever
// made it. Losing locally is the same answer as losing remotely and should read
// the same way.
func (s *session) branchIsFree(ctx context.Context, branch string, item *model.Item) error {
	_, err := s.git.RevParse(ctx, "refs/heads/"+branch)
	if err != nil {
		if errors.Is(err, gitx.ErrUnknownRevision) {
			return nil
		}

		return err
	}

	holder := "somebody"

	for _, claim := range item.Claims {
		if claim.Ref == "refs/heads/"+branch && claim.Claimant != "" {
			holder = claim.Claimant
		}
	}

	return fmt.Errorf("%s is already here and %s made it: %s has this one",
		branch, holder, holder)
}

// lostRace turns a rejected push into the sentence the loser needs.
//
// A rejected push on a branch is a lost race and nothing else — there is no ref
// namespace a remote might refuse, and so no policy failure to tell it apart
// from. Naming the holder costs one fetch and one log, and is worth it: "push
// rejected" tells somebody to retry, and "alice has this one" tells them to go
// and talk to alice.
func (s *session) lostRace(ctx context.Context, id, branch string, cause error) error {
	holder := s.claimHolder(ctx, branch)
	if holder == "" {
		return fmt.Errorf(
			"%s is already claimed: somebody pushed %s first, so the work is theirs: %w",
			id, branch, cause)
	}

	return fmt.Errorf("%s is already claimed: %s pushed %s first, so the work is theirs",
		id, holder, branch)
}

// claimHolder asks the remote who claimed a branch, and answers with nothing
// rather than a guess when it cannot tell.
func (s *session) claimHolder(ctx context.Context, branch string) string {
	remotes, err := s.git.Remotes(ctx)
	if err != nil || len(remotes) == 0 {
		return ""
	}

	const fetched = "refs/isu/contended"

	spec := "+refs/heads/" + branch + ":" + fetched
	if err := s.git.Fetch(ctx, remotes[0], spec); err != nil {
		return ""
	}

	defer func() { _ = s.git.DeleteRef(ctx, fetched) }()

	first, err := s.repo.LoadFirstCommits(ctx, s.trunk, []string{fetched})
	if err != nil {
		return ""
	}

	return first[fetched].Author.Name
}

func (a *app) unclaimCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "unclaim <id>",
		Short: "release a claim, keeping the branch",
		Long: `unclaim flips the issue's state back to open on isu/<id> and pushes.

It does not delete the branch. A one-word command must not throw away work:
what it releases is the claim, and what it leaves is an ordinary branch the
board says nothing about.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.unclaim(cmd, args[0])
		},
	}
}

func (a *app) unclaim(cmd *cobra.Command, id string) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	branch := claimBranchPrefix + id
	ref := "refs/heads/" + branch

	if _, err := s.git.RevParse(ctx, ref); err != nil {
		if errors.Is(err, gitx.ErrUnknownRevision) {
			return fmt.Errorf("there is no %s here, so there is no claim of yours to release", branch)
		}

		return err
	}

	claimed, err := s.readIssueAt(ctx, ref, id)
	if err != nil {
		return err
	}

	if claimed.State != issue.StateResolved {
		return fmt.Errorf("%s does not claim %s: its copy of the issue says %q, "+
			"and a branch that has not flipped the state is not a claim",
			branch, id, claimed.State)
	}

	claimed.State = issue.StateOpen

	commit, err := s.commitOn(ctx, commitSpec{
		branch:  branch,
		base:    ref,
		message: "unclaim " + id + "\n",
		changes: []change{{path: readmePath(id), blob: claimed.Encode()}},
	})
	if err != nil {
		return err
	}

	if err := s.push(ctx, branch); err != nil {
		return err
	}

	return a.reportWrite(Write{
		ID: id, Branch: branch, Commit: commit,
		Paths: []string{readmePath(id)}, Pushed: true,
	})
}
