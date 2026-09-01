package check_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/check"
	"github.com/dgorshkov/isu/internal/config"
)

// fake is a check whose findings a test decided in advance. The engine is a
// registry, an order and a summary; what a real check reads is the subject of
// every other file in this package.
type fake struct {
	name     string
	scope    check.Scope
	findings []check.Finding
	// saw is where a fake records the input it was handed, so that a test can
	// assert the engine passes it through rather than reconstructing one.
	saw *check.Input
}

func (f fake) Name() string { return f.name }
func (f fake) Scope() check.Scope {
	if f.scope == "" {
		return check.ScopeTree
	}

	return f.scope
}
func (f fake) Describe() string { return "what " + f.name + " is about" }

func (f fake) Run(in check.Input) []check.Finding {
	if f.saw != nil {
		*f.saw = in
	}

	return f.findings
}

func fails(name, id, message string) check.Finding {
	return check.Finding{Severity: check.SeverityFail, ID: id, Message: message}
}

func warns(id, message string) check.Finding {
	return check.Finding{Severity: check.SeverityWarn, ID: id, Message: message}
}

func TestARegistryRefusesADuplicateName(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()

	require.NoError(t, r.Register(fake{name: "links"}))
	require.Equal(t, 1, r.Len())

	err := r.Register(fake{name: "links"})
	require.ErrorContains(t, err, "already registered")
	require.Equal(t, 1, r.Len(), "the second registration changed nothing")
}

func TestARegistryRefusesACheckWithNoName(t *testing.T) {
	t.Parallel()

	require.ErrorContains(t, check.NewRegistry().Register(fake{}), "needs a name")
}

func TestMustRegisterPanicsOnWhatRegisterRefuses(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "links"})

	require.PanicsWithValue(t,
		`check: a check called "links" is already registered`,
		func() { r.MustRegister(fake{name: "links"}) })
}

func TestChecksRunInNameOrderWhateverOrderTheyWereRegisteredIn(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "owner"})
	r.MustRegister(fake{name: "attachments"})
	r.MustRegister(fake{name: "links"})

	report := r.Run(check.Input{})

	require.Equal(t, []string{"attachments", "links", "owner"}, report.Checks,
		"where a registry line sits in a file must not change a pipeline's output")
}

func TestTheEngineStampsEachFindingWithTheCheckThatFoundIt(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "links", findings: []check.Finding{
		fails("links", "ISU-7f3akq", "parent names nothing"),
	}})

	report := r.Run(check.Input{})

	require.Len(t, report.Findings, 1)
	require.Equal(t, "links", report.Findings[0].Check,
		"a check cannot disagree with the name it was registered under")
}

func TestFailuresSortBeforeWarningsAndThenByCheckIssuePathAndMessage(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "claims", findings: []check.Finding{
		warns("ISU-aaaaaa", "claimed twice"),
	}})
	r.MustRegister(fake{name: "attachments", findings: []check.Finding{
		{Severity: check.SeverityFail, ID: "ISU-bbbbbb", Path: "b", Message: "second"},
		{Severity: check.SeverityFail, ID: "ISU-bbbbbb", Path: "a", Message: "first"},
		{Severity: check.SeverityFail, ID: "ISU-aaaaaa", Path: "z", Message: "earlier issue"},
		{Severity: check.SeverityFail, ID: "ISU-bbbbbb", Path: "a", Message: "also first"},
	}})

	report := r.Run(check.Input{})

	got := make([]string, 0, len(report.Findings))
	for _, f := range report.Findings {
		got = append(got, f.Check+" "+f.ID+" "+f.Path+" "+f.Message)
	}

	require.Equal(t, []string{
		"attachments ISU-aaaaaa z earlier issue",
		"attachments ISU-bbbbbb a also first",
		"attachments ISU-bbbbbb a first",
		"attachments ISU-bbbbbb b second",
		"claims ISU-aaaaaa  claimed twice",
	}, got)
}

func TestASeverityNobodyRegisteredSortsLastRatherThanFirst(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "typo", findings: []check.Finding{
		{Severity: "catastrophe", Message: "invented"},
		{Severity: check.SeverityWarn, Message: "known"},
	}})

	report := r.Run(check.Input{})

	require.Equal(t, check.SeverityWarn, report.Findings[0].Severity,
		"a misspelt severity must not outrank a real one")
}

func TestScopePicksWhichChecksRun(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "links", scope: check.ScopeTree})
	r.MustRegister(fake{name: "evidence", scope: check.ScopeBranch})

	require.Equal(t, []string{"evidence", "links"}, r.Run(check.Input{}).Checks,
		"no scope is every scope")
	require.Equal(t, []string{"links"}, r.Run(check.Input{}, check.ScopeTree).Checks)
	require.Equal(t, []string{"evidence"}, r.Run(check.Input{}, check.ScopeBranch).Checks)
	require.Equal(t, []string{"evidence", "links"},
		r.Run(check.Input{}, check.ScopeTree, check.ScopeBranch).Checks)
}

func TestAFailingCheckIsNotOKAndAWarningIs(t *testing.T) {
	t.Parallel()

	r := check.NewRegistry()
	r.MustRegister(fake{name: "claims", findings: []check.Finding{
		warns("ISU-7f3akq", "claimed on two branches"),
	}})

	report := r.Run(check.Input{})

	require.True(t, report.OK(), "a warning is not a failure")
	require.Equal(t, 1, report.Count(check.SeverityWarn))
	require.Equal(t, 0, report.Count(check.SeverityFail))

	r.MustRegister(fake{name: "links", findings: []check.Finding{
		fails("links", "ISU-7f3akq", "parent names nothing"),
	}})

	report = r.Run(check.Input{})

	require.False(t, report.OK())
	require.Equal(t, 1, report.Count(check.SeverityFail))
}

func TestAnEmptyRunSaysSoRatherThanSayingNothing(t *testing.T) {
	t.Parallel()

	report := check.NewRegistry().Run(check.Input{})

	require.NotNil(t, report.Checks, "an empty list is a list")
	require.NotNil(t, report.Findings)
	require.True(t, report.OK())
}

func TestEveryCheckIsHandedTheInputTheEngineWasGiven(t *testing.T) {
	t.Parallel()

	var saw check.Input

	r := check.NewRegistry()
	r.MustRegister(fake{name: "links", saw: &saw})

	when := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	r.Run(check.Input{Trunk: "main", Head: "isu/ISU-7f3akq", Now: when})

	require.Equal(t, "main", saw.Trunk)
	require.Equal(t, "isu/ISU-7f3akq", saw.Head)
	require.Equal(t, when, saw.When())
}

func TestAnInputWithNoClockUsesTheRealOne(t *testing.T) {
	t.Parallel()

	require.WithinDuration(t, time.Now(), check.Input{}.When(), time.Minute)
}

func TestFetchAgeIsZeroWhenThereIsNothingToBeBehindOn(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	require.Zero(t, check.Fetch{}.Age(now))
	require.Zero(t, check.Fetch{Remote: true}.Age(now),
		"a remote whose refs have no date is a remote with no answer")
	require.Equal(t, 2*time.Hour,
		check.Fetch{Remote: true, Newest: now.Add(-2 * time.Hour)}.Age(now))
}

func TestAWarningAboutRefsSaysWhenTheRefsCannotBeTrusted(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	cfg := config.Default()

	require.Contains(t, check.Fetch{}.Caveat(cfg, now), "local branches only",
		"a repository with no remote is answering about itself and should say so")

	fresh := check.Fetch{Remote: true, Newest: now.Add(-time.Hour)}
	require.Empty(t, fresh.Caveat(cfg, now), "a fetch an hour old is not a caveat")

	stale := check.Fetch{Remote: true, Newest: now.Add(-72 * time.Hour)}
	require.Contains(t, stale.Caveat(cfg, now), "72 hours old")
	require.Contains(t, stale.Caveat(cfg, now), "--fetch")
}
