package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
)

// onBranch is a repository with one open issue and a branch checked out, which
// is where resolve and drop are meant to be run.
func onBranch(t *testing.T, opts ...gittest.IssueOption) *gittest.Repo {
	t.Helper()

	options := append([]gittest.IssueOption{
		gittest.Owner("dmitry"), gittest.Type("chore"), gittest.Title("Something to finish"),
	}, opts...)

	return configured(t).
		Issue("ISU-openly", options...).
		Commit("report ISU-openly").
		Branch("isu/ISU-openly").Checkout("isu/ISU-openly")
}

func TestResolveFlipsTheStateAndWritesTheTrailer(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.File("fix.go", "package fix\n").Commit("the actual fix")

	written := decode[Write](t, isu(t, r.Dir(), "--json", "resolve", "ISU-openly").ok(t))

	require.Equal(t, "isu/ISU-openly", written.Branch)
	require.NotEmpty(t, written.Commit)

	require.Contains(t, r.ReadFile("issues/ISU-openly/README.md"), "state: resolved")
	require.Equal(t, "resolve ISU-openly", r.Git("log", "-1", "--format=%s"))
	require.Contains(t, r.Git("log", "-1", "--format=%b"),
		model.ResolvesTrailer+": ISU-openly")
}

func TestTheOnlyWayToReachDoneIsAMergedPullRequest(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.File("fix.go", "package fix\n").Commit("the actual fix")

	isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	// Resolved on the branch is `in progress`: a branch is a proposal, and
	// trunk is where state is true.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, "in progress", payload.Issue.Status)

	r.Checkout(gittest.DefaultBranch).SquashMerge("isu/ISU-openly", "resolve ISU-openly")

	done := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, "done", done.Issue.Status)
}

func TestTheTrailerSurvivesASquashMerge(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.File("fix.go", "package fix\n").Commit("the actual fix")

	isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	body := r.Git("log", "-1", "--format=%b")

	r.Checkout(gittest.DefaultBranch).
		SquashMergeWithBody("isu/ISU-openly", "resolve ISU-openly", body)

	// The link from a trunk commit back to the issue it resolved is the one
	// thing file content cannot answer, so it has to survive the merge
	// strategy most teams use.
	ids, tier := model.Resolves("ISU",
		r.Git("log", "-1", "--format=%s"), r.Git("log", "-1", "--format=%b"))

	require.Equal(t, []string{"ISU-openly"}, ids)
	require.Equal(t, model.TierTrailer, tier)
}

func TestResolveOnAFreshlyClaimedIssueWritesOnlyTheTrailer(t *testing.T) {
	t.Parallel()

	r := claimable(t)

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)
	r.Checkout("isu/ISU-openly")

	before := r.ReadFile("issues/ISU-openly/README.md")

	isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	// The claim already wrote resolved, so what resolve adds is the link — and
	// the code beside it, which this branch deliberately does not have. A
	// branch that resolves an issue and changes nothing outside issues/ is
	// M5-S3's to reject, not this command's.
	require.Equal(t, before, r.ReadFile("issues/ISU-openly/README.md"))
	require.Contains(t, r.Git("log", "-1", "--format=%b"), model.ResolvesTrailer)
	require.Empty(t, r.Git("show", "--stat", "--format=", "HEAD"),
		"the commit changes no file at all, and the trailer is its whole payload")
}

func TestResolveOnASpikeWithNothingBesideItWarns(t *testing.T) {
	t.Parallel()

	r := onBranch(t, gittest.Type("spike"), gittest.Field("question", "how much?"))

	got := isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	require.Contains(t, got.stderr, "spike")
	require.Contains(t, got.stderr, "M5-S3",
		"a warning rather than a refusal: the check that fails a pull request is M5-S3's")
}

