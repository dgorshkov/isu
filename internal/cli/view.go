package cli

import (
	"context"
	"sort"
	"time"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// view is one derivation of the whole repository, which is what every read
// command starts from.
//
// It is assembled here rather than in each command because the assembly is the
// part with a process count: trunk and every branch in one load, the trunk
// history in two more, and one `git log` for each branch that actually claims
// something. Doing it per command would be fine; doing it twice in one command
// because two renderers each wanted a board would not.
type view struct {
	loaded *repo.Board
	board  *model.Board
	// trunkName is what to call trunk in the output: the branch name when the
	// ref is HEAD, and the ref otherwise.
	trunkName string
	freshness Freshness
	now       time.Time
}

func (s *session) view(ctx context.Context) (*view, error) {
	now := s.app.env.now()

	loaded, err := s.repo.LoadBoard(ctx, repo.BoardSpec{Trunk: s.trunk})
	if err != nil {
		return nil, err
	}

	history, err := s.repo.LoadHistory(ctx, s.trunk)
	if err != nil {
		return nil, err
	}

	claims, err := s.repo.LoadFirstCommits(ctx, s.trunk, model.ClaimRefs(loaded))
	if err != nil {
		return nil, err
	}

	remotes, err := s.remoteRefs(ctx)
	if err != nil {
		return nil, err
	}

	name, err := s.trunkName(ctx)
	if err != nil {
		return nil, err
	}

	return &view{
		loaded: loaded,
		board: model.Derive(model.Input{
			Loaded:  loaded,
			History: history,
			Claims:  claims,
			Config:  s.cfg,
			Now:     now,
		}),
		trunkName: name,
		freshness: s.freshness(remotes, now),
		now:       now,
	}, nil
}

// trunkName is what the output calls trunk.
//
// `--ref` defaults to HEAD, and printing "HEAD" at the top of a board tells
// nobody which branch they are looking at — which matters most in exactly the
// case it is easiest to get wrong, reading a board from a feature branch and
// wondering why everything is in progress.
func (s *session) trunkName(ctx context.Context) (string, error) {
	if s.trunk != "HEAD" {
		return s.trunk, nil
	}

	branch, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}

	if branch == "" {
		return "HEAD (detached)", nil
	}

	return branch, nil
}

// groups buckets every issue by its derived status, in the precedence order of
// PLAN.md's table, and drops the statuses nothing matched.
func (v *view) groups() []Group {
	byStatus := map[model.Status][]*model.Item{}

	for _, id := range v.board.IDs() {
		item, _ := v.board.Get(id)
		byStatus[item.Status] = append(byStatus[item.Status], item)
	}

	groups := make([]Group, 0, len(model.Statuses))

	for _, status := range model.Statuses {
		items := byStatus[status]
		if len(items) == 0 {
			continue
		}

		sortItems(items)

		issues := make([]Issue, 0, len(items))
		for _, item := range items {
			issues = append(issues, asIssue(item, v.now))
		}

		groups = append(groups, Group{Status: string(status), Issues: issues})
	}

	return groups
}

// sortItems is the order every list in this package renders in: most urgent
// first, then oldest first, then by id so that two runs over the same
// repository never disagree.
func sortItems(items []*model.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]

		ap, bp := priorityRank(a), priorityRank(b)
		if ap != bp {
			return ap < bp
		}

		at, bt := created(a), created(b)
		if !at.Equal(bt) {
			return at.Before(bt)
		}

		return a.ID < b.ID
	})
}

// priorityRank is where a priority sorts. An issue whose file would not decode
// has no priority at all and sorts as the default, because a board that put
// every unreadable issue at the top would be a board about the parser.
func priorityRank(item *model.Item) int {
	want := issue.DefaultPriority
	if item.Issue != nil {
		want = item.Issue.EffectivePriority()
	}

	for i, p := range issue.Priorities {
		if p == want {
			return i
		}
	}

	// A priority the schema does not know is M5's to report, and sorts last
	// rather than crashing a board.
	return len(issue.Priorities)
}

func created(item *model.Item) time.Time {
	if item.Issue == nil {
		return time.Time{}
	}

	return item.Issue.Created
}

// taken reports whether an id is already in use on any ref isu can see, which
// is what `isu new` regenerates against.
func (v *view) taken(id string) bool {
	_, ok := v.board.Get(id)

	return ok
}

// refNames lists the non-trunk refs the board was derived from.
func (v *view) refNames() []string {
	names := v.board.Names()
	if names == nil {
		return []string{}
	}

	return names
}
