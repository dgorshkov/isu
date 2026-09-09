package site

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
)

// built is where the site is committed. It is committed rather than only
// generated because that is what makes M8-S2's gate possible: the review of
// this site is a diff, and a diff needs something to compare against.
var built = filepath.Join(root, "web", "site")

// update rewrites the committed site. `make site` is this flag, which is the
// same bargain every golden file in this project makes — see
// internal/cli/testdata.
var update = flag.Bool("update", false, "rewrite the site under web/site")

// TestTheCommittedSiteIsWhatTheBuildProduces is M8-S2's central gate: a test
// regenerating every sample, failing if the committed page differs.
//
// It is what stops the site drifting from the product. Change the board's
// renderer and this test goes red with the two boards side by side, which is a
// better answer than a website that quietly starts lying.
func TestTheCommittedSiteIsWhatTheBuildProduces(t *testing.T) {
	files, err := Build(t.Context(), root, t.TempDir())
	require.NoError(t, err)

	if *update {
		require.NoError(t, os.RemoveAll(built))
		require.NoError(t, Write(built, files))

		return
	}

	for _, path := range sortedPaths(files) {
		want, err := os.ReadFile(filepath.Join(built, filepath.FromSlash(path)))
		require.NoErrorf(t, err, "%s is not committed; run `make site`", path)
		require.Equalf(t, string(want), string(files[path]),
			"%s is not what the build produces; run `make site`", path)
	}

	require.Equal(t, sortedPaths(files), committed(t),
		"web/site holds files this build does not produce; run `make site`")
}

// committed is every file under web/site, as site-relative slash paths.
func committed(t *testing.T) []string {
	t.Helper()

	var out []string

	require.NoError(t, filepath.Walk(built, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}

		rel, err := filepath.Rel(built, path)
		require.NoError(t, err)

		out = append(out, filepath.ToSlash(rel))

		return nil
	}))

	sort.Strings(out)

	return out
}

// TestNoTemplateWritesAColourOrAPixelSize is the other half of M8-S2's
// tokens rule. gates.go holds the stylesheet and the built pages to it; this
// holds the templates, which the built pages are only evidence about.
func TestNoTemplateWritesAColourOrAPixelSize(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob(filepath.Join(root, "web", "templates", "*.tmpl"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	for _, path := range matches {
		body, err := os.ReadFile(path)
		require.NoError(t, err)

		require.Empty(t, hexLiteral.FindString(string(body)),
			"%s writes a colour; the stylesheet is the one place", path)
		require.Empty(t, regexp.MustCompile(`\d+px`).FindString(string(body)),
			"%s writes a pixel size", path)
		require.NotContains(t, string(body), "style=",
			"%s carries a style attribute; the stylesheet is the one place", path)
	}
}
