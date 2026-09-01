package repo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// A pipeline checks out a merge commit and no branch, so the ref under review
// is HEAD and refs/heads/* is empty. A board that could not be told to read one
// more ref would have nothing to say about the pull request it was run for.
func TestLoadBoardReadsRefsThePatternsDoNotName(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		Issue("ISU-40b1cc").Commit("report ISU-40b1cc")

	at := r.Head()
	r.Checkout(gittest.DefaultBranch).DeleteBranch("topic")
	r.Git("checkout", "--quiet", "--detach", at)

	loader := open(t, r)

	without, err := loader.LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)
	require.Empty(t, without.Names())

	with, err := loader.LoadBoard(t.Context(), repo.BoardSpec{
		Trunk: gittest.DefaultBranch, Refs: []string{"HEAD", ""},
	})
	require.NoError(t, err)

	require.Equal(t, []string{"HEAD"}, with.Names(), "an empty name is ignored")
	require.Equal(t, []string{"ISU-40b1cc"}, with.Changed["HEAD"])
}

// A ref the patterns already matched is not loaded twice. Loading it twice
// would put a claimant's own branch on the board as two branches, which reads
// as somebody contending with themselves.
func TestLoadBoardDoesNotLoadANamedRefTwice(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		Issue("ISU-7f3akq", gittest.State("resolved")).Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{
		Trunk: gittest.DefaultBranch, Refs: []string{"refs/heads/topic"},
	})
	require.NoError(t, err)

	require.Equal(t, []string{"refs/heads/topic"}, board.Names())
}

func TestLoadBoardReadsTrunkAndEveryBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Issue("AR-40b1cc").Commit("two issues").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
		Checkout(gittest.DefaultBranch).
		Branch("report/AR-39ka2p").Checkout("report/AR-39ka2p").
		Issue("AR-39ka2p").Commit("report AR-39ka2p").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, board.Trunk.IDs())
	require.Equal(t,
		[]string{"refs/heads/isu/AR-7f3akq", "refs/heads/report/AR-39ka2p"},
		board.Names(), "trunk is not one of the other refs")

	// `in progress`: some branch has state: resolved where trunk has open.
	onBranch := board.Refs["refs/heads/isu/AR-7f3akq"]
	require.Equal(t, issue.StateResolved, mustGet(t, onBranch, "AR-7f3akq").State)
	require.Equal(t, issue.StateOpen, mustGet(t, board.Trunk, "AR-7f3akq").State)

	// `awaiting triage`: the folder exists on a branch and not on trunk.
	require.Contains(t, board.Refs["refs/heads/report/AR-39ka2p"].IDs(), "AR-39ka2p")
	require.NotContains(t, board.Trunk.IDs(), "AR-39ka2p")
}

func TestLoadBoardDefaultsToHeadAndEveryBranch(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq")

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{})
	require.NoError(t, err)

	require.Equal(t, []string{"AR-7f3akq"}, board.Trunk.IDs())
	require.Equal(t, []string{"refs/heads/isu/AR-7f3akq", "refs/heads/main"}, board.Names(),
		"HEAD is not a ref name, so nothing here matches it and main is loaded twice over")
}

// A branch whose file is byte-identical to trunk's is the same blob in git, and
// the board holds one decoded issue for it. That is what keeps two hundred
// branches from being a million issue files in memory.
func TestLoadBoardSharesAnIssueAcrossRefsThatShareTheFile(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Issue("AR-40b1cc").Commit("two issues").
		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
		Checkout(gittest.DefaultBranch)

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	onBranch := board.Refs["refs/heads/isu/AR-7f3akq"]

	untouched, _ := board.Trunk.Get("AR-40b1cc")
	sameOnBranch, _ := onBranch.Get("AR-40b1cc")
	require.Same(t, untouched, sameOnBranch, "the file did not change, so neither did the issue")

	changed, _ := board.Trunk.Get("AR-7f3akq")
	changedOnBranch, _ := onBranch.Get("AR-7f3akq")
	require.NotSame(t, changed, changedOnBranch)
}

func TestLoadBoardOnAnEmptyRepository(t *testing.T) {
	board, err := open(t, gittest.New(t)).LoadBoard(t.Context(), repo.BoardSpec{})
	require.NoError(t, err)

	require.Empty(t, board.Trunk.IDs())
	require.Empty(t, board.Names())
}

func TestLoadBoardReportsWhatItCouldNotRead(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("issues/AR-40b1cc/README.md", "no frontmatter here\n").
		Commit("one good issue and one bad").
		Branch("isu/AR-7f3akq")

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	require.Equal(t, []string{"AR-7f3akq"}, board.Trunk.IDs())
	require.Len(t, board.Trunk.Broken, 1)
	require.Equal(t, "AR-40b1cc", board.Trunk.Broken[0].ID)

	// The same unreadable blob at a second ref is reported there too, from the
	// cache rather than by parsing it again.
	require.Len(t, board.Refs["refs/heads/isu/AR-7f3akq"].Broken, 1)
}

func TestLoadBoardReportsAFolderThatCannotBeAnID(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("issues/not an id/README.md", "---\nschema: 1\n---\n").
		Commit("a folder nobody can name")

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	require.Len(t, board.Trunk.Broken, 1)
	require.ErrorContains(t, board.Trunk.Broken[0].Err, "cannot be an issue id")
}

func TestLoadBoardRefusesATrunkThatIsNotThere(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{Trunk: "refs/heads/nope"})
	require.ErrorContains(t, err, "nope")
}

func TestLoadBoardTakesThePatternsItIsGiven(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq").
		Branch("report/AR-40b1cc")

	board, err := open(t, r).LoadBoard(t.Context(), repo.BoardSpec{
		Trunk:    gittest.DefaultBranch,
		Patterns: []string{"refs/heads/isu/"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"refs/heads/isu/AR-7f3akq"}, board.Names())
}
