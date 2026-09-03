package importer_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

// One payload per attack, which is what PLAN.md M7-S2 asks for. Everything here
// is a way somebody has escaped a directory before; none of it is hypothetical,
// and the display-name one is the live one, because a comment file is named for
// its author.
func TestOneNameRefusedPerAttack(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"../../etc/passwd":                 "path separator",
		"..":                               "directory",
		".":                                "directory",
		"/etc/passwd":                      "path separator",
		`..\..\windows\system32`:           "path separator",
		"":                                 "needs a name",
		strings.Repeat("a", 4000) + ".md":  "longer than the 255",
		"a\nname\nwith\nnewlines.md":       "control character",
		"bell\aname.md":                    "control character",
		"nul.md":                           "device on Windows",
		"CON":                              "device on Windows",
		"com1.txt":                         "device on Windows",
		"trailing.":                        "dot or a space",
		"trailing ":                        "dot or a space",
		"--output=etc-passwd":              "begins with a hyphen",
		"con.fig.md":                       "device on Windows",
		string([]byte{0x7f}) + "delete.md": "control character",
	} {
		err := importer.SafeName(name)
		require.Errorf(t, err, "%q was allowed", name)
		require.Containsf(t, err.Error(), want, "%q", name)
	}

	require.NoError(t, importer.SafeName("2026-08-24-alice-01.md"))
	require.NoError(t, importer.SafeName("README.md"))
	require.NoError(t, importer.SafeName("console.md"),
		"the device is the name before the first dot, so `con.fig.md` is CON and "+
			"`console.md` is a file")
}

func TestAPathIsCheckedElementByElement(t *testing.T) {
	t.Parallel()

	require.NoError(t, importer.SafePath("comments/2026-08-24-alice-01.md"))

	for _, path := range []string{"", "/etc/passwd", "comments/../../escape.md"} {
		require.Errorf(t, importer.SafePath(path), "%q was allowed", path)
	}
}

// PLAN.md M7-S2's live payload: the comment file is named for the author, and a
// display name is whatever somebody typed into their profile.
func TestACommentAuthorWhoseDisplayNameIsAPathIsStillOneFileInOneFolder(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Comments = []importer.Comment{
		{Author: "../../../etc/passwd", When: created, Body: "hello"},
		{Author: "..", When: created, Body: "hello"},
		{Author: "C:\\Windows\\System32", When: created, Body: "hello"},
	}

	folder := find(t, mapped(t, b, importer.Options{}), "PROJ-1234")

	for _, name := range names(folder) {
		require.NoErrorf(t, importer.SafePath(name), "%q", name)
	}

	require.Equal(t, []string{
		issue.ReadmeName,
		importer.SourceFileName,
		"comments/2026-03-04-etc-passwd-01.md",
		"comments/2026-03-04-anon-01.md",
		"comments/2026-03-04-c-windows-system32-01.md",
	}, names(folder))
}

func TestTheGuardedPathWritesAFolder(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	w := importer.Writer{Root: root}

	folder := find(t, mapped(t, batch(), importer.Options{}), "PROJ-1234")
	folder.Files = append(folder.Files, importer.File{
		Name: "comments/2026-03-04-alice-01.md", Body: []byte("hello\n"),
	})

	wrote, err := w.Write(folder)
	require.NoError(t, err)
	require.Equal(t, []string{
		"issues/PROJ-1234/README.md",
		"issues/PROJ-1234/source.yml",
		"issues/PROJ-1234/comments/2026-03-04-alice-01.md",
	}, wrote)

	on, err := os.ReadFile(filepath.Join(root, "issues", "PROJ-1234", issue.ReadmeName))
	require.NoError(t, err)
	require.Contains(t, string(on), "id: PROJ-1234")
}

