package repo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// open is Open against a scripted repository.
func open(t *testing.T, r *gittest.Repo) *repo.Repo {
	t.Helper()

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	return loader
}

func TestLoadRefReadsTrunk(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq",
			gittest.Title("Login retries"),
			gittest.Type("bug"),
			gittest.Field("repro", "log in twice"),
			gittest.Priority("p1"),
			gittest.Body("Free-form markdown body.\n")).
		Issue("AR-40b1cc", gittest.Type("epic"), gittest.Without("state")).
		Commit("two issues")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Empty(t, set.Broken)
	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, set.IDs())

	bug, ok := set.Get("AR-7f3akq")
	require.True(t, ok)
	require.Equal(t, "Login retries", bug.Title)
	require.Equal(t, issue.TypeBug, bug.Type)
	require.Equal(t, issue.StateOpen, bug.State)
	require.Equal(t, issue.PriorityP1, bug.Priority)
	require.Equal(t, "Free-form markdown body.\n", bug.Body)
	require.Equal(t, "AR-7f3akq", bug.Folder,
		"the folder is what Validate holds the id against, so the loader sets it")
	require.NoError(t, bug.Validate())

	epic, ok := set.Get("AR-40b1cc")
	require.True(t, ok)
	require.Equal(t, issue.TypeEpic, epic.Type)
	require.Empty(t, epic.State)
	require.NoError(t, epic.Validate())
}

// The plan measures the mandated read path at 0.6 s where a git show per file
// takes 13.7 s. The difference is process count, so that is what is asserted:
// one ls-tree, one cat-file --batch, whatever the repository holds.
func TestLoadRefSpawnsExactlyTwoProcesses(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Issue("AR-40b1cc").Issue("AR-39ka2p").
		Commit("three issues")

	loader := open(t, r)

	_, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, int64(2), loader.Processes())
}

func TestLoadRefReadsABranchAndADetachedSHA(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")
	first := r.Head()

	r.Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
		Issue("AR-7f3akq", gittest.State("resolved")).
		Issue("AR-40b1cc").
		Commit("resolve AR-7f3akq and open AR-40b1cc").
		Checkout(gittest.DefaultBranch)

	loader := open(t, r)

	onBranch, err := loader.LoadRef(t.Context(), "isu/AR-7f3akq")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, onBranch.IDs())
	require.Equal(t, issue.StateResolved, mustGet(t, onBranch, "AR-7f3akq").State)

	detached, err := loader.LoadRef(t.Context(), first)
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, detached.IDs())
	require.Equal(t, issue.StateOpen, mustGet(t, detached, "AR-7f3akq").State,
		"a raw sha is a ref like any other")
}

// The status table in the plan turns on issues that are on one ref and not
// another — `awaiting triage` is exactly that — so the loader has to answer it
// without either ref contaminating the other.
func TestLoadRefKeepsRefsApart(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("report/AR-40b1cc").Checkout("report/AR-40b1cc").
		Issue("AR-40b1cc").Commit("report AR-40b1cc").
		Checkout(gittest.DefaultBranch)

	loader := open(t, r)

	trunk, err := loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, trunk.IDs())

	branch, err := loader.LoadRef(t.Context(), "report/AR-40b1cc")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, branch.IDs())
}

func TestLoadRefOnAnEmptyRepositoryIsAnEmptyBoard(t *testing.T) {
	set, err := open(t, gittest.New(t)).LoadRef(t.Context(), "HEAD")
	require.NoError(t, err)
	require.Empty(t, set.IDs())
	require.Empty(t, set.Broken)
}

func TestLoadRefOnARepositoryWithNoIssuesDirectory(t *testing.T) {
	r := gittest.New(t).File("README.md", "# a repository\n").Commit("no issues yet")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Empty(t, set.IDs())
}

// An unborn HEAD is an empty repository. A named ref that does not resolve is a
// mistake, and swallowing it would turn a typo into an empty board.
func TestLoadRefRefusesARefThatIsNotThere(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).LoadRef(t.Context(), "refs/heads/nope")
	require.ErrorContains(t, err, "nope")
}

// The cat-file --batch stream is length-prefixed, and a parser that scanned for
// the next header instead of counting bytes would read this issue's body as the
// start of another object. M2-S2 says to write this one first.
func TestLoadRefSurvivesAnIssueThatLooksLikeTwoObjects(t *testing.T) {
	trap := "0000000000000000000000000000000000000000 blob 42\n" +
		"---\nschema: 1\nid: NOT-AN-ISSUE\ntype: chore\n---\n"
	body := "The batch stream said:\n\n```\n" + trap + "```\n\nand then stopped.\n"

	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Body(body)).
		Issue("AR-40b1cc", gittest.Body("ordinary\n")).
		Commit("an issue that quotes a batch header")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, set.IDs())
	require.NotContains(t, set.IDs(), "NOT-AN-ISSUE")
	require.Equal(t, body, mustGet(t, set, "AR-7f3akq").Body,
		"the body arrives whole, trap and all")
}

