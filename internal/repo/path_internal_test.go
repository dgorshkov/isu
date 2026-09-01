package repo

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// What counts as an issue's README, asked directly.
//
// Every caller feeds this the output of a git command already limited to
// issues/, so the paths that are not under it never arrive from outside — which
// is exactly why the rule is worth pinning here rather than hoping a loader
// test wanders past it. The check is what stops a repository's own `src/` from
// becoming a hundred broken issues the day somebody drops the pathspec.
func TestIssueIDAcceptsOnlyAnIssuesReadme(t *testing.T) {
	for _, tc := range []struct {
		path string
		id   string
	}{
		{path: "issues/ISU-7f3akq/README.md", id: "ISU-7f3akq"},
		{path: "issues/ISU 7f3akq/README.md", id: "ISU 7f3akq"},

		// Not under issues/ at all.
		{path: "README.md"},
		{path: "src/main.go"},
		{path: "docs/issues/ISU-7f3akq/README.md"},
		{path: "issuesque/ISU-7f3akq/README.md"},

		// Under issues/, and not an issue's README.
		{path: "issues/README.md"},
		{path: "issues/ISU-7f3akq/repro.har"},
		{path: "issues/ISU-7f3akq/comments/2026-08-29-sam-01.md"},
		{path: "issues/ISU-7f3akq/README.md.bak"},
		{path: "issues/"},
		{path: ""},
	} {
		t.Run(tc.path, func(t *testing.T) {
			id, ok := issueID(tc.path)

			require.Equal(t, tc.id != "", ok)
			require.Equal(t, tc.id, id)
		})
	}
}

// The same question for everything else in an issue's folder, which is what the
// size cap and a spike's artifact are about. It is asked directly for the same
// reason as above: every caller has already limited git to issues/, so the
// paths that are not under it can only arrive here from a future caller who
// forgot to.
func TestIssueFileNamesWhatSitsInsideAnIssuesFolder(t *testing.T) {
	for _, tc := range []struct {
		path, id, name string
	}{
		{path: "issues/ISU-7f3akq/repro.har", id: "ISU-7f3akq", name: "repro.har"},
		{
			path: "issues/ISU-7f3akq/comments/2026-08-29-sam-01.md",
			id:   "ISU-7f3akq", name: "comments/2026-08-29-sam-01.md",
		},
		{path: "issues/ISU-7f3akq/README.md", id: "ISU-7f3akq", name: "README.md"},

		// Not under issues/ at all.
		{path: "README.md"},
		{path: "src/main.go"},

		// Under issues/ and not inside an issue.
		{path: "issues/README.md"},
		{path: "issues/ISU 7f3akq/repro.har"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			id, name, ok := issueFile(tc.path)

			require.Equal(t, tc.id != "", ok)
			require.Equal(t, tc.id, id)
			require.Equal(t, tc.name, name)
		})
	}
}