func TestTheGuardedPathRefusesAFolderThatIsNotAnID(t *testing.T) {
	t.Parallel()

	_, err := importer.Writer{Root: t.TempDir()}.Write(importer.Folder{ID: "../escape"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be an issue folder")
}

func TestNothingIsWrittenWhenOneNameIsBad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	_, err := importer.Writer{Root: root}.Write(importer.Folder{
		ID: "ISU-1",
		Files: []importer.File{
			{Name: issue.ReadmeName, Body: []byte("first")},
			{Name: "../escape.md", Body: []byte("second")},
		},
	})

	require.Error(t, err)
	require.NoDirExists(t, filepath.Join(root, "issues", "ISU-1"),
		"a folder with one bad name in it writes nothing rather than half of itself")
}

func TestAnImportDoesNotWriteThroughASymbolicLink(t *testing.T) {
	t.Parallel()

	for _, at := range []string{"issues", "issues/ISU-1", "issues/ISU-1/comments"} {
		root := t.TempDir()
		link := filepath.Join(root, filepath.FromSlash(at))

		require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o755))
		require.NoError(t, os.Symlink(t.TempDir(), link))

		_, err := importer.Writer{Root: root}.Write(importer.Folder{
			ID: "ISU-1",
			Files: []importer.File{
				{Name: issue.ReadmeName, Body: []byte("first")},
				{Name: "comments/2026-03-04-alice-01.md", Body: []byte("second")},
			},
		})

		require.Errorf(t, err, "wrote through the link at %s", at)
		require.Contains(t, err.Error(), "symbolic link")
	}
}

func TestTheCapsAreEnforcedPerFileAndPerIssue(t *testing.T) {
	t.Parallel()

	big := importer.Folder{
		ID:    "ISU-1",
		Files: []importer.File{{Name: issue.ReadmeName, Body: make([]byte, 400)}},
	}

	_, err := importer.Writer{Root: t.TempDir(), MaxFileBytes: 100}.Write(big)
	require.Error(t, err)
	require.Contains(t, err.Error(), "a single file may be")

	_, err = importer.Writer{Root: t.TempDir(), MaxIssueBytes: 100}.Write(big)
	require.Error(t, err)
	require.Contains(t, err.Error(), "one issue may be")

	_, err = importer.Writer{Root: t.TempDir()}.Write(big)
	require.NoError(t, err, "the defaults are generous: an issue is not a disk image")
}

func TestAnIssueThatIsAlreadyHereIsNotOverwritten(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	w := importer.Writer{Root: root}

	folder := find(t, mapped(t, batch(), importer.Options{}), "ISU-7")

	_, err := w.Write(folder)
	require.NoError(t, err)

	// The same import again: idempotent, because it is the same issue.
	_, err = w.Write(folder)
	require.NoError(t, err)

	// A different repository's #7, under the same prefix. The key is the same
	// `#7` on both sides — which is exactly why the ref is what is compared.
	other := folder
	other.Ref = "other/repo#7"

	_, err = w.Write(other)
	require.Error(t, err)
	require.Contains(t, err.Error(), "refuses rather than overwriting")

	// And an issue isu wrote itself, which has no source.yml at all.
	require.NoError(t, os.Remove(filepath.Join(root, "issues", "ISU-7", importer.SourceFileName)))

	_, err = w.Write(folder)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no import wrote it")
}

func TestAnIssueOverThePerIssueCapIsRefusedRatherThanTruncated(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items[0].Body = strings.Repeat("a very long line about nothing\n", 40)

	folder := find(t, mapped(t, b, importer.Options{}), "PROJ-1234")

	_, err := importer.Writer{Root: t.TempDir(), MaxIssueBytes: 512}.Write(folder)
	require.Error(t, err)
	require.Contains(t, err.Error(), "past the 512 one issue may be")
}

