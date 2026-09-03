package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

// The whole importer, end to end through the product, against a recorded dump
// and a real repository. Nothing here reaches github.com: PLAN.md M7-S5 asks
// for a recorded transcript and never the live API, and an import that only
// works when a service is up is one nobody can review.

// dumpPath is the fixture every test here reads, from the directory the tests
// run in.
const dumpPath = "testdata/import/acme.json"

// importable is a repository ready to be imported into: the configuration, and
// history carrying the conventions M7-S3's scan recovers links from.
func importable(t *testing.T) *gittest.Repo {
	t.Helper()

	r := configured(t)

	// A long tail of commits with no key in them, and one of each convention.
	r.File("login.go", "package main\n").Commit("Rework the login retries")
	r.File("board.go", "package main\n").Commit("Tidy the makefile")
	r.File("retry.go", "package main\n").Commit("Stop dropping the second POST\n\nCloses #1.")
	r.File("epics.go", "package main\n").Commit("Render epics on the board (#2)")
	r.File("drop.go", "package main\n").Commit("Nothing to do with any issue")

	return r
}

func TestADryRunIsTheDefaultAndWritesNothing(t *testing.T) {
	t.Parallel()

	r := importable(t)

	got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath).ok(t)

	require.Contains(t, got.stdout, "would write")
	require.Contains(t, got.stdout, "nothing was written")
	require.NoDirExists(t, filepath.Join(r.Dir(), "issues"),
		"an importer is the one command that creates thousands of files, and a tool "+
			"whose first invocation does that is one people run once, in the wrong "+
			"repository")

	require.Equal(t, "", strings.TrimSpace(r.Git("status", "--porcelain")))
}

func TestWritingTakesAFlagAndStagesWhatItWrote(t *testing.T) {
	t.Parallel()

	r := importable(t)

	got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)

	require.Contains(t, got.stdout, "wrote")
	require.NotContains(t, got.stdout, "nothing was written")

	for _, id := range []string{"ISU-1", "ISU-2", "ISU-3", "ISU-5", "ISU-M1"} {
		require.FileExistsf(t, filepath.Join(r.Dir(), "issues", id, issue.ReadmeName), "%s", id)
	}

	require.NoDirExists(t, filepath.Join(r.Dir(), "issues", "ISU-4"),
		"a pull request is not an issue")

	staged := r.Git("diff", "--cached", "--name-only")
	require.Contains(t, staged, "issues/ISU-1/README.md")
	require.Contains(t, staged, "issues/ISU-1/"+importer.SourceFileName)
	require.Contains(t, staged, "issues/ISU-1/comments/2026-03-05-alice-o-hara-02.md")
}

// PLAN.md M7-S4: "the imported tree passes isu check with zero failures".
func TestTheImportedTreePassesIsuCheck(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)

	got := isu(t, r.Dir(), "check", "--worktree")
	require.Equalf(t, 0, got.code, "isu check over the imported tree:\n%s%s",
		got.stdout, got.stderr)
	require.Contains(t, got.stdout, "nothing to report")
}

// PLAN.md M7-S5: "a realistic export imports completely and idempotently —
// running it twice produces a zero-length diff."
func TestRunningTheSameImportTwiceProducesNoDiff(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)
	r.Commit("import acme/widgets")

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)

	require.Empty(t, strings.TrimSpace(r.Git("status", "--porcelain")),
		"an import that is not idempotent is one nobody can re-run after a fix")
}

func TestAnImportRefusesToOverwriteADifferentRepositorysIssue(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)
	r.Commit("import acme/widgets")

	other := filepath.Join(t.TempDir(), "other.json")
	body, err := os.ReadFile(dumpPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(other,
		[]byte(strings.ReplaceAll(string(body), "acme/widgets", "other/widgets")), 0o644))

	got := isu(t, r.Dir(), "import", "github", "other/widgets", "--dump", other,
		"--owner", "support", "--write")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "refuses rather than overwriting")
}

