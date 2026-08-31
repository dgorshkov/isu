package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/issue"
)

func TestNewProducesAValidIssueOnABranchTrunkHasNeverSeen(t *testing.T) {
	t.Parallel()

	r := configured(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "new",
		"--title", "Login retries drop the second attempt",
		"--repro", "post twice and watch the second vanish").ok(t))

	require.True(t, strings.HasPrefix(written.ID, "ISU-"))
	require.Equal(t, "report/"+written.ID, written.Branch)
	require.NotEmpty(t, written.Commit)
	require.Equal(t, []string{"issues/" + written.ID + "/README.md"}, written.Paths)

	// The issue it wrote is one the schema accepts.
	folder, err := issue.Load(filepath.Join(r.Dir(), "issues", written.ID))
	require.NoError(t, err)
	require.NoError(t, folder.Issue.Validate())
	require.Equal(t, "Login retries drop the second attempt", folder.Issue.Title)
	require.Equal(t, issue.TypeBug, folder.Issue.Type)
	require.Equal(t, issue.StateOpen, folder.Issue.State)
	require.Equal(t, issue.PriorityP2, folder.Issue.Priority)
	require.Equal(t, today(), folder.Issue.Created.Format("2006-01-02"))

	// And the repository reads it as a report nobody has accepted yet.
	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", written.ID).ok(t))
	require.Equal(t, "awaiting triage", payload.Issue.Status)
	require.False(t, payload.Issue.OnTrunk)
}

func TestNewLeavesYouOnTheReportBranch(t *testing.T) {
	t.Parallel()

	r := configured(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "new",
		"--title", "A report", "--repro", "run it").ok(t))

	require.Equal(t, "report/"+written.ID, r.Git("rev-parse", "--abbrev-ref", "HEAD"))
}

func TestNewWithoutABranchWritesIntoTheWorkingTree(t *testing.T) {
	t.Parallel()

	r := configured(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "new", "--no-branch",
		"--title", "A bulk conversion writes many of these",
		"--repro", "import them").ok(t))

	require.Empty(t, written.Branch)
	require.Empty(t, written.Commit)

	require.Equal(t, gittest.DefaultBranch, r.Git("rev-parse", "--abbrev-ref", "HEAD"),
		"--no-branch leaves the repository on the branch it started on")
	require.NotEmpty(t, r.Git("status", "--porcelain"),
		"the issue is staged, waiting for the one commit a bulk conversion makes")
}

func TestNewWithoutATitleIsRefused(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "new")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "--title")
}

func TestNewTwiceInTheSameSecondProducesDifferentIDs(t *testing.T) {
	t.Parallel()

	r := configured(t)

	first := decode[Write](t, isu(t, r.Dir(), "--json", "new", "--no-branch",
		"--title", "The same title", "--repro", "the same repro").ok(t))
	second := decode[Write](t, isu(t, r.Dir(), "--json", "new", "--no-branch",
		"--title", "The same title", "--repro", "the same repro").ok(t))

	// Same title, same owner, same day: everything the hash reads except the
	// eight bytes of randomness that are there for exactly this.
	require.NotEqual(t, first.ID, second.ID)
}

func TestNewRegeneratesAnIDThatCollidesWithOneItCanSee(t *testing.T) {
	t.Parallel()

	// The id the generator will produce cannot be predicted — that is the
	// point of it — so the collision is seeded from the other side: the first
	// id it offers is declared taken, and the answer has to be a later one.
	s := openSession(t, configured(t).Dir())

	var offered []string

	id, err := s.allocate(func(id string) bool {
		offered = append(offered, id)

		return len(offered) == 1
	}, draftFor("Collides"))

	require.NoError(t, err)
	require.Len(t, offered, 2, "the collision is regenerated, not returned")
	require.Equal(t, offered[1], id)
	require.NotEqual(t, offered[0], id)
}

func TestNewRefusesToRunOutOfIDs(t *testing.T) {
	t.Parallel()

	// Every id is taken, so the loop gives up rather than spinning: eight
	// collisions in a row is not a collision, it is a random source that has
	// stopped being random.
	s := openSession(t, configured(t).Dir())

	_, err := s.allocate(func(string) bool { return true }, draftFor("Always taken"))

	require.ErrorContains(t, err, "stopped being random")
}

