package repo_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/repo"
)

func TestOpenBindsToARepositoryWithoutRunningGit(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	loader, err := repo.Open(r.Dir())
	require.NoError(t, err)

	require.Equal(t, r.Dir(), loader.Root())
	require.Equal(t, r.Dir(), loader.Git().Dir())
	require.Zero(t, loader.Processes(),
		"binding is not reading: a command that is about to fail should fail with "+
			"the message of the thing it was asked to do")
}

func TestOpenRefusesADirectoryThatIsNotThere(t *testing.T) {
	_, err := repo.Open(filepath.Join(t.TempDir(), "absent"))
	require.Error(t, err)
}

func TestWithGitReachesTheBinding(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Commit("add AR-7f3akq")

	loader, err := repo.Open(r.Dir(), repo.WithGit(gitx.WithTimeout(time.Nanosecond)))
	require.NoError(t, err)

	_, err = loader.LoadRef(t.Context(), gittest.DefaultBranch)
	require.ErrorContains(t, err, "did not finish within")
}

func TestSetLen(t *testing.T) {
	r := gittest.New(t).Issue("AR-7f3akq").Issue("AR-40b1cc").Commit("two issues")

	set, err := open(t, r).LoadRef(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)
	require.Equal(t, 2, set.Len())
}
