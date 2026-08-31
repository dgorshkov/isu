package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// triageable is a repository with a report and an epic to put it under.
func triageable(t *testing.T, config string) *gittest.Repo {
	t.Helper()

	return gittest.New(t).
		File(".isu.yml", config).
		File("README.md", "# a repository\n").
		Commit("set isu up").
		Issue("ISU-epical", gittest.Type("epic"), gittest.Without("state"),
			gittest.Owner("dmitry"), gittest.Title("Make login reliable")).
		Issue("ISU-blockr", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Something in the way")).
		Issue("ISU-triage", gittest.Owner("support"), gittest.Type("bug"),
			gittest.Field("repro", "sign up on Firefox"),
			gittest.Title("Sign-up page 500s on Firefox"),
			gittest.Field("unknown_key", "kept verbatim"),
			gittest.Body("A paragraph nobody triaging should touch.\n")).
		Commit("a report and an epic")
}

func TestTriageRoundTripsEveryFieldOntoABranch(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	written := decode[Write](t, isu(t, r.Dir(), "--json", "triage", "ISU-triage",
		"--owner", "dmitry",
		"--parent", "ISU-epical",
		"--blocked-by", "ISU-blockr",
		"--priority", "p0").ok(t))

	require.Equal(t, "triage/ISU-triage", written.Branch)
	require.NotEmpty(t, written.Commit)
	require.False(t, written.Pushed)

	// Branch-and-pull-request, like everything else here: trunk is untouched
	// and the working tree never moved.
	require.Equal(t, gittest.DefaultBranch, r.Git("rev-parse", "--abbrev-ref", "HEAD"))
	require.Contains(t, r.ReadFile("issues/ISU-triage/README.md"), "owner: support")

	payload := decode[ShowPayload](t,
		isu(t, r.Dir(), "--json", "--ref", "triage/ISU-triage", "show", "ISU-triage").ok(t))

	require.Equal(t, "dmitry", payload.Issue.Owner)
	require.Equal(t, "ISU-epical", payload.Issue.Parent)
	require.Equal(t, []string{"ISU-blockr"}, payload.Issue.BlockedBy)
	require.Equal(t, "p0", payload.Issue.Priority)
}

func TestTriageLeavesTheBodyAndUnknownKeysByteForByte(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")
	before := r.ReadFile("issues/ISU-triage/README.md")

	isu(t, r.Dir(), "triage", "ISU-triage", "--priority", "p1").ok(t)

	after := r.Git("show", "triage/ISU-triage:issues/ISU-triage/README.md")

	// One line changed, and nothing else: not the body, not the key isu has
	// never heard of, not the order the author wrote their frontmatter in.
	require.Contains(t, after, "unknown_key: kept verbatim")
	require.Contains(t, after, "A paragraph nobody triaging should touch.")

	// One line added and not one line rewritten: take the new line back out
	// and what is left is the file that went in, byte for byte.
	require.Equal(t, strings.TrimRight(before, "\n"), withoutLine(after, "priority: p1"),
		"triage writes the line it was asked to and leaves every other one alone")
}

func TestTriageWithNothingToSetIsRefused(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-triage")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "needs something to set")
}

func TestTriageRefusesAPriorityThatIsNotOne(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--priority", "urgent")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "is not a priority")
	require.Contains(t, got.stderr, "p0, p1, p2 or p3")
}

func TestTriageRefusesAParentThatIsNotAnEpicAtTheCommand(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--parent", "ISU-blockr")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "is not an epic",
		"refused at the command rather than left for CI")
}

func TestTriageOfAnIssueNobodyHasIsRefused(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-nobody", "--owner", "dmitry")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "no issue ISU-nobody")
}

func TestPushWithoutTheConfigFlagIsRefusedAndNamesIt(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry", "--push")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "direct_triage")
	require.Contains(t, got.stderr, ".isu.yml")

	require.Equal(t, "a report and an epic", r.Git("log", "-1", "--format=%s"),
		"and it wrote nothing")
}

func TestPushWithTheConfigFlagProducesExactlyOneTrunkCommit(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")
	before := r.Git("rev-list", "--count", gittest.DefaultBranch)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "triage", "ISU-triage",
		"--owner", "dmitry", "--priority", "p0", "--push").ok(t))

	require.Equal(t, gittest.DefaultBranch, written.Branch)
	require.False(t, written.Pushed, "there is no remote here to reach")

	after := r.Git("rev-list", "--count", gittest.DefaultBranch)
	require.Equal(t, atoi(t, before)+1, atoi(t, after),
		"exactly one trunk commit: triage is not a code review, but it is still one commit")

	// Six reports filed on a Friday do not have to wait for Monday's queue.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-triage").ok(t))
	require.Equal(t, "dmitry", payload.Issue.Owner)
	require.Equal(t, "p0", payload.Issue.Priority)
	require.Equal(t, "open", payload.Issue.Status, "triage sets everything but the state")
}

func TestPushSendsTrunkOnWhenThereIsARemote(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n").WithRemote()

	written := decode[Write](t, isu(t, r.Dir(), "--json", "triage", "ISU-triage",
		"--owner", "dmitry", "--push").ok(t))

	require.True(t, written.Pushed)
	require.Equal(t, r.Git("rev-parse", gittest.DefaultBranch),
		r.Git("rev-parse", "refs/remotes/origin/"+gittest.DefaultBranch))
}

