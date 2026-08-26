package gitx_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
)

// errUnhappy is what a test's own callback returns when the subject is what
// happens to a caller's error.
var errUnhappy = errors.New("the caller is unhappy")

func TestLogWalksTheCommitsOfARef(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Issue("AR-40b1cc").Commit("add AR-40b1cc")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "HEAD"})
	require.NoError(t, err)
	require.Len(t, commits, 2)

	require.Equal(t, "add AR-40b1cc", commits[0].Subject, "newest first, as git logs")
	require.Equal(t, "add AR-7f3akq", commits[1].Subject)
	require.Equal(t, r.Head(), commits[0].OID)
	require.Equal(t, []string{commits[1].OID}, commits[0].Parents)
	require.Empty(t, commits[1].Parents, "the root commit has none")
	require.Equal(t, "isu tester", commits[0].Author.Name)
	require.Equal(t, "tester@example.invalid", commits[0].Author.Email)
	require.False(t, commits[0].Author.When.IsZero())
}

func TestLogReversesWhenAsked(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("first").
		Issue("AR-40b1cc").Commit("second")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "HEAD", Reverse: true})
	require.NoError(t, err)
	require.Equal(t, "first", commits[0].Subject)
	require.Equal(t, "second", commits[1].Subject)
}

func TestLogCarriesTheBodySoATrailerCanBeRead(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq", gittest.State("resolved"))
	r.Git("commit", "--quiet", "-m", "resolve the login bug\n\nIsu-Resolves: AR-7f3akq\n")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "HEAD"})
	require.NoError(t, err)
	require.Equal(t, "resolve the login bug", commits[0].Subject)
	require.Contains(t, commits[0].Body, "Isu-Resolves: AR-7f3akq")
}

func TestLogRawCarriesTheObjectIdOfEveryChange(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")
	first := r.Head()

	r.Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{
		Rev: "HEAD", Raw: true, Reverse: true, Paths: []string{"issues"},
	})
	require.NoError(t, err)
	require.Len(t, commits, 2)

	added := commits[0]
	require.Equal(t, first, added.OID)
	require.Len(t, added.Changes, 1)
	require.Equal(t, "A", added.Changes[0].Status)
	require.Equal(t, "issues/AR-7f3akq/README.md", added.Changes[0].Path)
	require.Len(t, added.Changes[0].NewOID, 40, "the object id is what the batch read is fed")
	require.Equal(t, "0000000000000000000000000000000000000000", added.Changes[0].OldOID)

	modified := commits[1]
	require.Len(t, modified.Changes, 1)
	require.Equal(t, "M", modified.Changes[0].Status)
	require.Equal(t, added.Changes[0].NewOID, modified.Changes[0].OldOID,
		"what one commit wrote is what the next one changed")
	require.NotEqual(t, modified.Changes[0].OldOID, modified.Changes[0].NewOID)
}

// The state an issue holds "at trunk" changes at the commit that landed on
// trunk, which is the merge. Without --first-parent git reports the change at
// the *branch* commit instead: a merge shows no diff of its own, and under a
// pathspec history simplification drops the merge and promotes the branch
// commit that is TREESAME to it. A history index built that way would date
// every state change from a commit that was never on trunk.
func TestLogFirstParentAttributesAMergeToTrunkAndNotToTheBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq")
	onBranch := r.Head()

	r.Checkout(gittest.DefaultBranch).Merge("isu/AR-7f3akq")
	merge := r.Head()

	g := open(t, r)
	spec := gitx.LogSpec{Rev: "HEAD", Raw: true, Paths: []string{"issues"}}

	plain, err := g.Log(t.Context(), spec)
	require.NoError(t, err)
	require.Equal(t, onBranch, plain[0].OID,
		"without --first-parent the change is reported at the branch commit")

	spec.FirstParent = true

	firstParent, err := g.Log(t.Context(), spec)
	require.NoError(t, err)
	require.Len(t, firstParent, 2, "trunk's own timeline is two commits long")
	require.Equal(t, merge, firstParent[0].OID)
	require.Len(t, firstParent[0].Changes, 1,
		"the merge reports the change it brought in, which a merge shows only under --first-parent")
	require.Equal(t, "M", firstParent[0].Changes[0].Status)
	require.Equal(t, "issues/AR-7f3akq/README.md", firstParent[0].Changes[0].Path)
}

// Renames are deliberately not followed, so a moved issue folder is a delete
// and an add rather than one file with two names. See M2-S4.
func TestLogDoesNotDetectRenames(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")
	r.Git("mv", "issues/AR-7f3akq", "issues/AR-40b1cc")
	r.Commit("move the folder")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{
		Rev: "HEAD", Raw: true, Paths: []string{"issues"},
	})
	require.NoError(t, err)

	statuses := map[string]string{}
	for _, c := range commits[0].Changes {
		statuses[c.Path] = c.Status
	}

	require.Equal(t, map[string]string{
		"issues/AR-7f3akq/README.md": "D",
		"issues/AR-40b1cc/README.md": "A",
	}, statuses)

	for _, c := range commits[0].Changes {
		require.Equal(t, c.Path == "issues/AR-7f3akq/README.md", c.Deleted())
	}
}

func TestLogLimitsTheWalk(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("first").
		Issue("AR-40b1cc").Commit("second").
		Issue("AR-39ka2p").Commit("third")

	commits, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "HEAD", Limit: 2})
	require.NoError(t, err)
	require.Len(t, commits, 2)
	require.Equal(t, "third", commits[0].Subject)
}

func TestLogOnAnEmptyRepositoryIsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t)

	_, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "HEAD"})
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

func TestLogReportsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).Log(t.Context(), gitx.LogSpec{Rev: "refs/heads/nope"})
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}
