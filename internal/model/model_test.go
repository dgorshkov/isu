package model_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// load is the wiring `isu board` will do in M4: read every ref, read what trunk
// has said about each issue over time, and hand both to a derivation that never
// touches git itself.
//
// It is here rather than in each test so that every test in this package reads
// a repository the way the product will, rather than the way that particular
// assertion would have been easiest to write.
func load(t *testing.T, r *gittest.Repo, now time.Time) model.Input {
	t.Helper()

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	ctx := t.Context()

	board, err := loader.LoadBoard(ctx, repo.BoardSpec{Trunk: gittest.DefaultBranch})
	require.NoError(t, err)

	history, err := loader.LoadHistory(ctx, gittest.DefaultBranch)
	require.NoError(t, err)

	// Which branches claim something is a pure question about what was already
	// loaded; who claimed and when is one git log each, and it belongs to the
	// loader. That two-step is the wiring, and it is here so that every test in
	// this package exercises it rather than the shortest path to its assertion.
	claims, err := loader.LoadFirstCommits(ctx, gittest.DefaultBranch, model.ClaimRefs(board))
	require.NoError(t, err)

	return model.Input{
		Loaded:  board,
		History: history,
		Claims:  claims,
		Config:  defaultConfig(),
		Now:     now,
	}
}

// defaultConfig is a repository whose .isu.yml sets nothing but the prefix it
// must set — seven days to a stale claim.
func defaultConfig() config.Config {
	cfg := config.Default()
	cfg.Prefix = gittest.DefaultPrefix

	return cfg
}

// derive is load plus the derivation, for the tests whose subject is the answer
// and not the wiring.
func derive(t *testing.T, r *gittest.Repo) *model.Board {
	t.Helper()

	return model.Derive(load(t, r, time.Time{}))
}

// item is one issue off a derived board, failing the test rather than returning
// a second value nobody checks.
func item(t *testing.T, b *model.Board, id string) *model.Item {
	t.Helper()

	got, ok := b.Get(id)
	require.True(t, ok, "%s is not on the board, which holds %v", id, b.IDs())

	return got
}

// statusOf is the assertion most of these tests are really making.
func statusOf(t *testing.T, b *model.Board, id string) model.Status {
	t.Helper()

	return item(t, b, id).Status
}

// droppedIssue is the frontmatter a dropped issue needs. `dropped` on its own
// cannot tell a duplicate from a won't-fix, so the schema requires both a
// reason and a resolution, and a fixture that skipped them would be one no
// repository could contain.
var droppedIssue = []gittest.IssueOption{
	gittest.State("dropped"),
	gittest.Field("reason", "the customer withdrew the request"),
	gittest.Field("resolution", "wontfix"),
}

// epicIssue is an epic: the one type that declares no state, because its status
// is the fold over its children.
var epicIssue = []gittest.IssueOption{
	gittest.Type("epic"),
	gittest.Without("state"),
}
