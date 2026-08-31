package cli

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
	"github.com/dgorshkov/isu/internal/gitx"
)

// The failure paths, gathered.
//
// PLAN.md's definition of done puts the coverage floor at 99% of the whole
// module and says out loud what that is for: "a story that adds an error path
// it cannot reach should expect to argue for it". These are the arguments — an
// error return nobody has ever taken is a message nobody has ever read, and
// half of what a command is, is what it says when it cannot do the thing.
//
// Three levers do most of the work, and none of them depends on the test
// running as an unprivileged user — the suite runs as root in some containers,
// where a chmod proves nothing:
//
//   - a directory that is not a repository, which every command has to survive;
//   - a --ref that does not resolve, which fails the load every read starts with;
//   - a path that cannot be a file because something else is already there,
//     which is how a write fails without anybody's permissions changing.

// everyCommand is one invocation of each, for the failures that are the same
// failure in all of them.
func everyCommand() [][]string {
	return [][]string{
		{"board"},
		{"show", "ISU-openly"},
		{"ready"},
		{"new", "--title", "x", "--repro", "y"},
		{"claim", "ISU-openly"},
		{"unclaim", "ISU-openly"},
		{"resolve", "ISU-openly"},
		{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
		{"comment", "ISU-openly", "-m", "hello"},
		{"triage", "ISU-openly", "--owner", "dmitry"},
		{"init", "--prefix", "NEW"},
	}
}

func TestEveryCommandSurvivesSomewhereThatIsNotARepository(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	for _, args := range everyCommand() {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()

			got := isu(t, dir, args...)

			require.Equal(t, 1, got.code)
			require.Contains(t, got.stderr, "not inside a git working tree")
		})
	}
}

func TestEveryReadingCommandSurvivesARefThatDoesNotResolve(t *testing.T) {
	t.Parallel()

	r := board(t)

	// Every read starts by loading trunk and every branch beside it, so a ref
	// that names nothing fails there rather than three functions later.
	for _, args := range [][]string{
		{"board"},
		{"show", "ISU-openly"},
		{"ready"},
		{"new", "--title", "x", "--repro", "y"},
		{"claim", "ISU-openly"},
		{"triage", "ISU-openly", "--owner", "dmitry"},
	} {
		t.Run(args[0], func(t *testing.T) {
			t.Parallel()

			got := isu(t, r.Dir(), append([]string{"--ref", "refs/heads/no-such-ref"}, args...)...)

			require.Equal(t, 1, got.code)
			require.NotEmpty(t, got.stderr)
		})
	}
}

func TestAWriteRefusesAPathSomethingElseIsAlreadySittingOn(t *testing.T) {
	t.Parallel()

	// A directory where the issue file belongs. Nothing can write a file there,
	// and no permission bit is involved — which matters, because this suite
	// runs as root often enough that a chmod would prove nothing.
	r := claimable(t)

	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)
	r.Checkout("isu/ISU-openly")

	readme := filepath.Join(r.Dir(), "issues", "ISU-openly", "README.md")
	require.NoError(t, os.Remove(readme))
	require.NoError(t, os.Mkdir(readme, 0o755))

	// unclaim reads the issue out of git and writes it through the working
	// tree, so the read succeeds and the write is what fails.
	got := isu(t, r.Dir(), "unclaim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "issues/ISU-openly/README.md")
}

func TestResolveSurvivesAnIssueFileItCannotRead(t *testing.T) {
	t.Parallel()

	r := onBranch(t)

	readme := filepath.Join(r.Dir(), "issues", "ISU-openly", "README.md")
	require.NoError(t, os.Remove(readme))
	require.NoError(t, os.Mkdir(readme, 0o755))

	for _, args := range [][]string{
		{"resolve", "ISU-openly"},
		{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 1, got.code)
		require.Contains(t, got.stderr, "issues/ISU-openly/README.md")
	}
}

func TestResolveSurvivesAnIssueFileThatWillNotDecode(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.WriteFile("issues/ISU-openly/README.md", "no frontmatter at all\n")

	got := isu(t, r.Dir(), "resolve", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "ISU-openly")
}

func TestNewSurvivesAFolderItCannotWrite(t *testing.T) {
	t.Parallel()

	r := configured(t)

	// issues/ itself is a file, so no issue folder can be made under it.
	r.File("issues", "not a directory\n").Commit("issues is a file here")

	for _, args := range [][]string{
		{"new", "--no-branch", "--title", "x", "--repro", "y"},
		{"new", "--title", "x", "--repro", "y"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 1, got.code)
		require.Contains(t, got.stderr, "issues")
	}
}

