package gitx_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
)

func TestLsTreeReturnsObjectIdsAndPaths(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Attachment("repro.har", "{}\n")).
		Issue("AR-40b1cc").
		File("README.md", "# not an issue\n").
		Commit("two issues")

	entries, err := open(t, r).LsTree(t.Context(), "HEAD")
	require.NoError(t, err)

	byPath := map[string]gitx.TreeEntry{}
	for _, e := range entries {
		byPath[e.Path] = e
	}

	readme := byPath["issues/AR-7f3akq/README.md"]
	require.Equal(t, "blob", readme.Type)
	require.Len(t, readme.OID, 40, "the object id is what the fast read path is fed")
	require.Equal(t, "100644", readme.Mode)

	require.Contains(t, byPath, "issues/AR-7f3akq/repro.har")
	require.Contains(t, byPath, "issues/AR-40b1cc/README.md")
	require.Contains(t, byPath, "README.md")
}

func TestLsTreeLimitsItselfToThePathsItIsGiven(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").
		File("docs/guide.md", "hello\n").
		Commit("an issue and a doc")

	entries, err := open(t, r).LsTree(t.Context(), "HEAD", "issues")
	require.NoError(t, err)

	for _, e := range entries {
		require.True(t, strings.HasPrefix(e.Path, "issues/"), "found %s", e.Path)
	}
	require.NotEmpty(t, entries)
}

func TestLsTreeReportsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).LsTree(t.Context(), "refs/heads/nope")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision,
		"an unborn trunk and a typo are told apart by the caller, so the loader needs the kind")
}

func TestLsTreeOnAnUnbornHeadIsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t)

	_, err := open(t, r).LsTree(t.Context(), "HEAD")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

func TestCatFileBatchStreamsTheObjectsItIsFed(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Body("first\n")).
		Issue("AR-40b1cc", gittest.Body("second\n")).
		Commit("two issues")

	g := open(t, r)

	entries, err := g.LsTree(t.Context(), "HEAD", "issues")
	require.NoError(t, err)

	oids := make([]string, 0, len(entries))
	for _, e := range entries {
		oids = append(oids, e.OID)
	}

	got := map[string]string{}
	require.NoError(t, g.CatFileBatch(t.Context(), oids, func(o gitx.Object) error {
		got[o.OID] = string(o.Data)

		return nil
	}))

	require.Len(t, got, len(oids))
	for _, e := range entries {
		require.Contains(t, got[e.OID], "id: "+strings.Split(e.Path, "/")[1])
	}
}

// The batch stream is <oid> <type> <size> LF, then exactly size bytes, then LF.
// A parser that looks for the next header line instead of counting bytes reads
// a body that contains a header-shaped line as the start of the next object.
// This is the test PLAN.md says to write first.
func TestCatFileBatchSurvivesABlobContainingTheBatchDelimiter(t *testing.T) {
	trap := "89b3d5feacfa4b37139ad6e315fb43374a2bebda blob 117\n" +
		"---\nschema: 1\nid: NOT-AN-ISSUE\n---\n"
	body := "Here is what a naive parser reads as a header:\n" + trap + "and text after it\n"

	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Body(body)).
		Issue("AR-40b1cc", gittest.Body("plain\n")).
		Commit("a blob that lies about being two blobs")

	g := open(t, r)

	entries, err := g.LsTree(t.Context(), "HEAD", "issues")
	require.NoError(t, err)

	oids := make([]string, 0, len(entries))
	for _, e := range entries {
		oids = append(oids, e.OID)
	}

	var objects []gitx.Object
	require.NoError(t, g.CatFileBatch(t.Context(), oids, func(o gitx.Object) error {
		objects = append(objects, o)

		return nil
	}))

	require.Len(t, objects, 2, "two blobs went in, so two come out")

	for _, o := range objects {
		require.Equal(t, "blob", o.Type)
		require.Len(t, o.Data, int(o.Size), "the size in the header is the size of the object")
		if strings.Contains(string(o.Data), "id: AR-7f3akq") {
			require.Contains(t, string(o.Data), trap, "the body arrives whole")
			require.Contains(t, string(o.Data), "and text after it")
		}
	}
}

func TestCatFileBatchOnNothingSpawnsNothing(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")
	g := open(t, r)

	calls := 0
	require.NoError(t, g.CatFileBatch(t.Context(), nil, func(gitx.Object) error {
		calls++

		return nil
	}))

	require.Zero(t, calls)
	require.Zero(t, g.Processes(), "no objects is no work, not an empty git process")
}

func TestCatFileBatchReportsAMissingObject(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	err := open(t, r).CatFileBatch(t.Context(),
		[]string{"0000000000000000000000000000000000000000"},
		func(gitx.Object) error { return nil })

	require.ErrorContains(t, err, "missing")
}

func TestCatFileBatchStopsWhenTheCallbackFails(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Issue("AR-40b1cc").Issue("AR-39ka2p").
		Commit("three issues")

	g := open(t, r)

	entries, err := g.LsTree(t.Context(), "HEAD", "issues")
	require.NoError(t, err)

	oids := make([]string, 0, len(entries))
	for _, e := range entries {
		oids = append(oids, e.OID)
	}

	err = g.CatFileBatch(t.Context(), oids, func(gitx.Object) error {
		return errUnhappy
	})
	require.ErrorIs(t, err, errUnhappy)
}