func TestPushWritesThroughTheWorkingTreeWhenTrunkIsCheckedOut(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")

	isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry", "--push").ok(t)

	// Moving trunk out from under a checkout would leave the user looking at a
	// file that is no longer what their branch says.
	require.Contains(t, r.ReadFile("issues/ISU-triage/README.md"), "owner: dmitry")
	require.Empty(t, r.Git("status", "--porcelain"))
}

func TestPushFromSomewhereThatIsNotTrunkStillWritesTrunk(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")
	r.Branch("side").Checkout("side")

	isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry", "--push").ok(t)

	require.Equal(t, "side", r.Git("rev-parse", "--abbrev-ref", "HEAD"))
	require.Contains(t, r.Git("show", gittest.DefaultBranch+":issues/ISU-triage/README.md"),
		"owner: dmitry")
	require.NotContains(t, r.ReadFile("issues/ISU-triage/README.md"), "owner: dmitry",
		"the branch the user is on is not the one triage was told to write")
}

func TestPushWhenTrunkIsNotALocalBranchSaysSo(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")
	head := r.Head()

	got := isu(t, r.Dir(), "--ref", head, "triage", "ISU-triage", "--owner", "dmitry", "--push")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "not a local branch")
}

func TestTriageRefusesToWriteAnIssueItWouldInvalidate(t *testing.T) {
	t.Parallel()

	// blocked_by is checked for shape by Validate, and triage is the command
	// that could put a bad one there.
	r := triageable(t, "prefix: ISU\n")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--blocked-by", "not/an/id")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "blocked_by")
}

func TestTriageSummaryNamesWhatChanged(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	isu(t, r.Dir(), "triage", "ISU-triage",
		"--owner", "dmitry", "--parent", "ISU-epical",
		"--priority", "p1", "--blocked-by", "ISU-blockr").ok(t)

	body := r.Git("log", "-1", "--format=%b", "triage/ISU-triage")

	require.Contains(t, body, "owner: dmitry")
	require.Contains(t, body, "parent: ISU-epical")
	require.Contains(t, body, "priority: p1")
	require.Contains(t, body, "blocked_by: ISU-blockr")
}

// withoutLine is text with one line taken out, which is how a test says "the
// only difference is that this line was added".
func withoutLine(text, drop string) string {
	var kept []string

	for line := range strings.SplitSeq(text, "\n") {
		if strings.TrimSpace(line) == drop {
			continue
		}

		kept = append(kept, line)
	}

	return strings.Join(kept, "\n")
}

func atoi(t *testing.T, s string) int {
	t.Helper()

	n := 0

	for _, c := range s {
		require.True(t, c >= '0' && c <= '9', "%q is not a number", s)
		n = n*10 + int(c-'0')
	}

	return n
}

func TestTriageOfAReportThatIsOnlyOnABranch(t *testing.T) {
	t.Parallel()

	// The case triage exists for. `awaiting triage` is defined as a folder
	// trunk has never seen, so reading the issue at trunk would make the
	// command fail on precisely the issues it was built to answer.
	r := triageable(t, "prefix: ISU\n")
	r.Branch("report/ISU-report").Checkout("report/ISU-report").
		Issue("ISU-report", gittest.Owner("support"), gittest.Type("bug"),
			gittest.Field("repro", "it 500s"), gittest.Title("Filed on Friday")).
		Commit("report ISU-report").
		Checkout(gittest.DefaultBranch)

	require.Equal(t, "awaiting triage", statusOf(t, r.Dir(), "ISU-report"))

	isu(t, r.Dir(), "triage", "ISU-report", "--owner", "dmitry", "--priority", "p0").ok(t)

	payload := decode[ShowPayload](t,
		isu(t, r.Dir(), "--json", "--ref", "triage/ISU-report", "show", "ISU-report").ok(t))

	require.Equal(t, "dmitry", payload.Issue.Owner)
	require.Equal(t, "p0", payload.Issue.Priority)
	require.Equal(t, "Filed on Friday", payload.Issue.Title, "and the report itself came with it")
}

func TestPushAcceptsAReportOntoTrunk(t *testing.T) {
	t.Parallel()

	// Six reports filed on a Friday should not wait for Monday's review queue.
	r := triageable(t, "prefix: ISU\ndirect_triage: true\n")
	r.Branch("report/ISU-report").Checkout("report/ISU-report").
		Issue("ISU-report", gittest.Owner("support"), gittest.Type("bug"),
			gittest.Field("repro", "it 500s"), gittest.Title("Filed on Friday")).
		Commit("report ISU-report").
		Checkout(gittest.DefaultBranch)

	isu(t, r.Dir(), "triage", "ISU-report", "--owner", "dmitry", "--push").ok(t)

	require.Equal(t, "open", statusOf(t, r.Dir(), "ISU-report"),
		"accepting a report is putting it on trunk")
	require.Contains(t, r.ReadFile("issues/ISU-report/README.md"), "owner: dmitry")
}

func statusOf(t *testing.T, dir, id string) string {
	t.Helper()

	return decode[ShowPayload](t, isu(t, dir, "--json", "show", id).ok(t)).Issue.Status
}