func TestAWriteThatGitRefusesToStageIsReported(t *testing.T) {
	t.Parallel()

	// git refuses to add a path its ignore rules exclude, which is the one way
	// a staging failure happens to somebody who has done nothing wrong: they
	// ignored a directory and forgot.
	r := configured(t).
		File(".gitignore", "issues/ISU-ignore/comments/\n").
		Commit("ignore something")

	r.Issue("ISU-ignore", gittest.Owner("dmitry"), gittest.Type("chore"),
		gittest.Title("Its comments are ignored")).
		Commit("report ISU-ignore")

	got := isu(t, r.Dir(), "comment", "ISU-ignore", "-m", "this cannot be staged")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "ignored")
}

func TestCommandsSurviveARepositoryWithNoIdentity(t *testing.T) {
	// Not parallel: taking the identity away means taking the global and system
	// configuration away too, and that is process-wide.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)

	r := onBranch(t)
	r.Git("config", "--unset", "user.email")
	r.Git("config", "--unset", "user.name")

	// git will not write a commit for somebody it cannot name.
	for _, args := range [][]string{
		{"resolve", "ISU-openly"},
		{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 1, got.code)
		require.NotEmpty(t, got.stderr)
	}

	// And a report branch is created and then cannot be committed to.
	filed := isu(t, r.Dir(), "new", "--title", "Nobody to file it", "--repro", "x",
		"--owner", "dmitry")
	require.Equal(t, 1, filed.code)
}

func TestUnclaimSurvivesABranchWithoutTheIssueOnIt(t *testing.T) {
	t.Parallel()

	r := configured(t)
	before := r.Head()

	r.Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
		gittest.Title("Reported later")).
		Commit("report ISU-openly")

	// A branch in the claim namespace cut from before the issue existed.
	r.Git("branch", "isu/ISU-openly", before)

	got := isu(t, r.Dir(), "unclaim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "ISU-openly")
}

func TestUnclaimAndTriageSurviveARemoteThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	r := claimable(t)
	isu(t, r.Dir(), "claim", "ISU-openly").ok(t)

	r.DetachRemote()

	unclaimed := isu(t, r.Dir(), "unclaim", "ISU-openly")
	require.Equal(t, 1, unclaimed.code)

	// And the state flip is still on the branch: what failed was telling
	// anybody about it.
	require.Contains(t, r.Git("show", "isu/ISU-openly:issues/ISU-openly/README.md"),
		"state: open")
}

func TestDirectTriagePushSurvivesARemoteThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\ndirect_triage: true\n").WithRemote().DetachRemote()

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry", "--push")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestInitSaysWhatIsWrongWithEveryWayOfGettingItWrong(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		dir  func(t *testing.T) string
		code int
		want string
	}{
		{
			name: "with no prefix",
			args: []string{"init"},
			dir:  func(t *testing.T) string { return bare(t) },
			code: 2,
			want: "--prefix",
		},
		{
			name: "with a prefix that is not a legal folder name",
			args: []string{"init", "--prefix", "not/a/prefix"},
			dir:  func(t *testing.T) string { return bare(t) },
			code: 1,
			want: "prefix",
		},
		{
			name: "somewhere that is not a repository",
			args: []string{"init", "--prefix", "NEW"},
			dir:  func(t *testing.T) string { return t.TempDir() },
			code: 1,
			want: "not inside a git working tree",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isu(t, tt.dir(t), tt.args...)

			require.Equal(t, tt.code, got.code)
			require.Contains(t, got.stderr, tt.want)
		})
	}
}

func TestInitWritesTheFileAndThenRefusesToOverwriteIt(t *testing.T) {
	t.Parallel()

	dir := bare(t)

	got := isu(t, dir, "init", "--prefix", "NEW").ok(t)

	require.Contains(t, got.stdout, ".isu.yml",
		"the human form names the file, because init has no issue to name")

	written, err := os.ReadFile(filepath.Join(dir, ".isu.yml"))
	require.NoError(t, err)
	require.Contains(t, string(written), "prefix: NEW")

	// Everything else works now, which is the whole point of the command.
	isu(t, dir, "board").ok(t)

	again := isu(t, dir, "init", "--prefix", "OTHER")

	require.Equal(t, 1, again.code)
	require.Contains(t, again.stderr, "already has",
		"the prefix in it is part of every id already written")
}