// PLAN.md M7-S4: the import refuses when an issue has neither an assignee nor
// --owner, and the dry run says how many.
func TestAnUnassignedIssueStopsAWriteAndNotADryRun(t *testing.T) {
	t.Parallel()

	r := importable(t)

	dry := isu(t, r.Dir(), "import", "github", "--dump", dumpPath).ok(t)
	require.Contains(t, dry.stdout, "1 issue with no assignee")

	got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath, "--write")
	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "no assignee and no --owner")
	require.NoDirExists(t, filepath.Join(r.Dir(), "issues"))
}

// PLAN.md M7-S3: "the scanner runs against a real repository and reports its
// coverage."
func TestTheScanReportsWhichCommitResolvedWhat(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--write").ok(t)

	first := r.Git("rev-list", "--reverse", gittest.DefaultBranch)
	commits := strings.Fields(first)

	one := readFile(t, r, "issues/ISU-1/"+importer.SourceFileName)
	require.Contains(t, one, "resolved_by_evidence: commit message")
	require.Contains(t, one, "resolved_by: "+commits[3],
		"`Closes #1.` in a commit body, which is the strongest tier the scan has")

	two := readFile(t, r, "issues/ISU-2/"+importer.SourceFileName)
	require.Contains(t, two, "resolved_by_evidence: closing pull request",
		"GitHub already stores which pull request closed an issue, and that "+
			"outranks every tier of the scan")
	require.Contains(t, two, "https://github.com/acme/widgets/pull/456")

	require.NotContains(t, readFile(t, r, "issues/ISU-3/"+importer.SourceFileName),
		"resolved_by")
}

func TestTheScanCanBeTurnedOff(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--scan=false", "--write").ok(t)

	require.NotContains(t, readFile(t, r, "issues/ISU-1/"+importer.SourceFileName),
		"resolved_by")
	require.Contains(t, readFile(t, r, "issues/ISU-2/"+importer.SourceFileName),
		"resolved_by", "the source's own answer needs no scan")
}

func TestTheJSONReportIsTheDocumentedShape(t *testing.T) {
	t.Parallel()

	r := importable(t)

	got := decode[ImportPayload](t,
		isu(t, r.Dir(), "--json", "import", "github", "--dump", dumpPath).ok(t))

	require.Equal(t, "github", got.Source)
	require.Equal(t, "acme/widgets", got.Repository)
	require.Equal(t, "ISU", got.Prefix)
	require.False(t, got.Wrote)
	require.Equal(t, 5, got.Items,
		"four issues once the pull request is gone, and the milestone that became "+
			"an epic")
	require.Equal(t, 4, got.Folders, "the fifth has no owner and no --owner")
	require.Equal(t, 1, got.Epics)
	require.Equal(t, 2, got.Comments)
	require.Equal(t, 1, got.Attachments)
	require.Equal(t, 12, got.Fields, "everything the schema has no place for, counted")
	require.Equal(t, 2, got.Provenance)
	require.Equal(t, 1, got.Unowned)
	require.Equal(t, 1, got.Dangling)
	require.Zero(t, got.Requests, "a dump costs nothing")
	require.Equal(t, map[string]int{"bug": 1, "story": 1, "chore": 1, "epic": 1}, got.Types)
	require.Equal(t, []string{"enhancement"}, got.Unplaced)
	require.Contains(t, got.Found, "milestones")
	require.NotEmpty(t, got.Notes)
	require.Len(t, got.Skipped, 2)
	require.Len(t, got.Samples, importer.DefaultSamples)
	require.Empty(t, got.Paths)
	require.Contains(t, got.Evidence, "closing pull request")
}

