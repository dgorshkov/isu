package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
)

func TestBoardRendersEveryStatus(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "board").ok(t)

	golden(t, "board/every-status.txt", got.stdout)

	// The golden file is the review; these are the claims it is reviewed
	// against, so that a careless -update cannot quietly delete a status.
	for _, status := range model.Statuses {
		require.Containsf(t, got.stdout, string(status),
			"a repository holding an issue in every status must render every status")
	}
}

func TestBoardJSONRoundTripsThroughTheDocumentedStruct(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Equal(t, gittest.DefaultBranch, payload.Trunk)
	require.Equal(t,
		[]string{"refs/heads/isu/ISU-inprog", "refs/heads/report/ISU-triage"},
		payload.Refs)

	byStatus := map[string][]string{}
	for _, group := range payload.Groups {
		for _, item := range group.Issues {
			byStatus[group.Status] = append(byStatus[group.Status], item.ID)
		}
	}

	require.Equal(t, map[string][]string{
		"done":            {"ISU-donede"},
		"dropped":         {"ISU-dropit"},
		"awaiting triage": {"ISU-triage"},
		"in progress":     {"ISU-inprog"},
		"reopened":        {"ISU-reopen"},
		"open":            {"ISU-openly", "ISU-epical"},
	}, byStatus)
}

func TestBoardGroupsAreInPrecedenceOrder(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	var got []model.Status
	for _, group := range payload.Groups {
		got = append(got, model.Status(group.Status))
	}

	require.Equal(t, model.Statuses, got,
		"the statuses overlap and the first match wins, so the groups are that order")
}

func TestBoardCarriesTheAnnotationsThatSurviveTheStatus(t *testing.T) {
	t.Parallel()

	r := board(t)

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	byID := map[string]Issue{}
	for _, group := range payload.Groups {
		for _, item := range group.Issues {
			byID[item.ID] = item
		}
	}

	require.True(t, byID["ISU-reopen"].Reopened,
		"trunk resolved it once and says open now")

	claimed := byID["ISU-inprog"]
	require.Len(t, claimed.Claims, 1)
	require.Equal(t, "alice", claimed.Claims[0].Claimant)
	require.Equal(t, "refs/heads/isu/ISU-inprog", claimed.Claims[0].Ref)
	require.False(t, claimed.Contended)

	epic := byID["ISU-epical"]
	require.NotNil(t, epic.Epic)
	require.Equal(t, []string{"ISU-openly"}, epic.Epic.Children)
}

func TestBoardOrdersByPriorityThenAge(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-oldp2", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Created("2020-01-01"), gittest.Title("An old p2")).
		Issue("ISU-newp0", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Created("2026-08-30"), gittest.Priority("p0"), gittest.Title("A new p0")).
		Issue("ISU-midp2", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Created("2024-01-01"), gittest.Title("A middling p2")).
		Commit("three issues")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Len(t, payload.Groups, 1)

	var order []string
	for _, item := range payload.Groups[0].Issues {
		order = append(order, item.ID)
	}

	require.Equal(t, []string{"ISU-newp0", "ISU-oldp2", "ISU-midp2"}, order,
		"most urgent first, then oldest first")
}

func TestBoardOnAnEmptyRepositorySaysSo(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "board").ok(t)

	require.Contains(t, got.stdout, "nothing to show")
}

func TestBoardRendersAnIssueTrunkCannotDecode(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-broken/README.md", "this file has no frontmatter at all\n").
		Commit("write a half-written issue")

	got := isu(t, r.Dir(), "board").ok(t)

	// A half-written issue must not blind the whole board, least of all for the
	// one issue somebody most needs to hear about.
	require.Contains(t, got.stdout, "ISU-broken")
	require.Contains(t, got.stdout, "unreadable")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))
	require.NotNil(t, payload.Groups[0].Issues[0].Broken)
	require.Equal(t, "issues/ISU-broken/README.md", payload.Groups[0].Issues[0].Broken.Path)
}

func TestBoardOnARefOtherThanTrunk(t *testing.T) {
	t.Parallel()

	r := board(t)

	// Read the branch as though it were trunk: the report on it is then a
	// folder trunk has, and reads open rather than awaiting triage.
	payload := decode[BoardPayload](t,
		isu(t, r.Dir(), "--json", "--ref", "report/ISU-triage", "board").ok(t))

	require.Equal(t, "report/ISU-triage", payload.Trunk)

	for _, group := range payload.Groups {
		for _, item := range group.Issues {
			if item.ID == "ISU-triage" {
				require.Equal(t, "open", item.Status)
			}
		}
	}
}

