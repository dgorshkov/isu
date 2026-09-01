package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

// One repository per violation, and one clean repository asserting nothing is
// reported about it. A check suite that says something about a healthy
// repository is a suite people learn to run through `grep -v`, so the fixtures
// below hold exactly what they are about and the assertion is usually that the
// whole run produced one finding.

// rules runs the checks and hands back what they found.
func rules(t *testing.T, r *gittest.Repo, args ...string) CheckPayload {
	t.Helper()

	got := isu(t, r.Dir(), append([]string{"check", "--json"}, args...)...)

	return decode[CheckPayload](t, got)
}

// only is the one finding a fixture is about. A fixture that produced two has
// grown a second problem, and the test that ignored it would be the reason
// nobody trusts the first.
func only(t *testing.T, payload CheckPayload) Finding {
	t.Helper()

	require.Lenf(t, payload.Findings, 1, "expected one finding, got %+v", payload.Findings)

	return payload.Findings[0]
}

func TestAnIssueThatDoesNotSatisfyTheSchemaFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Without("owner")).
		Commit("report an issue nobody is answerable for")

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Equal(t, "fail", found.Severity)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Equal(t, "issues/ISU-7f3akq/README.md", found.Path)
	require.Contains(t, found.Message, "owner: required")
}

func TestAnIdThatDisagreesWithItsFolderFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Field("id", "ISU-40b1cc")).
		Commit("put one issue in another issue's folder")

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "must equal the folder name")
}

func TestAnIssueFileNothingCanDecodeFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-7f3akq/README.md", "there is no frontmatter here at all\n").
		Commit("write something that is not an issue")

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "will not decode")
}

// An epic must not declare a state: its status is the fold over its children,
// and a state of its own is a second answer to a question that already has one.
func TestAnEpicThatDeclaresAStateFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-epical", gittest.Type("epic"), gittest.State("open")).
		Issue("ISU-7f3akq", gittest.Parent("ISU-epical")).
		Commit("an epic with a state of its own")

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Equal(t, "ISU-epical", found.ID)
	require.Contains(t, found.Message, "must not declare a state")
}

// A file that is fine at trunk and broken on the branch proposing it is a
// broken file. The finding names the branch, because a report that said this
// about trunk would send somebody looking in the wrong place.
func TestAViolationOnABranchNamesTheBranch(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		Issue("ISU-7f3akq", gittest.Without("title")).Commit("take the title away").
		Checkout(gittest.DefaultBranch)

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "on topic: title: required")
}

// A file that decodes at trunk and does not on the branch. The branch's copy is
// the one under review, so it is the one reported — and the finding names the
// branch, because a reader sent to trunk would find nothing wrong there.
func TestAnIssueOnlyTheBranchCannotDecodeIsReportedAgainstTheBranch(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq").Commit("report ISU-7f3akq").
		Branch("topic").Checkout("topic").
		File("issues/ISU-7f3akq/README.md", "there is no frontmatter here\n").
		Commit("break it on the branch").
		Checkout(gittest.DefaultBranch)

	found := only(t, rules(t, r))

	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "on topic: will not decode")
}

func TestTwoBranchesReportingDifferentIssuesUnderOneIdFail(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Branch("report/one").Checkout("report/one").
		Issue("ISU-7f3akq", gittest.Title("Sign-up 500s on Firefox")).
		Commit("one clone files an issue").
		Checkout(gittest.DefaultBranch).
		Branch("report/two").Checkout("report/two").
		Issue("ISU-7f3akq", gittest.Title("The board renders nothing")).
		Commit("another clone files a different one, in the same second").
		Checkout(gittest.DefaultBranch)

	found := only(t, rules(t, r))

	require.Equal(t, "duplicates", found.Check)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Contains(t, found.Message, "report/one")
	require.Contains(t, found.Message, "report/two")
	require.Contains(t, found.Message, "permanent from creation")
}