// Two issues whose files happen to be byte-identical are one blob in git. The
// loader is fed object ids, so it must not lose the second path.
func TestLoadRefKeepsBothIssuesThatShareABlob(t *testing.T) {
	same := []gittest.IssueOption{
		gittest.Without("id"),
		gittest.Title("the same file twice"),
		gittest.Created("2026-08-24"),
	}

	r := gittest.New(t).
		Issue("AR-7f3akq", same...).
		Issue("AR-40b1cc", same...).
		Commit("one blob, two issues")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"AR-7f3akq", "AR-40b1cc"}, set.IDs())
}

func TestLoadRefIgnoresEverythingThatIsNotAnIssueReadme(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq",
			gittest.Attachment("repro.har", "{}\n"),
			gittest.Comment("2026-08-24-support-01.md", "Seen it too.\n")).
		File("issues/README.md", "# how this directory works\n").
		File("issues/AR-40b1cc/notes/deep.md", "not a README\n").
		File("docs/AR-39ka2p/README.md", "not under issues/\n").
		Commit("an issue and some decoys")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Empty(t, set.Broken)
}

// A half-written issue must not blind the board: the ones around it still load
// and the broken one is reported rather than thrown.
func TestLoadRefReportsAnIssueItCannotRead(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("issues/AR-40b1cc/README.md", "no frontmatter here\n").
		File("issues/AR-39ka2p/README.md", "---\nschema: 99\nid: AR-39ka2p\n---\n").
		Commit("one good issue and two bad ones")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Len(t, set.Broken, 2)

	broken := map[string]string{}
	for _, b := range set.Broken {
		broken[b.ID] = b.Err.Error()
		require.Equal(t, "issues/"+b.ID+"/README.md", b.Path)
	}

	require.Contains(t, broken["AR-40b1cc"], "must open with ---")
	require.Contains(t, broken["AR-39ka2p"], "version 99")
}

// A folder under issues/ whose name cannot be an id is somebody's mistake, not
// a file to skip. Skipping it makes the issue disappear from the board with
// nothing said.
func TestLoadRefReportsAFolderThatCannotBeAnID(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("issues/not an id/README.md", "---\nschema: 1\n---\n").
		Commit("a folder nobody can name")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.Len(t, set.Broken, 1)
	require.Equal(t, "not an id", set.Broken[0].ID)
	require.ErrorContains(t, set.Broken[0].Err, "cannot be an issue id")
	require.Contains(t, set.Broken[0].String(), "issues/not an id/README.md")
}

// Broken comes back in a stable order, or two runs of `isu check` on the same
// repository print their complaints in different orders.
func TestLoadRefSortsWhatItCouldNotRead(t *testing.T) {
	r := gittest.New(t)
	for _, id := range []string{"AR-zzzzzz", "AR-mmmmmm", "AR-aaaaaa"} {
		r.File("issues/"+id+"/README.md", "not an issue\n")
	}
	r.Commit("three bad issues")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	ids := make([]string, 0, len(set.Broken))
	for _, b := range set.Broken {
		ids = append(ids, b.ID)
	}

	require.Equal(t, []string{"AR-aaaaaa", "AR-mmmmmm", "AR-zzzzzz"}, ids)
}

// Decoding is not validating. `isu check` is what reports a bug with no repro;
// the loader's job is to hand it over so that something can.
func TestLoadRefDecodesAnIssueThatDoesNotValidate(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Type("bug"), gittest.Without("owner")).
		Commit("a bug with no repro and no owner")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Empty(t, set.Broken, "it parsed, so it is not broken; it is wrong")

	bad := mustGet(t, set, "AR-7f3akq")
	require.ErrorContains(t, bad.Validate(), "repro")
	require.ErrorContains(t, bad.Validate(), "owner")
}

// The folder name is the id. When the frontmatter disagrees the folder still
// wins as the key, because that is what every parent: and blocked_by: in the
// repository points at — and Validate is what says the two disagree.
func TestLoadRefKeysByFolderNotByTheIdField(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Field("id", "AR-40b1cc")).
		Commit("an issue that disagrees with its own folder")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, []string{"AR-7f3akq"}, set.IDs())
	require.ErrorContains(t, mustGet(t, set, "AR-7f3akq").Validate(), "must equal the folder name")
}

func mustGet(t *testing.T, set *repo.Set, id string) *issue.Issue {
	t.Helper()

	got, ok := set.Get(id)
	require.True(t, ok, "%s is not in the set", id)

	return got
}
