package site

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEveryCommandInTheDocsRuns is M8-S3's executable documentation: every
// fenced console block in docs/ runs against a scratch repository and prints
// what the page says it prints.
//
// Documentation that does not execute is documentation that rots, and these
// docs will be read by agents.
func TestEveryCommandInTheDocsRuns(t *testing.T) {
	for _, d := range Documents {
		t.Run(d.Name, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join(root, "docs", d.Name+".md"))
			require.NoError(t, err)

			require.NoError(t, Verify(scratchFor(t, d), string(body)))
		})
	}
}

// scratchFor builds the repository one document's commands run against.
func scratchFor(t *testing.T, d Document) string {
	t.Helper()

	build := Fixture
	if d.Live {
		build = Writable
	}

	dir, err := build(t.Context(), t.TempDir())
	require.NoError(t, err)

	return dir
}

// TestTheDocumentsAndTheDocumentationAgree keeps the list in build.go and the
// directory it names in step, so a page added to docs/ and forgotten here is a
// page nothing runs and nothing publishes.
func TestTheDocumentsAndTheDocumentationAgree(t *testing.T) {
	t.Parallel()

	matches, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	require.NoError(t, err)

	var found []string

	for _, path := range matches {
		found = append(found, strings.TrimSuffix(filepath.Base(path), ".md"))
	}

	var listed []string
	for _, d := range Documents {
		listed = append(listed, d.Name)
	}

	sort.Strings(found)
	sort.Strings(listed)

	require.Equal(t, found, listed,
		"every page in docs/ is published and executed, and nothing else is")
}