func TestEmptyTemporaryDirectoryBreaksTheCommandsThatNeedOne(t *testing.T) {
	// Not parallel: TMPDIR is process-wide, and the point is that the process
	// cannot make a temporary directory at all.
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "does-not-exist"))

	r := claimable(t)

	// The claim builds its commit through a temporary index, and `isu comment`
	// opens the editor on a temporary file. Neither can be made here.
	claimed := isu(t, r.Dir(), "claim", "ISU-openly")
	require.Equal(t, 1, claimed.code)

	commented := isuIn(t, r.Dir(), map[string]string{"EDITOR": "true"},
		"comment", "ISU-openly")
	require.Equal(t, 1, commented.code)
}

func TestTheProcessEnvironmentAndClockAreTheDefaults(t *testing.T) {
	// Not parallel: it runs isu with no working directory of its own, which
	// makes the process's own the one under test — and that is this package's
	// directory, inside the isu repository.
	var stdout, stderr strings.Builder

	code := Run(Env{Args: []string{"board"}, Stdout: &stdout, Stderr: &stderr})

	require.Equalf(t, 0, code, "isu board on the isu repository itself: %s", stderr.String())
	require.Contains(t, stdout.String(), "issue",
		"PLAN.md M4-S2 is done when isu board renders this repository")
}

func TestAClosedFileIsNotATerminal(t *testing.T) {
	t.Parallel()

	device, err := osOpenDevNull()
	require.NoError(t, err)
	require.NoError(t, device.Close())

	// A stream that cannot be asked what it is, is not one to write escape
	// sequences at.
	require.False(t, isCharDevice(device))
}

func TestShortAndReportWriteHandleWhatTheyAreHandedNothingOf(t *testing.T) {
	t.Parallel()

	require.Equal(t, "abc", short("abc"))
	require.Equal(t, "abcdefgh", short("abcdefgh12345"))

	var out strings.Builder

	a := &app{env: Env{Stdout: &out, Getenv: func(string) string { return "" }}}

	require.NoError(t, a.reportWrite(Write{ID: "ISU-openly"}))
	require.Equal(t, "ISU-openly\n", out.String())
}

// bare is a repository with a commit and no isu configuration, which is what
// somebody adopting isu is standing in.
func bare(t *testing.T) string {
	t.Helper()

	return gittest.New(t).File("README.md", "a repository\n").Commit("first").Dir()
}

func TestShowSurvivesAFolderThatIsNotAFolder(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Its folder goes away")).
		Commit("report ISU-openly")

	// Trunk has a folder; the working tree has a file where it was.
	dir := filepath.Join(r.Dir(), "issues", "ISU-openly")
	require.NoError(t, os.RemoveAll(dir))
	require.NoError(t, os.WriteFile(dir, []byte("not a folder\n"), 0o600))

	got := isu(t, r.Dir(), "show", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "ISU-openly")
}

func TestShowAndCommentSurviveACommentsPathThatIsNotADirectory(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("Its comments are a file")).
		Commit("report ISU-openly")

	r.WriteFile("issues/ISU-openly/comments", "a file where a directory belongs\n")

	shown := isu(t, r.Dir(), "show", "ISU-openly")
	require.Equal(t, 1, shown.code)
	require.Contains(t, shown.stderr, "comments")

	// And the command that would put one there says the same.
	commented := isu(t, r.Dir(), "comment", "ISU-openly", "-m", "hello")
	require.Equal(t, 1, commented.code)
	require.Contains(t, commented.stderr, "comments")
}

func TestShowIgnoresWhatIsNotACommentAndReadsWhatIs(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("A folder with odds and ends in it"),
			gittest.Comment("2026-08-24-support-01", "the first\n"),
			gittest.Comment("2026-08-24-support-02", "the second\n")).
		Commit("report ISU-openly")

	// A directory and a file that is not markdown, both inside comments/.
	require.NoError(t, os.Mkdir(
		filepath.Join(r.Dir(), "issues", "ISU-openly", "comments", "drafts"), 0o755))
	r.WriteFile("issues/ISU-openly/comments/notes.txt", "not a comment\n")

	payload := decode[ShowPayload](t, isu(t, r.Dir(), "--json", "show", "ISU-openly").ok(t))

	require.Len(t, payload.Comments, 2, "anything that is not a .md file is not a comment")
	require.Equal(t, "the first\n", payload.Comments[0].Body)
}