func TestFreshnessWarnsPastFetchWarnHoursAndDoesNotBefore(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		daysAgo  int
		wantWarn bool
		want     string
	}{
		{name: "fetched three days ago", daysAgo: 3, wantWarn: true, want: "3d ago"},
		{name: "freshly fetched", daysAgo: 0, wantWarn: false, want: "just now"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// What makes a fetch old is the date on the ref it wrote, so the
			// fixture's last commit is dated and then pushed. WithRemote
			// pushes the current branch and sets the remote-tracking ref,
			// which is what freshness reads.
			r := configured(t).
				Backdate(tt.daysAgo).
				File("something.txt", "to push\n").
				Commit("something to push").
				WithRemote()

			got := isu(t, r.Dir(), "board").ok(t)
			payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

			require.True(t, payload.Freshness.Remote)
			require.Equal(t, tt.wantWarn, payload.Freshness.Warn)
			require.Contains(t, got.stdout, tt.want)

			if tt.wantWarn {
				require.Contains(t, got.stdout, "--fetch")
			} else {
				require.NotContains(t, got.stdout, "older than")
			}
		})
	}
}

func TestFreshnessInARepositoryWithNoRemoteRefs(t *testing.T) {
	t.Parallel()

	r := board(t)

	got := isu(t, r.Dir(), "board").ok(t)
	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.False(t, payload.Freshness.Remote)
	require.Empty(t, payload.Freshness.Newest)
	require.Contains(t, got.stdout, "no remote refs")
}

func TestBoardNamesTheBranchWhenTrunkIsHEAD(t *testing.T) {
	t.Parallel()

	// A repository whose trunk is called neither main nor master falls back to
	// HEAD, and printing "HEAD" at the top of a board tells nobody which branch
	// they are looking at.
	r := configured(t)
	r.Git("branch", "-m", gittest.DefaultBranch, "trunk")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Equal(t, "trunk", payload.Trunk)
}

func TestBoardFromADetachedHEADSaysSo(t *testing.T) {
	t.Parallel()

	r := configured(t)
	r.Git("branch", "-m", gittest.DefaultBranch, "trunk")
	r.Checkout(r.Head())

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Equal(t, "HEAD (detached)", payload.Trunk)
}

func TestTrunkIsMasterWhenThatIsWhatItIsCalled(t *testing.T) {
	t.Parallel()

	r := configured(t)
	r.Git("branch", "-m", gittest.DefaultBranch, "master")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Equal(t, "master", payload.Trunk)
}

func TestColourIsOffForAPipeAndOnForATerminal(t *testing.T) {
	t.Parallel()

	// The streams every test in this package writes to are buffers, which is
	// why every golden file here is plain text — the same reason a redirect
	// gets no escape sequences.
	require.False(t, isCharDevice(&testWriter{}))

	device, err := osOpenDevNull()
	require.NoError(t, err)

	defer func() { _ = device.Close() }()

	require.True(t, isCharDevice(device),
		"a character device is the cheap half of asking whether something is a terminal")

	on := theme{color: true}
	require.Equal(t, ansiBold+"x"+ansiReset, on.bold("x"))
	require.Equal(t, ansiDim+"x"+ansiReset, on.dim("x"))
	require.Empty(t, on.bold(""), "there is nothing to colour in nothing")

	off := theme{}
	require.Equal(t, "x", off.bold("x"))
}

func TestNoColourWhenTheEnvironmentSaysNot(t *testing.T) {
	t.Parallel()

	device, err := osOpenDevNull()
	require.NoError(t, err)

	defer func() { _ = device.Close() }()

	tests := []struct {
		name string
		app  *app
		env  map[string]string
		want bool
	}{
		{name: "a terminal", app: &app{}, env: map[string]string{"TERM": "xterm"}, want: true},
		{name: "--no-color", app: &app{noColor: true}, env: map[string]string{"TERM": "xterm"}},
		{name: "NO_COLOR", app: &app{}, env: map[string]string{"TERM": "xterm", "NO_COLOR": "1"}},
		{name: "a dumb terminal", app: &app{}, env: map[string]string{"TERM": "dumb"}},
		{name: "no terminal at all", app: &app{}, env: map[string]string{}},
	}

	for _, tt := range tests {
		// Not parallel: a parallel subtest resumes after this function returns,
		// which is after the deferred Close above has already run — and Stat on
		// a closed file says "not a terminal" for the wrong reason.
		t.Run(tt.name, func(t *testing.T) {
			tt.app.env = Env{Getenv: func(name string) string { return tt.env[name] }}

			require.Equal(t, tt.want, tt.app.themeFor(device).color)
		})
	}
}

func TestAgeSpellsEveryUnit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		d    time.Duration
		want string
	}{
		{d: 30 * time.Second, want: "just now"},
		{d: 5 * time.Minute, want: "5m ago"},
		{d: 3 * time.Hour, want: "3h ago"},
		{d: 50 * time.Hour, want: "2d ago"},
	}

	for _, tt := range tests {
		require.Equal(t, tt.want, age(tt.d))
	}
}

func TestPluralCountsThings(t *testing.T) {
	t.Parallel()

	require.Equal(t, "1 issue", plural(1, "issue"))
	require.Equal(t, "2 issues", plural(2, "issue"))
	require.Equal(t, "1 child", plural(1, "child"))
	require.Equal(t, "0 children", plural(0, "child"))
}

// testWriter is a writer that is not a file, which is what every stream in this
// package's tests is.
type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }
