package cli

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// reassigning is a repository whose configuration names one agent, holding an
// issue owned by a person, on a branch that hands it to somebody else. Who
// wrote that commit is the whole subject of these tests.
func reassigning(t *testing.T, author string) *gittest.Repo {
	t.Helper()

	owned := func(owner string) []gittest.IssueOption {
		return []gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner(owner),
			gittest.Field("repro", "post twice"),
		}
	}

	return gittest.New(t).
		File(".isu.yml", "prefix: ISU\nagents:\n  - claude\n").
		Issue("ISU-7f3akq", owned("dmitry")...).Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		As(author).
		Issue("ISU-7f3akq", owned("alice")...).Commit("hand it to alice")
}

func TestAnAgentReassigningAnOwnerFails(t *testing.T) {
	t.Parallel()

	found := only(t, rules(t, reassigning(t, "claude")))

	require.Equal(t, "owner", found.Check)
	require.Equal(t, "fail", found.Severity)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Contains(t, found.Message, `"dmitry" to "alice"`)
	require.Contains(t, found.Message, "only a person changes it")
}

func TestAPersonReassigningAnOwnerIsFine(t *testing.T) {
	t.Parallel()

	require.Empty(t, rules(t, reassigning(t, "dmitry")).Findings,
		"accountability moves between people, and that is what triage is")
}

// The rule is about one field. An agent doing the work it is for — claiming,
// resolving, commenting — touches the file constantly and must not trip it.
func TestAnAgentChangingEverythingButTheOwnerIsFine(t *testing.T) {
	t.Parallel()

	fields := func(extra ...gittest.IssueOption) []gittest.IssueOption {
		return append([]gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner("dmitry"),
			gittest.Field("repro", "post twice"),
		}, extra...)
	}

	r := gittest.New(t).
		File(".isu.yml", "prefix: ISU\nagents:\n  - claude\n").
		Issue("ISU-7f3akq", fields()...).Commit("report ISU-7f3akq").
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		As("claude").
		Issue("ISU-7f3akq", fields(gittest.State("resolved"), gittest.Priority("p0"))...).
		File("login.go", "package login\n").
		Commit("claim ISU-7f3akq and fix it")

	require.Empty(t, rules(t, r).Findings)
}

// A branch is not one author. The rule is about the commit that did it, so a
// person's commits on the same branch neither excuse the agent's nor are
// blamed for it.
func TestAMixedBranchFailsOnTheAgentsCommitAndOnlyThat(t *testing.T) {
	t.Parallel()

	owned := func(owner string, extra ...gittest.IssueOption) []gittest.IssueOption {
		return append([]gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner(owner),
			gittest.Field("repro", "post twice"),
		}, extra...)
	}

	r := gittest.New(t).
		File(".isu.yml", "prefix: ISU\nagents:\n  - claude\n").
		Issue("ISU-7f3akq", owned("dmitry")...).Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		As("dmitry").
		Issue("ISU-7f3akq", owned("alice")...).Commit("hand it to alice").
		As("claude").
		Issue("ISU-7f3akq", owned("claude")...).Commit("and the agent takes it").
		As("dmitry").
		Issue("ISU-7f3akq", owned("claude", gittest.Priority("p0"))...).
		Commit("raise the priority")

	found := only(t, rules(t, r))

	require.Equal(t, "owner", found.Check)
	require.Contains(t, found.Message, `"alice" to "claude"`)
}

// A repository that has not said which authors are agents has none, which is
// the default and is not a rule that fires on everybody.
func TestWithNoAgentsConfiguredNobodyIsOne(t *testing.T) {
	t.Parallel()

	owned := func(owner string) []gittest.IssueOption {
		return []gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner(owner),
			gittest.Field("repro", "post twice"),
		}
	}

	r := configured(t).
		Issue("ISU-7f3akq", owned("dmitry")...).Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		As("claude").
		Issue("ISU-7f3akq", owned("alice")...).Commit("hand it to alice")

	require.Empty(t, rules(t, r).Findings)
}

// An agent is named by what git records, which is a name and an address. A
// team that configured the address gets the same answer as one that configured
// the name.
func TestAnAgentIsRecognisedByItsAddressToo(t *testing.T) {
	t.Parallel()

	owned := func(owner string) []gittest.IssueOption {
		return []gittest.IssueOption{
			gittest.Title("Login retries"), gittest.Owner(owner),
			gittest.Field("repro", "post twice"),
		}
	}

	r := gittest.New(t).
		File(".isu.yml", "prefix: ISU\nagents:\n  - CLAUDE@example.invalid\n").
		Issue("ISU-7f3akq", owned("dmitry")...).Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		As("claude").
		Issue("ISU-7f3akq", owned("alice")...).Commit("hand it to alice")

	require.Len(t, rules(t, r).Findings, 1)
}
