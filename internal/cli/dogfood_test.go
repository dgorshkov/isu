package cli

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// isu tracks its own construction, and this is what keeps that true.
//
// The conversion in M5-S7 turned every remaining story in PLAN.md into an issue
// folder. Two documents saying the same thing drift the moment nobody is
// checking, so these read both and hold them to each other: the plan is the
// brief, the issues are the queue, and neither is allowed to grow a story the
// other has never heard of.
//
// It reads the working tree rather than trunk, deliberately, where PLAN.md says
// trunk. An assertion about trunk is one that cannot fail on the pull request
// that breaks it — trunk has not merged it yet — so it would go green for the
// whole of the review and red immediately afterwards, which is the one moment
// nobody is looking.

// planRoot is this repository, from the directory the tests run in.
const planRoot = "../.."

var (
	storyHeading     = regexp.MustCompile(`(?m)^### (M[5-9]-S\d+ · .+?)\s*$`)
	milestoneHeading = regexp.MustCompile(`(?m)^# (M[5-9] · .+?)\s*$`)
	doneHeading      = regexp.MustCompile(` ✅$`)
)

// ours is every issue in this repository, as the working tree holds it.
func ours(t *testing.T) *repo.Set {
	t.Helper()

	loader, err := repo.Open(planRoot)
	require.NoError(t, err)

	set, err := loader.LoadWorktree(t.Context())
	require.NoError(t, err)
	require.Empty(t, set.Broken, "every issue in this repository has to be readable")

	return set
}

// brief is PLAN.md, read.
func brief(t *testing.T) string {
	t.Helper()

	body, err := os.ReadFile(planRoot + "/PLAN.md")
	require.NoError(t, err)

	return string(body)
}

func TestEveryIssueInThisRepositoryValidates(t *testing.T) {
	t.Parallel()

	set := ours(t)
	require.NotEmpty(t, set.Issues, "isu tracks its own construction from M5-S7 on")

	for _, id := range set.IDs() {
		i, _ := set.Get(id)
		require.NoErrorf(t, i.Validate(), "%s", id)
	}
}

func TestEveryStoryInThisRepositoryBelongsToAnEpicThatHasChildren(t *testing.T) {
	t.Parallel()

	set := ours(t)

	children := map[string]int{}

	for _, id := range set.IDs() {
		i, _ := set.Get(id)

		if i.Type == issue.TypeEpic {
			continue
		}

		// `parent` stays optional in the schema — a repository with no epics
		// at all is a perfectly good repository — so this is an assertion
		// about this repository's tree, which the conversion controls.
		require.NotEmptyf(t, i.Parent, "%s names no epic", id)

		parent, ok := set.Get(i.Parent)
		require.Truef(t, ok, "%s names %s, which is not here", id, i.Parent)
		require.Equalf(t, issue.TypeEpic, parent.Type, "%s is %s's parent", i.Parent, id)

		children[i.Parent]++
	}

	for _, id := range set.IDs() {
		if i, _ := set.Get(id); i.Type == issue.TypeEpic {
			require.Positivef(t, children[id], "%s is an epic with no children", id)
		}
	}
}

func TestTheDependencyGraphOfThisRepositoryIsAcyclic(t *testing.T) {
	t.Parallel()

	set := ours(t)

	// Kahn's algorithm, which is the shortest honest way to say "acyclic": a
	// graph whose nodes cannot all be removed in dependency order has a ring in
	// it, and it does not matter for this assertion which ring.
	waiting := map[string]int{}
	blocks := map[string][]string{}

	for _, id := range set.IDs() {
		i, _ := set.Get(id)

		for _, blocker := range i.BlockedBy {
			_, known := set.Get(blocker)
			require.Truef(t, known, "%s waits on %s, which is not here", id, blocker)

			waiting[id]++
			blocks[blocker] = append(blocks[blocker], id)
		}
	}

	var ready []string

	for _, id := range set.IDs() {
		if waiting[id] == 0 {
			ready = append(ready, id)
		}
	}

	settled := 0

	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		settled++

		for _, next := range blocks[id] {
			waiting[next]--
			if waiting[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	require.Equal(t, len(set.Issues), settled,
		"a ring in blocked_by is a queue in which nothing is ever ready")
}

// PLAN.md is the brief and issues/ is the queue. A story in one and not the
// other is the drift this whole conversion exists to prevent.
func TestEveryStoryInThePlanHasAnIssueAndTheOtherWayRound(t *testing.T) {
	t.Parallel()

	body := brief(t)

	var want []string

	for _, match := range milestoneHeading.FindAllStringSubmatch(body, -1) {
		want = append(want, strings.TrimSpace(doneHeading.ReplaceAllString(match[1], "")))
	}

	for _, match := range storyHeading.FindAllStringSubmatch(body, -1) {
		want = append(want, strings.TrimSpace(doneHeading.ReplaceAllString(match[1], "")))
	}

	set := ours(t)

	var got []string

	for _, id := range set.IDs() {
		i, _ := set.Get(id)
		got = append(got, i.Title)
	}

	sort.Strings(want)
	sort.Strings(got)

	require.Equal(t, want, got,
		"every milestone from M5 on is an epic and every story is an issue, and "+
			"nothing else is: PLAN.md and issues/ are two documents saying the same "+
			"thing, and two documents drift the moment nobody is checking")
}

// The rules this milestone built, run over the repository that built them.
func TestThisRepositoryPassesItsOwnChecks(t *testing.T) {
	t.Parallel()

	got := isu(t, planRoot, "check", "--worktree")

	require.Equalf(t, 0, got.code, "isu check on the isu repository: %s%s",
		got.stdout, got.stderr)
	require.Contains(t, got.stdout, "nothing to report")
}