func TestShowSurvivesACommentItCannotRead(t *testing.T) {
	t.Parallel()

	r := configured(t).
		Issue("ISU-openly", gittest.Owner("dmitry"), gittest.Type("chore"),
			gittest.Title("One of its comments is a broken link")).
		Commit("report ISU-openly")

	comments := filepath.Join(r.Dir(), "issues", "ISU-openly", "comments")
	require.NoError(t, os.MkdirAll(comments, 0o755))

	// A symbolic link to nothing is a file by every test but reading it.
	require.NoError(t, os.Symlink(
		filepath.Join(comments, "nowhere"), filepath.Join(comments, "dangling.md")))

	got := isu(t, r.Dir(), "show", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "dangling.md")
}

func TestARepositoryWhoseConfigurationWillNotParse(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).
		File(".isu.yml", "prefix:\n  - not\n  - a\n  - string\n").
		Commit("a configuration nobody can read")

	got := isu(t, r.Dir(), "board")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "prefix")
}

func TestAnIssueWithASchemaThisBuildDoesNotReadIsRefusedByTheWriters(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-future/README.md",
			"---\nschema: 99\nid: ISU-future\ntitle: From the future\n"+
				"type: chore\nstate: open\nowner: dmitry\ncreated: 2026-08-31\n---\n").
		Commit("an issue from a later isu").
		Branch("side").Checkout("side")

	// A reader refuses a version it does not know rather than misparsing it,
	// and every command that writes has to say so rather than guess.
	for _, args := range [][]string{
		{"resolve", "ISU-future"},
		{"triage", "ISU-future", "--owner", "dmitry"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 1, got.code)
		require.Contains(t, got.stderr, "ISU-future")
	}
}

func TestClaimAndTriageSurviveAnIssueTheyCannotDecode(t *testing.T) {
	t.Parallel()

	r := configured(t).
		File("issues/ISU-broken/README.md", "no frontmatter here\n").
		Commit("a half-written issue").
		WithRemote()

	// The board renders it — that is the loader's deliberate choice — and the
	// commands that have to read the file to change it cannot.
	isu(t, r.Dir(), "board").ok(t)

	for _, args := range [][]string{
		{"claim", "ISU-broken"},
		{"triage", "ISU-broken", "--owner", "dmitry"},
	} {
		got := isu(t, r.Dir(), args...)

		require.Equal(t, 1, got.code)
		require.Contains(t, got.stderr, "ISU-broken")
	}
}

func TestTriageSurvivesABranchNameItCannotCreate(t *testing.T) {
	t.Parallel()

	r := triageable(t, "prefix: ISU\n")

	// A branch called `triage` makes `triage/ISU-triage` impossible: git stores
	// refs as paths, and a file cannot also be a directory.
	r.Branch("triage")

	got := isu(t, r.Dir(), "triage", "ISU-triage", "--owner", "dmitry")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
}

func TestClaimSurvivesABranchNameItCannotCreate(t *testing.T) {
	t.Parallel()

	r := claimable(t)
	r.Branch("isu")

	got := isu(t, r.Dir(), "claim", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.NotEmpty(t, got.stderr)
	require.NotContains(t, branches(t, r.Dir()), "isu/ISU-openly")
}

func TestAnEditorThatTakesTheFileAwayIsReported(t *testing.T) {
	t.Parallel()

	if runtime.GOOS == "windows" {
		t.Skip("the editor stand-in is a shell script; CI is linux and macos")
	}

	r := commentable(t)

	editor := filepath.Join(t.TempDir(), "editor")
	require.NoError(t, os.WriteFile(editor,
		[]byte("#!/bin/sh\nrm \"$1\" && mkdir \"$1\"\n"), 0o700)) //nolint:gosec // a test's own script

	got := isuIn(t, r.Dir(), map[string]string{"EDITOR": editor}, "comment", "ISU-openly")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "reading what you wrote")
}