func TestRevParseResolvesARevision(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	oid, err := open(t, r).RevParse(t.Context(), "HEAD")
	require.NoError(t, err)
	require.Equal(t, r.Head(), oid)
}

func TestRevParseReportsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).RevParse(t.Context(), "refs/heads/nope")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

func TestForEachRefListsBranchesAndClaims(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		Branch("isu/AR-7f3akq")

	// A claim is a parentless commit with an empty tree at refs/claims/<ID>.
	tree := r.Git("hash-object", "-t", "tree", "-w", "--stdin")
	claim := r.Git("commit-tree", tree, "-m", "claim AR-7f3akq")
	r.Git("update-ref", "refs/claims/AR-7f3akq", claim)

	refs, err := open(t, r).ForEachRef(t.Context(), "refs/heads/", "refs/claims/")
	require.NoError(t, err)

	byName := map[string]gitx.Ref{}
	for _, ref := range refs {
		byName[ref.Name] = ref
	}

	require.Contains(t, byName, "refs/heads/main")
	require.Contains(t, byName, "refs/heads/isu/AR-7f3akq")

	got := byName["refs/claims/AR-7f3akq"]
	require.Equal(t, claim, got.OID)
	require.Equal(t, "commit", got.Type)
}

func TestForEachRefOnAPatternThatMatchesNothing(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	refs, err := open(t, r).ForEachRef(t.Context(), "refs/claims/")
	require.NoError(t, err)
	require.Empty(t, refs, "no claims is an empty list, not a failure")
}

func TestForEachRefReportsABadPattern(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).ForEachRef(t.Context(), "--nope")
	require.Error(t, err)
}

func TestDiffNameOnlyListsTheFilesThatChanged(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq")
	base := r.Head()

	r.File("cmd/fix.go", "package cmd\n").
		Issue("AR-7f3akq", gittest.State("resolved")).
		Commit("resolve AR-7f3akq")

	names, err := open(t, r).DiffNameOnly(t.Context(), base, "HEAD")
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"cmd/fix.go", "issues/AR-7f3akq/README.md"}, names)

	// M5-S3's evidence check asks exactly this question: did anything outside
	// issues/ change?
	outside, err := open(t, r).DiffNameOnly(t.Context(), base, "HEAD", "cmd")
	require.NoError(t, err)
	require.Equal(t, []string{"cmd/fix.go"}, outside)
}

func TestDiffNameOnlyReportsAnUnknownRevision(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).DiffNameOnly(t.Context(), "refs/heads/nope", "HEAD")
	require.ErrorIs(t, err, gitx.ErrUnknownRevision)
}

func TestShowReturnsBlobBytes(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq", gittest.Body("the body\n")).
		Commit("add AR-7f3akq")

	data, err := open(t, r).Show(t.Context(), "HEAD:issues/AR-7f3akq/README.md")
	require.NoError(t, err)
	require.Equal(t, r.ReadFile("issues/AR-7f3akq/README.md"), string(data))
}

func TestShowReportsAnUnknownObject(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	_, err := open(t, r).Show(t.Context(), "HEAD:issues/AR-40b1cc/README.md")
	require.Error(t, err)
}

func TestPushSendsARefspec(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WithRemote()

	tree := r.Git("hash-object", "-t", "tree", "-w", "--stdin")
	claim := r.Git("commit-tree", tree, "-m", "claim AR-7f3akq")

	require.NoError(t, open(t, r).Push(t.Context(), "origin", claim+":refs/claims/AR-7f3akq"))

	require.Equal(t, claim,
		r.Git("ls-remote", "origin", "refs/claims/AR-7f3akq")[:40],
		"the claim reached the remote")
}

// A second claimant pushes a parentless commit at a ref that already has one.
// It can never be a fast-forward, so the push is rejected — which is what makes
// step 1 of `isu claim` a real compare-and-swap.
func TestPushReportsARejection(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WithRemote()

	tree := r.Git("hash-object", "-t", "tree", "-w", "--stdin")
	first := r.Git("commit-tree", tree, "-m", "claim AR-7f3akq")
	second := r.Git("commit-tree", tree, "-m", "claim AR-7f3akq again")

	g := open(t, r)
	require.NoError(t, g.Push(t.Context(), "origin", first+":refs/claims/AR-7f3akq"))

	err := g.Push(t.Context(), "origin", second+":refs/claims/AR-7f3akq")
	require.Error(t, err)
	require.ErrorContains(t, err, "refs/claims/AR-7f3akq",
		"the rejection names the ref, which is how M4-S4 tells a lost race from a "+
			"remote that refuses refs outside refs/heads")
}

func TestPushReportsAnUnreachableRemote(t *testing.T) {
	r := gittest.New(t).
		Issue("AR-7f3akq").Commit("add AR-7f3akq").
		WithRemote().DetachRemote()

	err := open(t, r).Push(t.Context(), "origin", "HEAD:refs/heads/main")
	require.Error(t, err)
}