// The ordinary case, which this rule must never fire on: one issue, edited on
// two branches, is one issue.
func TestOneIssueEditedOnTwoBranchesIsNotADuplicate(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Title("Login retries")).Commit("report ISU-7f3akq").
		Branch("triage/ISU-7f3akq").Checkout("triage/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Title("Login retries"), gittest.Priority("p0")).
		Commit("triage it").
		Checkout(gittest.DefaultBranch).
		Branch("isu/ISU-7f3akq").Checkout("isu/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Title("Login retries"), gittest.State("resolved")).
		Commit("claim ISU-7f3akq").
		Checkout(gittest.DefaultBranch)

	require.Empty(t, rules(t, r).Findings)
}

// A report on one branch and its triage on another differ in everything a
// triage edit touches, and are still one issue.
func TestATriagedReportOnASecondBranchIsNotADuplicate(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Branch("report/ISU-7f3akq").Checkout("report/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Title("Sign-up 500s"), gittest.Owner("support")).
		Commit("report it").
		Branch("triage/ISU-7f3akq").Checkout("triage/ISU-7f3akq").
		Issue("ISU-7f3akq", gittest.Title("Sign-up 500s"), gittest.Owner("dmitry"),
			gittest.Priority("p0")).
		Commit("triage it").
		Checkout(gittest.DefaultBranch)

	require.Empty(t, rules(t, r).Findings)
}

func TestAParentThatDoesNotExistFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Parent("ISU-nobody")).
		Commit("belong to nothing")

	found := only(t, rules(t, r))

	require.Equal(t, "links", found.Check)
	require.Contains(t, found.Message, "parent ISU-nobody does not exist")
}

func TestAParentThatIsNotAnEpicFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-40b1cc").
		Issue("ISU-7f3akq", gittest.Parent("ISU-40b1cc")).
		Commit("point at something that is not an epic")

	found := only(t, rules(t, r))

	require.Equal(t, "links", found.Check)
	require.Contains(t, found.Message, "is not an epic")
}

func TestAnIssueThatIsItsOwnParentFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Parent("ISU-7f3akq")).
		Commit("belong to itself")

	found := only(t, rules(t, r))

	require.Equal(t, "links", found.Check)
	require.Contains(t, found.Message, "names itself as its parent")
}

func TestABlockerThatDoesNotExistFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.BlockedBy("ISU-nobody")).
		Commit("wait on nothing")

	found := only(t, rules(t, r))

	require.Equal(t, "links", found.Check)
	require.Contains(t, found.Message, "blocked_by names ISU-nobody")
}

func TestAnIssueBlockedByItselfFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.BlockedBy("ISU-7f3akq")).
		Commit("wait on itself")

	found := only(t, rules(t, r))

	require.Equal(t, "links", found.Check)
	require.Contains(t, found.Message, "never be terminal before itself")
}

func TestEpicsThatAreEachOthersParentsFail(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-aaaaaa", gittest.Type("epic"), gittest.Without("state"),
			gittest.Parent("ISU-bbbbbb")).
		Issue("ISU-bbbbbb", gittest.Type("epic"), gittest.Without("state"),
			gittest.Parent("ISU-aaaaaa")).
		Commit("two epics inside each other")

	found := only(t, rules(t, r))

	require.Equal(t, "cycles", found.Check)
	require.Contains(t, found.Message, "ISU-aaaaaa, ISU-bbbbbb")
	require.Contains(t, found.Message, "each other's ancestors")
}

func TestIssuesThatWaitOnEachOtherFail(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-aaaaaa", gittest.BlockedBy("ISU-bbbbbb")).
		Issue("ISU-bbbbbb", gittest.BlockedBy("ISU-cccccc")).
		Issue("ISU-cccccc", gittest.BlockedBy("ISU-aaaaaa")).
		Commit("a queue in which nothing is ever ready")

	found := only(t, rules(t, r))

	require.Equal(t, "cycles", found.Check)
	require.Contains(t, found.Message, "ISU-aaaaaa, ISU-bbbbbb, ISU-cccccc")
	require.Contains(t, found.Message, "is ever ready")
}

func TestAnEpicWithNoChildrenFails(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-epical", gittest.Type("epic"), gittest.Without("state")).
		Commit("an epic nobody filled in")

	found := only(t, rules(t, r))

	require.Equal(t, "epics", found.Check)
	require.Equal(t, "ISU-epical", found.ID)
	require.Contains(t, found.Message, "folds over nothing")
}

