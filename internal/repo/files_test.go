package repo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/repo"
)

func TestLoadFilesReadsWhatLivesBesideAnIssueAndItsSizes(t *testing.T) {
	r := gittest.New(t).
		Issue("ISU-7f3akq",
			gittest.Attachment("repro.har", "0123456789"),
			gittest.Comment("2026-08-24-support-01.md", "it happens on Firefox too\n")).
		Issue("ISU-40b1cc").
		File("issues/README.md", "the issues live here\n").
		Commit("report two issues")

	files, err := open(t, r).LoadFiles(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	require.Len(t, files, 1, "an issue with nothing beside its README has no files")

	names := make([]string, 0, len(files["ISU-7f3akq"]))
	for _, f := range files["ISU-7f3akq"] {
		names = append(names, f.Name)
	}

	require.Equal(t, []string{"comments/2026-08-24-support-01.md", "repro.har"}, names,
		"the README is the issue, not something beside it, and issues/README.md "+
			"is not an issue at all")

	attachments := files.Attachments("ISU-7f3akq")
	require.Len(t, attachments, 1, "a comment is prose isu wrote, not an attachment")
	require.Equal(t, "repro.har", attachments[0].Name)
	require.Equal(t, "issues/ISU-7f3akq/repro.har", attachments[0].Path)
	require.Equal(t, "ISU-7f3akq", attachments[0].ID)
	require.EqualValues(t, 10, attachments[0].Size)

	require.Empty(t, files.Attachments("ISU-40b1cc"))
}

func TestLoadFilesOfARepositoryWithNoCommitsIsEmptyRatherThanAFailure(t *testing.T) {
	files, err := open(t, gittest.New(t)).LoadFiles(t.Context(), "HEAD")
	require.NoError(t, err)
	require.Empty(t, files)
}

func TestLoadFilesRefusesARefThatIsNotThere(t *testing.T) {
	r := gittest.New(t).Issue("ISU-7f3akq").Commit("report ISU-7f3akq")

	_, err := open(t, r).LoadFiles(t.Context(), "refs/heads/nope")
	require.Error(t, err)
}

// A submodule inside an issue folder has no content to weigh, and git reports
// its size as `-`. It is not an attachment and it must not be read as one.
func TestLoadFilesSkipsWhatIsNotABlob(t *testing.T) {
	inner := gittest.New(t).File("README.md", "a module\n").Commit("first")

	r := gittest.New(t).
		Issue("ISU-7f3akq", gittest.Attachment("repro.har", "0123456789")).
		Commit("report ISU-7f3akq")

	r.Git("-c", "protocol.file.allow=always", "submodule", "--quiet", "add",
		inner.Dir(), "issues/ISU-7f3akq/vendor")
	r.Commit("vendor something inside the issue")

	files, err := open(t, r).LoadFiles(t.Context(), gittest.DefaultBranch)
	require.NoError(t, err)

	names := make([]string, 0, len(files["ISU-7f3akq"]))
	for _, f := range files["ISU-7f3akq"] {
		names = append(names, f.Name)
	}

	require.Equal(t, []string{"repro.har"}, names)
}

func TestAttachmentsOfAnIssueNobodyHasHeardOfIsNothing(t *testing.T) {
	require.Empty(t, repo.Files{}.Attachments("ISU-nobody"))
}