// PLAN.md M7-S2: "a token interpolated into an error message".
func TestATokenNeverReachesAMessage(t *testing.T) {
	t.Parallel()

	const value = "ghp_0123456789abcdefghijklmnopqrstuvwxyz"

	token := importer.Secret(value)

	require.Equal(t, importer.Redacted, token.String())
	require.Equal(t, "sent "+importer.Redacted, fmt.Sprintf("sent %s", token))
	require.Equal(t, importer.Redacted, fmt.Sprintf("%v", token))
	require.Equal(t, importer.Redacted, fmt.Sprintf("%#v", token))
	require.Equal(t, value, token.Reveal(), "one way to read it, spelled to be greppable")

	encoded, err := json.Marshal(struct {
		Token importer.Secret `json:"token"`
	}{token})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), value)

	// The half no type system reaches: a message isu did not write, from a
	// transport that echoed the request it failed on.
	echoed := fmt.Errorf("Get \"https://api.github.com/x?access_token=%s\": refused", value)

	scrubbed := importer.Redact(echoed, token)
	require.NotContains(t, scrubbed.Error(), value)
	require.Contains(t, scrubbed.Error(), importer.Redacted)

	require.NoError(t, importer.Redact(nil, token))

	clean := errors.New("nothing secret here")
	require.Equal(t, clean, importer.Redact(clean, token, importer.Secret("")),
		"an error with nothing to scrub is the error itself, wrappers and all")
}

// The grep test PLAN.md M7-S2 asks for, in the same style as M2-S1's: **every
// importer writes exclusively through the guarded path**, and this is what
// stops that being true only on the day it was written.
func TestOnlySafeWritesTouchesTheFilesystem(t *testing.T) {
	t.Parallel()

	// safe.go is the guarded path itself. Everything else in this package and
	// under it may read, map and report, and may not create a file.
	const exempt = "safe.go"

	writes := map[string]bool{
		"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true,
		"MkdirAll": true, "Mkdir": true, "MkdirTemp": true, "Rename": true,
		"Remove": true, "RemoveAll": true, "Symlink": true, "Link": true,
		"Chmod": true, "Truncate": true, "WriteString": true,
	}

	require.NoError(t, filepath.Walk("..", func(path string, info os.FileInfo, err error) error {
		require.NoError(t, err)

		base := filepath.Base(path)

		switch {
		case info.IsDir(), !strings.HasSuffix(base, ".go"):
			return nil
		case strings.HasSuffix(base, "_test.go"), base == exempt:
			return nil
		case !strings.Contains(filepath.ToSlash(path), "/importer/"):
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		require.NoError(t, err)

		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			pkg, ok := call.X.(*ast.Ident)
			if !ok || pkg.Name != "os" {
				return true
			}

			require.Falsef(t, writes[call.Sel.Name],
				"%s calls os.%s: every importer writes through importer.Writer, "+
					"and %s is the only file that may touch the filesystem",
				path, call.Sel.Name, exempt)

			return true
		})

		return nil
	}))
}

func TestAFilesystemThatWillNotTakeTheFolderIsReported(t *testing.T) {
	t.Parallel()

	// The two ways a write fails once every name has been checked: something
	// else is sitting where the folder belongs, and something else is sitting
	// where the file belongs.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "issues"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "issues", "ISU-1"), nil, 0o644))

	_, err := importer.Writer{Root: root}.Write(importer.Folder{
		ID:    "ISU-1",
		Files: []importer.File{{Name: issue.ReadmeName, Body: []byte("x")}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "making the folder for")

	root = t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, "issues", "ISU-1", "comments", "2026-03-04-alice-01.md"), 0o755))

	_, err = importer.Writer{Root: root}.Write(importer.Folder{
		ID:    "ISU-1",
		Files: []importer.File{{Name: "comments/2026-03-04-alice-01.md", Body: []byte("x")}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "writing comments/2026-03-04-alice-01.md")
}

func TestAFolderWhoseSourceFileIsNotYAMLIsNotOverwritten(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "issues", "ISU-7")

	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, issue.ReadmeName), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, importer.SourceFileName), []byte("\tnot: [yaml"), 0o644))

	_, err := importer.Writer{Root: root}.Write(
		find(t, mapped(t, batch(), importer.Options{}), "ISU-7"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "no import wrote it")
}