func TestAnIDPrefixOfItsOwn(t *testing.T) {
	t.Parallel()

	r := importable(t)

	isu(t, r.Dir(), "import", "github", "--dump", dumpPath,
		"--owner", "support", "--id-prefix", "ACME", "--write").ok(t)

	require.FileExists(t, filepath.Join(r.Dir(), "issues", "ACME-1", issue.ReadmeName))
	require.Contains(t, readFile(t, r, "issues/ACME-1/"+issue.ReadmeName), "parent: ACME-M1")
}

func TestOnlySomeStates(t *testing.T) {
	t.Parallel()

	r := importable(t)

	got := decode[ImportPayload](t, isu(t, r.Dir(), "--json", "import", "github",
		"--dump", dumpPath, "--state", "open", "--owner", "support").ok(t))

	require.Equal(t, map[string]int{"open": 2, "(fold over its children)": 1}, got.States)
}

func TestALabelMapPlacesWhatGitHubsOwnTypesDoNot(t *testing.T) {
	t.Parallel()

	r := importable(t)

	got := decode[ImportPayload](t, isu(t, r.Dir(), "--json", "import", "github",
		"--dump", dumpPath, "--owner", "support", "--type-map", "enhancement=spike").ok(t))

	require.Equal(t, 1, got.Types["spike"])
	require.Empty(t, got.Unplaced)
}

func TestATypeMapThatIsNotOne(t *testing.T) {
	t.Parallel()

	r := importable(t)

	entries := []string{"enhancement", "=story", "enhancement=nonsense", "enhancement=epic"}

	for _, entry := range entries {
		got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath, "--type-map", entry)

		require.Equalf(t, 2, got.code, "%q", entry)
		require.Contains(t, got.stderr, "--type-map")
	}
}

func TestImportNeedsASourceItKnows(t *testing.T) {
	t.Parallel()

	r := configured(t)

	for _, args := range [][]string{
		{"import"},
		{"import", "github", "acme/widgets", "extra"},
		{"import", "jira", "PROJ"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 2, got.code)
		require.Contains(t, got.stderr, "github")
	}
}

func TestADumpThatIsNotThere(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "--dump", "nowhere.json")
	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "reading the dump")
}

func TestAnAPIImportNeedsARepository(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github")
	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "owner/repo")
}

func TestAPrefixThatCannotBeOne(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath, "--id-prefix", "not/a/prefix")
	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "cannot be an id prefix")
}

func TestAStateThatIsNotOne(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "--dump", dumpPath, "--state", "ajar")
	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "--state is one of")
}

func TestGoldenImport(t *testing.T) {
	t.Parallel()

	r := importable(t)

	golden(t, "import/dry-run.txt",
		isu(t, r.Dir(), "--no-color", "import", "github", "--dump", dumpPath,
			"--owner", "support", "--samples", "1").ok(t).stdout)
}

// readFile is one file out of the working tree, as the import left it.
func readFile(t *testing.T, r *gittest.Repo, path string) string {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(r.Dir(), filepath.FromSlash(path)))
	require.NoError(t, err)

	return string(body)
}

func TestImportSomewhereThatIsNotARepository(t *testing.T) {
	t.Parallel()

	got := isu(t, t.TempDir(), "import", "github", "--dump", dumpPath)
	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "not inside a git working tree")
}

func TestADumpThatIsNotJSON(t *testing.T) {
	t.Parallel()

	r := configured(t)

	path := filepath.Join(t.TempDir(), "broken.json")
	require.NoError(t, os.WriteFile(path, []byte("{not json at all"), 0o644))

	got := isu(t, r.Dir(), "import", "github", "--dump", path)
	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "reading the dump")
}

func TestAnIssueTheSchemaWouldRefuseStopsTheImport(t *testing.T) {
	t.Parallel()

	r := configured(t)

	path := filepath.Join(t.TempDir(), "undated.json")
	require.NoError(t, os.WriteFile(path, []byte(
		`[{"number":1,"title":"No date at all","state":"open",`+
			`"assignees":[{"login":"dmitry"}]}]`), 0o644))

	got := isu(t, r.Dir(), "import", "github", "--dump", path)
	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "would not be a valid issue")
	require.Contains(t, got.stderr, "created")
}

func TestAnImportOfNothingWritesNothingAndSaysSo(t *testing.T) {
	t.Parallel()

	r := configured(t)

	path := filepath.Join(t.TempDir(), "empty.json")
	require.NoError(t, os.WriteFile(path, []byte("[]"), 0o644))

	got := isu(t, r.Dir(), "--no-color", "import", "github", "acme/widgets",
		"--dump", path, "--write").ok(t)

	require.Contains(t, got.stdout, "wrote 0 folders from 0 issues")
	require.Contains(t, got.stdout, "types: none")
	require.NotContains(t, got.stdout, "skipped")
	require.NoDirExists(t, filepath.Join(r.Dir(), "issues"))
}

// --fetch-only writes the dump and stops, which is what makes an import
// reproducible without a network and reviewable as a diff. The API it reads is
// one this test runs.
func TestFetchOnlyWritesADumpAndStops(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/comments") || r.URL.Query().Get("page") != "1" {
			_, _ = w.Write([]byte("[]"))

			return
		}

		require.Empty(t, r.Header.Get("Authorization"),
			"no token in the environment is a public repository, and those answer")

		_, _ = w.Write([]byte(`[{"number":1,"title":"From the API","state":"open",` +
			`"created_at":"2026-03-04T09:00:00Z","assignees":[{"login":"dmitry"}]}]`))
	}))
	defer server.Close()

	r := configured(t)
	path := filepath.Join(t.TempDir(), "dump.json")

	got := isu(t, r.Dir(), "import", "github", "acme/widgets",
		"--api", server.URL, "--fields=false", "--fetch-only", path).ok(t)

	require.Contains(t, got.stdout, path)
	require.NoDirExists(t, filepath.Join(r.Dir(), "issues"), "--fetch-only maps nothing")

	// And the dump it wrote is one this importer reads.
	again := decode[ImportPayload](t,
		isu(t, r.Dir(), "--json", "import", "github", "--dump", path).ok(t))
	require.Equal(t, 1, again.Folders)
	require.Equal(t, "acme/widgets", again.Repository)
}

func TestFetchOnlyRefusesWhatItCannotDo(t *testing.T) {
	t.Parallel()

	r := configured(t)

	for _, args := range [][]string{
		{"import", "github", "acme/widgets", "--fetch-only", "x.json", "--dump", dumpPath},
		{"import", "github", "--fetch-only", "x.json"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 2, got.code)
		require.Contains(t, got.stderr, "--fetch-only")
	}
}

func TestFetchOnlyReportsWhatItCouldNotWrite(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "acme/widgets",
		"--api", server.URL, "--fetch-only", filepath.Join(t.TempDir(), "nope", "dump.json"))

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "writing the dump")
}

func TestFetchOnlyReportsWhatItCouldNotRead(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "acme/widgets",
		"--api", server.URL, "--fetch-only", filepath.Join(t.TempDir(), "dump.json"))

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "404")
}

func TestAnAPIThatWillNotAnswerIsReported(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	base := server.URL

	server.Close()

	r := configured(t)

	got := isu(t, r.Dir(), "import", "github", "acme/widgets", "--api", base)
	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

// The token is read from the environment, never from a flag: a flag is in the
// shell history, in the process list, and in whatever CI log recorded the
// command line.
func TestTheTokenComesFromTheEnvironment(t *testing.T) {
	t.Parallel()

	var sent string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = r.Header.Get("Authorization")

		_, _ = w.Write([]byte("[]"))
	}))
	defer server.Close()

	repo := configured(t)

	isuIn(t, repo.Dir(), map[string]string{"GITHUB_TOKEN": "ghp_secret"},
		"import", "github", "acme/widgets", "--api", server.URL).ok(t)

	require.Equal(t, "Bearer ghp_secret", sent)
}
