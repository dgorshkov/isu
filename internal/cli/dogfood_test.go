package cli

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// isu tracks its own construction, and this is what keeps that true.
//
// M5-S7 turned every remaining story in the plan into an issue folder, and the
// tests here held the two documents to each other because two documents saying
// the same thing drift the moment nobody is checking. The drift is gone because
// the second document is: the plan's fifty-one stories are all issues now, from
// M0-S1 to M9-S4, and `issues/` is the build order, the queue and the history
// at once.
//
// What is left to assert is that the one remaining document is whole. A tree
// with a gap in it — a milestone with no epic, a story numbered past the end of
// its milestone, a `blocked_by` chain that skips a link — is the same defect the
// correspondence test used to catch, asked of one document instead of two.
//
// It reads the working tree rather than trunk, deliberately. An assertion about
// trunk is one that cannot fail on the pull request that breaks it — trunk has
// not merged it yet — so it would go green for the whole of the review and red
// immediately afterwards, which is the one moment nobody is looking.

// planRoot is this repository, from the directory the tests run in.
const planRoot = "../.."

// milestones is how many the build has: M0 through M9, all of them epics.
const milestones = 10

var (
	// storyTitle and epicTitle are the two shapes a title in this repository
	// takes. Nothing else is allowed one, which is what keeps a story from
	// arriving outside a milestone.
	storyTitle = regexp.MustCompile(`^M(\d)-S(\d+) · .+$`)
	epicTitle  = regexp.MustCompile(`^M(\d) · .+$`)

	// doneRecord is the line the working agreement asks every finished story to
	// carry: which pull request landed it, and when.
	doneRecord = regexp.MustCompile(`(?m)^\*\*Done\*\* #\d+, \d{4}-\d{2}-\d{2}`)
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

// build is this repository's issues indexed the way the plan named them: the
// epic of each milestone, and each milestone's stories by their number.
type build struct {
	epics   map[int]string         // milestone -> epic id
	stories map[int]map[int]string // milestone -> story number -> id
}

// read indexes the tree and fails on anything whose title is not a milestone or
// a story of one. A title outside those two shapes is a story that belongs to no
// milestone, which is the thing this repository's queue may not grow.
func read(t *testing.T) (*repo.Set, build) {
	t.Helper()

	set := ours(t)
	b := build{epics: map[int]string{}, stories: map[int]map[int]string{}}

	for _, id := range set.IDs() {
		i, _ := set.Get(id)

		if m := storyTitle.FindStringSubmatch(i.Title); m != nil {
			mile, story := atoi(t, m[1]), atoi(t, m[2])
			if b.stories[mile] == nil {
				b.stories[mile] = map[int]string{}
			}

			require.Emptyf(t, b.stories[mile][story], "two issues are M%d-S%d", mile, story)
			b.stories[mile][story] = id

			continue
		}

		m := epicTitle.FindStringSubmatch(i.Title)
		require.NotNilf(t, m, "%s is titled %q, which is neither a milestone nor "+
			"a story of one: this repository's queue is its build order", id, i.Title)

		mile := atoi(t, m[1])
		require.Emptyf(t, b.epics[mile], "two issues are M%d", mile)
		b.epics[mile] = id
	}

	return set, b
}

// The build is whole: every milestone has an epic, and every milestone's
// stories are numbered from one with no gap. A gap is a story that was planned
// and never written down, which is exactly what the old correspondence test
// caught when the plan was a second file.
func TestTheBuildHasEveryMilestoneAndEveryStoryOfEachOne(t *testing.T) {
	t.Parallel()

	_, b := read(t)

	require.Len(t, b.epics, milestones, "M0 through M9 are epics, and nothing else is")

	for mile := range milestones {
		require.NotEmptyf(t, b.epics[mile], "M%d has no epic", mile)
		require.NotEmptyf(t, b.stories[mile], "M%d has no stories", mile)

		for story := 1; story <= len(b.stories[mile]); story++ {
			require.NotEmptyf(t, b.stories[mile][story],
				"M%d has %d stories and no M%d-S%d: the numbers run from one "+
					"without a gap, because a gap is a story nobody wrote down",
				mile, len(b.stories[mile]), mile, story)
		}
	}
}

// `blocked_by` is the build order, and it is a chain rather than a suggestion:
// a story waits on the one before it, and the first story of a milestone waits
// on the milestone before it. That is what makes `isu ready` the answer to what
// to work on next.
func TestBlockedByIsTheBuildOrder(t *testing.T) {
	t.Parallel()

	set, b := read(t)

	waits := func(id string) []string {
		i, _ := set.Get(id)

		return i.BlockedBy
	}

	for mile := range milestones {
		require.Emptyf(t, waits(b.epics[mile]), "M%d is an epic and waits on nothing", mile)

		for story := 1; story <= len(b.stories[mile]); story++ {
			id := b.stories[mile][story]

			var want []string

			switch {
			case story > 1:
				want = []string{b.stories[mile][story-1]}
			case mile > 0:
				want = []string{b.epics[mile-1]}
			}

			require.Equalf(t, want, waits(id),
				"M%d-S%d is the %s", mile, story,
				map[bool]string{true: "first story of its milestone", false: "next story"}[story == 1])
		}
	}
}

// A finished story says which pull request finished it and when. The working
// agreement asks for that line in the same pull request that resolves the
// story, and a resolved issue without one is a story whose record is a state
// field and nothing else.
func TestEveryResolvedStoryRecordsWhenItWasDone(t *testing.T) {
	t.Parallel()

	set, _ := read(t)

	resolved := 0

	for _, id := range set.IDs() {
		i, _ := set.Get(id)
		if i.State != issue.StateResolved {
			continue
		}

		resolved++

		require.Regexpf(t, doneRecord, i.Body,
			"%s is resolved and carries no `**Done** #<pr>, <date>` line", id)
	}

	require.Positive(t, resolved, "six milestones of this build have landed")
}

// The rules this milestone built, run over the repository that built them.
func TestThisRepositoryPassesItsOwnChecks(t *testing.T) {
	t.Parallel()

	got := isu(t, planRoot, "check", "--worktree")

	require.Equalf(t, 0, got.code, "isu check on the isu repository: %s%s",
		got.stdout, got.stderr)
	require.Contains(t, got.stdout, "nothing to report")
}