func TestResolveOnASpikeWithItsAnswerBesideItDoesNotWarn(t *testing.T) {
	t.Parallel()

	r := onBranch(t, gittest.Type("spike"), gittest.Field("question", "how much?"),
		gittest.Attachment("answer.md", "about 1.4 seconds\n"))

	got := isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	require.Empty(t, got.stderr)
}

func TestResolveAndDropRefuseToRunOnTrunk(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("On trunk")).
		Commit("report ISU-openly")

	for _, args := range [][]string{
		{"resolve", "ISU-openly"},
		{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 2, got.code)
		require.Contains(t, got.stderr, "you are on "+gittest.DefaultBranch)
	}
}

func TestResolveRefusesAnEpic(t *testing.T) {
	t.Parallel()

	r := onBranch(t, gittest.Type("epic"), gittest.Without("state"))

	got := isu(t, r.Dir(), "resolve", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "an epic has no state of its own")

	dropped := isu(t, r.Dir(), "drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix")

	require.Equal(t, 1, dropped.code)
	require.Contains(t, dropped.stderr, "an epic has no state of its own")
}

func TestResolveAnIssueTheWorkingTreeDoesNotHave(t *testing.T) {
	t.Parallel()

	r := onBranch(t)

	got := isu(t, r.Dir(), "resolve", "ISU-nobody")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "issues/ISU-nobody/README.md")
}

func TestDropWritesTheReasonAndTheResolution(t *testing.T) {
	t.Parallel()

	r := onBranch(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "drop", "ISU-openly",
		"--reason", "the platform does this for us now",
		"--resolution", "works-as-intended").ok(t))

	require.NotEmpty(t, written.Commit)

	file := r.ReadFile("issues/ISU-openly/README.md")
	require.Contains(t, file, "state: dropped")
	require.Contains(t, file, "reason: the platform does this for us now")
	require.Contains(t, file, "resolution: works-as-intended")

	require.Equal(t, "drop ISU-openly", r.Git("log", "-1", "--format=%s"))

	r.Checkout(gittest.DefaultBranch).SquashMerge("isu/ISU-openly", "drop ISU-openly")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))
	require.Equal(t, "dropped", payload.Issue.Status)
	require.Equal(t, "works-as-intended", payload.Issue.Resolution)
}

func TestDropWithoutAReasonOrAResolutionIsRefused(t *testing.T) {
	t.Parallel()

	r := onBranch(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no reason", []string{"--resolution", "wontfix"}, "--reason"},
		{"no resolution", []string{"--reason", "no"}, "--resolution"},
		{
			name: "a resolution that is not one",
			args: []string{"--reason", "no", "--resolution", "cannot-be-bothered"},
			want: "is not a resolution",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isu(t, r.Dir(), append([]string{"drop", "ISU-openly"}, tt.args...)...)

			require.Equal(t, 2, got.code)
			require.Contains(t, got.stderr, tt.want)
		})
	}
}

func TestNeitherResolveNorDropMergesAnything(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	trunk := r.Git("rev-parse", gittest.DefaultBranch)

	isu(t, r.Dir(), "resolve", "ISU-openly").ok(t)

	require.Equal(t, trunk, r.Git("rev-parse", gittest.DefaultBranch),
		"both leave a branch for a pull request and touch trunk not at all")
}

func TestResolveFromADetachedHEADIsAllowedAndSaysWhereItWent(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.Checkout(r.Head())

	written := decode[Write](t, isu(t, r.Dir(), "--json", "resolve", "ISU-openly").ok(t))

	// A detached HEAD is on no branch, so it cannot be on trunk — and the
	// commit is real even though nothing names it.
	require.Empty(t, written.Branch)
	require.NotEmpty(t, written.Commit)
}

func TestJoinReadsAsASentence(t *testing.T) {
	t.Parallel()

	require.Empty(t, join(nil))
	require.Equal(t, "one", join([]string{"one"}))
	require.Equal(t, "one or two", join([]string{"one", "two"}))
	require.Equal(t, "one, two or three", join([]string{"one", "two", "three"}))
}