func TestAnAttachmentOverTheCapFails(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).
		File(".isu.yml", "prefix: ISU\nattachment_max_bytes: 16\n").
		Issue("ISU-7f3akq",
			gittest.Attachment("repro.har", strings.Repeat("x", 64)),
			gittest.Attachment("small.txt", "fine\n"),
			gittest.Comment("2026-08-24-support-01.md", strings.Repeat("y", 64))).
		Commit("attach something enormous")

	found := only(t, rules(t, r))

	require.Equal(t, "attachments", found.Check)
	require.Equal(t, "ISU-7f3akq", found.ID)
	require.Equal(t, "issues/ISU-7f3akq/repro.har", found.Path)
	require.Contains(t, found.Message, "64 bytes")
	require.Contains(t, found.Message, "attachment_max_bytes is 16")
}

func TestTheStructuralRulesAreAllInTheTreeScope(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Parent("ISU-nobody")).
		Commit("belong to nothing")

	require.Len(t, rules(t, r, "--scope", "tree").Findings, 1)
	require.Empty(t, rules(t, r, "--scope", "branch").Findings,
		"a structural rule is about the repository and not about a branch")
}

// A failing run exits 1, and says so once. The report is the message: a second,
// vaguer sentence on stderr would be the other half of every failing pipeline's
// output.
func TestAFailingCheckExitsOneAndSaysNothingTwice(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-7f3akq", gittest.Parent("ISU-nobody")).
		Commit("belong to nothing")

	got := isu(t, r.Dir(), "check")

	require.Equal(t, 1, got.code)
	require.Empty(t, got.stderr)
	require.Contains(t, got.stdout, "1 failure")
	require.Contains(t, got.stdout, "fail")

	asJSON := isu(t, r.Dir(), "--json", "check")

	require.Equal(t, 1, asJSON.code)
	require.Empty(t, asJSON.stderr, "the report is on stdout, and it is the whole answer")
	require.False(t, decode[CheckPayload](t, asJSON).OK)
}

func TestGoldenCheckReport(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-epical", gittest.Type("epic"), gittest.Without("state")).
		Issue("ISU-7f3akq", gittest.Parent("ISU-nobody"), gittest.Without("owner")).
		Commit("two issues, four problems")

	golden(t, "check/findings.txt", isu(t, r.Dir(), "check").stdout)
}

// The pre-commit hook reads the working tree, and this is why. A check that
// read a ref before a commit would be answering about the commit before the one
// being made: it would miss what is about to land, and it would refuse the
// commit that fixed what it was complaining about.
func TestTheWorktreeRunReadsWhatIsAboutToBeCommitted(t *testing.T) {
	t.Parallel()

	r := configured(t)

	r.WriteFile("issues/ISU-7f3akq/README.md", brokenIssue)

	require.Empty(t, rules(t, r).Findings, "nothing is committed, so no ref says anything")

	found := only(t, rules(t, r, "--worktree"))
	require.Equal(t, "schema", found.Check)
	require.Contains(t, found.Message, "owner: required")
}

func TestTheWorktreeRunDoesNotRefuseTheCommitThatFixesTheProblem(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-7f3akq/README.md", brokenIssue).
		Commit("commit something broken")

	require.Len(t, rules(t, r).Findings, 1, "the ref says it is broken, and it is")

	r.WriteFile("issues/ISU-7f3akq/README.md", fixedIssue)

	require.Empty(t, rules(t, r, "--worktree").Findings,
		"a hook that read the ref here would refuse the commit that fixes it")
}

func TestTheWorktreeRunSaysSoAndAsksNoBranchQuestions(t *testing.T) {
	t.Parallel()

	r := resolved(t)

	require.Len(t, rules(t, r).Findings, 1, "the branch resolved an issue and wrote no code")

	payload := rules(t, r, "--worktree")

	require.True(t, payload.Worktree)
	require.Empty(t, payload.Findings,
		"the working tree is not a set of commits, so there is nothing here for "+
			"the branch rules to be about")
	require.Contains(t, isu(t, r.Dir(), "check", "--worktree").ok(t).stdout,
		"the working tree")
}

// brokenIssue and fixedIssue are one file, without and with the one field that
// makes it valid.
const (
	brokenIssue = `---
schema: 1
id: ISU-7f3akq
title: Login retries
type: chore
state: open
created: 2026-08-24
---
`
	fixedIssue = `---
schema: 1
id: ISU-7f3akq
title: Login retries
type: chore
state: open
owner: dmitry
created: 2026-08-24
---
`
)