func TestReadyOnAStreamThatWillNotTakeIt(t *testing.T) {
	t.Parallel()

	r := board(t)

	var stderr strings.Builder

	code := Run(Env{
		Args:   []string{"--repo", r.Dir(), "ready"},
		Stdout: refusingWriter{},
		Stderr: &stderr,
		Dir:    r.Dir(),
		Now:    now,
		Getenv: func(string) string { return "" },
	})

	require.Equal(t, 1, code, "a stream that will not take the answer is a failure")
	require.Contains(t, stderr.String(), "no room")
}

func TestATrunkIsuCannotNameIsAskedForRatherThanGuessed(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.Git("branch", "-m", gittest.DefaultBranch, "development")

	// Neither main nor master, and no origin to ask. isu reads HEAD as trunk,
	// which means it cannot tell whether this branch is trunk — and refusing
	// every branch or accepting every branch are both wrong.
	got := isu(t, r.Dir(), "resolve", "ISU-openly")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "cannot tell what trunk is called")
	require.Contains(t, got.stderr, "--ref")

	// Named once, it works, and still refuses trunk itself.
	isu(t, r.Dir(), "--ref", "development", "resolve", "ISU-openly").ok(t)

	r.Checkout("development")

	onTrunk := isu(t, r.Dir(), "--ref", "development", "resolve", "ISU-openly")
	require.Equal(t, 2, onTrunk.code)
	require.Contains(t, onTrunk.stderr, "you are on development")
}

func TestTheRemoteSaysWhatTrunkIsCalled(t *testing.T) {
	t.Parallel()

	r := onBranch(t)
	r.Checkout(gittest.DefaultBranch)
	r.Git("branch", "-m", gittest.DefaultBranch, "development")
	r.WithRemote()

	// A clone records the forge's default branch in origin/HEAD, which is the
	// only thing in a repository that knows trunk is called development.
	r.Git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/development")

	payload := decode[BoardPayload](t, isu(t, r.Dir(), "--json", "board").ok(t))

	require.Equal(t, "development", payload.Trunk,
		"a repository that has told git once should not have to tell isu again")
}

func TestInitFromTheProcessWorkingDirectory(t *testing.T) {
	// Not parallel: it runs with no working directory of its own, which makes
	// the process's own the one under test — this package's, inside the isu
	// repository, which already has an .isu.yml and must not be overwritten.
	var stdout, stderr strings.Builder

	code := Run(Env{
		Args:   []string{"init", "--prefix", "NEVER"},
		Stdout: &stdout,
		Stderr: &stderr,
	})

	require.Equal(t, 1, code)
	require.Contains(t, stderr.String(), "already has")
}

// refusingWriter is a stream with no room on it, which is what a full disk and
// a closed pipe both look like from here.
type refusingWriter struct{}

func (refusingWriter) Write([]byte) (int, error) {
	return 0, errNoRoom
}

var errNoRoom = errors.New("no room on this stream")

func TestWhereACommitStartsFrom(t *testing.T) {
	t.Parallel()

	s := openSession(t, configured(t).Dir())

	trunk := "refs/heads/" + gittest.DefaultBranch

	// A branch that is already there is all three answers at once: where the
	// commit starts, what the ref must not have moved from, and its parent.
	old, base, parents, err := s.startFrom(t.Context(), trunk, "")
	require.NoError(t, err)
	require.Len(t, parents, 1)
	require.Equal(t, parents[0], old)
	require.Equal(t, parents[0], base)

	// A branch that is not there starts where the caller said.
	old, base, parents, err = s.startFrom(t.Context(), "refs/heads/absent", trunk)
	require.NoError(t, err)
	require.Equal(t, gitx.ZeroOID, old)
	require.Equal(t, trunk, base)
	require.Len(t, parents, 1)

	// Neither resolving is an empty repository, whose first commit builds from
	// nothing and hangs off nothing.
	old, base, parents, err = s.startFrom(t.Context(), "refs/heads/absent", "refs/heads/nothing")
	require.NoError(t, err)
	require.Equal(t, gitx.ZeroOID, old)
	require.Empty(t, base)
	require.Empty(t, parents)

	// And a base git refuses for any other reason is reported rather than read
	// as an empty repository. A reflog entry this repository is far too young
	// to have is such a reason: git knows exactly what was asked for and says
	// it does not go back that far.
	_, _, _, err = s.startFrom(t.Context(), "refs/heads/absent", "HEAD@{999}")
	require.Error(t, err)
	require.NotErrorIs(t, err, gitx.ErrUnknownRevision)
}