func draftFor(title string) *issue.Issue {
	return &issue.Issue{Title: title, Owner: "dmitry", Created: now()}
}

func TestNewEpicHasNoStateAndEveryOtherTypeMustHaveOne(t *testing.T) {
	t.Parallel()

	r := configured(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "new", "--no-branch",
		"--type", "epic", "--title", "Make login reliable").ok(t))

	folder, err := issue.Load(filepath.Join(r.Dir(), "issues", written.ID))
	require.NoError(t, err)
	require.Empty(t, folder.Issue.State, "an epic must not declare a state")
	require.NoError(t, folder.Issue.Validate())
	require.NotContains(t, r.ReadFile("issues/"+written.ID+"/README.md"), "state:")
}

func TestNewRefusesATypeWhoseRequiredFieldIsMissing(t *testing.T) {
	t.Parallel()

	r := configured(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"a bug needs a repro", []string{"--type", "bug"}, "repro"},
		{"a story needs acceptance", []string{"--type", "story"}, "acceptance"},
		{"a spike needs a question", []string{"--type", "spike"}, "question"},
		{"and the type has to be one", []string{"--type", "saga", "--repro", "x"}, "is not a type"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isu(t, r.Dir(), append([]string{"new", "--no-branch",
				"--title", "Needs a field"}, tt.args...)...)

			require.Equal(t, 2, got.code)
			require.Contains(t, got.stderr, tt.want)
		})
	}
}

func TestNewChoreNeedsNothingBeyondATitle(t *testing.T) {
	t.Parallel()

	r := configured(t)

	isu(t, r.Dir(), "new", "--no-branch", "--type", "chore", "--title", "Tidy up").ok(t)
}

func TestNewParentMustNameAnEpic(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "new", "--no-branch", "--title", "Belongs somewhere",
		"--repro", "x", "--parent", "ISU-openly")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "is not an epic")

	missing := isu(t, r.Dir(), "new", "--no-branch", "--title", "Belongs nowhere",
		"--repro", "x", "--parent", "ISU-nobody")

	require.Equal(t, 2, missing.code)
	require.Contains(t, missing.stderr, "no issue ISU-nobody")

	// And the epic itself is accepted.
	isu(t, r.Dir(), "new", "--no-branch", "--title", "Belongs to the epic",
		"--repro", "x", "--parent", "ISU-epical").ok(t)
}

func TestNewTakesEveryFieldItOffers(t *testing.T) {
	t.Parallel()

	r := board(t)

	written := decode[Write](t, isu(t, r.Dir(), "--json", "new", "--no-branch",
		"--title", "Everything at once",
		"--type", "story",
		"--acceptance", "it all round-trips",
		"--owner", "someone-else",
		"--priority", "p0",
		"--parent", "ISU-epical",
		"--blocked-by", "ISU-openly,ISU-reopen",
		"--body", "A paragraph.\n").ok(t))

	folder, err := issue.Load(filepath.Join(r.Dir(), "issues", written.ID))
	require.NoError(t, err)

	got := folder.Issue
	require.Equal(t, issue.TypeStory, got.Type)
	require.Equal(t, "it all round-trips", got.Acceptance)
	require.Equal(t, "someone-else", got.Owner)
	require.Equal(t, issue.PriorityP0, got.Priority)
	require.Equal(t, "ISU-epical", got.Parent)
	require.Equal(t, []string{"ISU-openly", "ISU-reopen"}, got.BlockedBy)
	require.Equal(t, "A paragraph.\n", got.Body)
}

func TestNewNeedsAnOwnerFromSomewhere(t *testing.T) {
	// Not parallel: taking the identity away means taking away the global and
	// system configuration too, and that is process-wide.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	r := configured(t)
	r.Git("config", "--unset", "user.name")

	got := isu(t, r.Dir(), "new", "--no-branch", "--title", "Who owns this?", "--repro", "x")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "--owner")

	// Named explicitly, it is fine.
	isu(t, r.Dir(), "new", "--no-branch", "--title", "Who owns this?",
		"--repro", "x", "--owner", "dmitry").ok(t)
}

func openSession(t *testing.T, dir string) *session {
	t.Helper()

	a := &app{env: Env{Dir: dir, Now: now, Getenv: func(string) string { return "" }}}

	s, err := a.open(t.Context())
	require.NoError(t, err)

	return s
}
