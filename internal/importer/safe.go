package importer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/repo"
)

// This file is the only place in this package that touches the filesystem, and
// TestOnlySafeWritesTouchesTheFilesystem fails the build when that stops being
// true — the same grep test M2-S1 uses to keep git inside internal/gitx.
//
// It has its own story (PLAN.md M7-S2) for one reason: **an importer writes
// attacker-influenced data into your repository.** Ticket titles, comment
// bodies, author display names and custom field values all originate outside
// your control, and on a public repository "outside your control" means anyone
// with a browser. v1.0.0 never serves that data over HTTP — the web UI is out
// of scope and glamour renders to a terminal — so writing it to disk is the
// entire attack surface.
//
// The decompressed-size limit and the content-type sniffing PLAN.md names are
// deferred along with the downloads that needed them: v1.0.0's one importer
// records attachment links and fetches nothing, so a zip-bomb guard here would
// be a defence with no traffic on it and a 99% coverage floor to answer to.
// They return with the first importer that fetches a file, behind this guard.

// The caps. They are bytes rather than a policy because the thing being
// defended against is a source that hands over a body the size of a disk.
const (
	// MaxNameBytes is the longest a single path element may be. Every
	// filesystem anybody runs this on stops somewhere near here.
	MaxNameBytes = 255
	// DefaultMaxFileBytes is the per-file cap: one comment, one README.
	DefaultMaxFileBytes = 1 << 20
	// DefaultMaxIssueBytes is the per-issue cap, across every file in the
	// folder. An issue with ten thousand comments is a denial of service
	// against whoever has to clone this repository afterwards.
	DefaultMaxIssueBytes = 8 << 20
)

// reservedWindows are the device names Windows resolves before it resolves a
// file, with or without an extension. A file called `nul.md` is not a file.
var reservedWindows = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// SafeName reports why name cannot be one element of a path inside an issue
// folder, and nil when it can be.
//
// It is a whitelist wearing a blacklist's clothes: everything below is a way
// somebody has escaped a directory before, and the last rule — no separators of
// either kind — is what makes the list finite.
func SafeName(name string) error {
	switch {
	case name == "":
		return errors.New("a file needs a name")
	case name == "." || name == "..":
		return fmt.Errorf("%q is a directory, not a file name", name)
	case len(name) > MaxNameBytes:
		return fmt.Errorf("a name of %d bytes is longer than the %d a filesystem takes",
			len(name), MaxNameBytes)
	case strings.ContainsAny(name, `/\`):
		return fmt.Errorf("%q carries a path separator, and a file goes where it is put", name)
	case strings.HasPrefix(name, "-"):
		return fmt.Errorf("%q begins with a hyphen, which is an option to every command "+
			"anybody later runs over this folder", name)
	case reservedWindows[strings.ToLower(stem(name))]:
		return fmt.Errorf("%q is a device on Windows, and a device is not a file", name)
	case strings.HasSuffix(name, ".") || strings.HasSuffix(name, " "):
		return fmt.Errorf("%q ends in a dot or a space, which Windows silently trims — "+
			"so it is not the name it looks like", name)
	}

	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%q carries a control character, and a name a terminal "+
				"cannot print is a name nobody can check", name)
		}
	}

	return nil
}

// stem is the name without its extension, which is what Windows resolves a
// device name from.
func stem(name string) string {
	if at := strings.Index(name, "."); at > 0 {
		return name[:at]
	}

	return name
}

// SafePath checks every element of a path relative to an issue folder.
func SafePath(path string) error {
	if path == "" {
		return errors.New("a file needs a name")
	}

	if strings.HasPrefix(path, "/") {
		return fmt.Errorf("%q is an absolute path, and an import writes inside the "+
			"issue's own folder", path)
	}

	for _, element := range strings.Split(path, "/") {
		if err := SafeName(element); err != nil {
			return err
		}
	}

	return nil
}

// Writer is the one path every importer writes through.
type Writer struct {
	// Root is the repository root. Everything lands under Root/issues/<id>/.
	Root string
	// MaxFileBytes and MaxIssueBytes default to the constants above when zero.
	MaxFileBytes  int
	MaxIssueBytes int
}

// Write puts one issue folder on disk and returns the paths it wrote, relative
// to the repository root and slash-separated.
//
// Every name is checked before anything is created, so a folder with one bad
// name in it writes nothing at all rather than half of itself.
func (w Writer) Write(f Folder) ([]string, error) {
	if !issue.ValidID(f.ID) {
		return nil, fmt.Errorf("%q cannot be an issue folder", f.ID)
	}

	if err := w.check(f); err != nil {
		return nil, fmt.Errorf("%s: %w", f.ID, err)
	}

	if err := w.claimed(f); err != nil {
		return nil, err
	}

	var wrote []string

	for _, file := range f.Files {
		rel := repo.IssuesDir + "/" + f.ID + "/" + file.Name

		if err := safeParents(w.Root, rel); err != nil {
			return nil, fmt.Errorf("%s: %w", f.ID, err)
		}

		path := filepath.Join(w.Root, filepath.FromSlash(rel))

		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("making the folder for %s: %w", file.Name, err)
		}

		if err := os.WriteFile(path, file.Body, 0o644); err != nil { //nolint:gosec // an imported issue is not a secret
			return nil, fmt.Errorf("writing %s: %w", file.Name, err)
		}

		wrote = append(wrote, rel)
	}

	return wrote, nil
}

// claimed refuses a folder that is already somebody else's issue.
//
// Two imports into one tracker can collide — one repository's `#7` and
// another's are both `<PREFIX>-7` — and PLAN.md says isu refuses rather than
// overwriting. It cannot refuse every folder that is already there, because
// M7-S5 asks that running one import twice produce a zero-length diff. So the
// question is not "is something here" but "is what is here this issue".
//
// The `ref` in the folder's own source.yml answers it, and the `key` beside it
// would not: a key is repository-relative, so `acme/widgets#7` and
// `other/widgets#7` are both `#7` and the collision this exists to catch is
// exactly the pair that would look identical. A folder isu created by hand has
// no source.yml at all and is never overwritten either.
func (w Writer) claimed(f Folder) error {
	dir := filepath.Join(w.Root, repo.IssuesDir, f.ID)

	if _, err := os.Stat(filepath.Join(dir, issue.ReadmeName)); err != nil {
		return nil
	}

	ref, err := existingRef(filepath.Join(dir, SourceFileName))

	switch {
	case err != nil:
		return fmt.Errorf("%s is already an issue in this repository and no import wrote "+
			"it: ids are permanent, so this import needs a different --id-prefix", f.ID)
	case ref != f.Ref:
		return fmt.Errorf("%s is already %s here and this import would make it %s: two "+
			"imports into one tracker collide, and isu refuses rather than overwriting — "+
			"import under a different --id-prefix", f.ID, ref, f.Ref)
	default:
		return nil
	}
}

// existingRef reads the source reference out of a folder's source.yml.
func existingRef(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	var was struct {
		Ref string `yaml:"ref"`
	}

	if err := yaml.Unmarshal(data, &was); err != nil {
		return "", err
	}

	return was.Ref, nil
}

// check holds a folder to the names and the caps before anything is created.
func (w Writer) check(f Folder) error {
	perFile, perIssue := w.MaxFileBytes, w.MaxIssueBytes
	if perFile == 0 {
		perFile = DefaultMaxFileBytes
	}

	if perIssue == 0 {
		perIssue = DefaultMaxIssueBytes
	}

	total := 0

	for _, file := range f.Files {
		if err := SafePath(file.Name); err != nil {
			return err
		}

		if len(file.Body) > perFile {
			return fmt.Errorf("%s is %d bytes, past the %d a single file may be",
				file.Name, len(file.Body), perFile)
		}

		total += len(file.Body)
	}

	if total > perIssue {
		return fmt.Errorf("%d bytes across %d files, past the %d one issue may be",
			total, len(f.Files), perIssue)
	}

	return nil
}

// safeParents refuses to write through a symlink.
//
// SafeName cannot see this one: every element of the path is an ordinary name,
// and the escape is that one of them is already a link to somewhere else. So
// every element between the repository root and the file is stat'ed without
// following, and a link anywhere along it stops the write — including the
// issue folder itself, which is the case somebody gets by committing a symlink
// named for an issue and waiting for an import.
//
// rel is slash-separated and relative to root, which is where it comes from
// rather than something recovered from an absolute path: a rule about what a
// path may contain should not be enforced against a path something else
// reconstructed.
func safeParents(root, rel string) error {
	at := root

	for _, element := range strings.Split(rel, "/") {
		at = filepath.Join(at, element)

		info, err := os.Lstat(at)
		if err != nil {
			// Nothing is there yet, which is the ordinary case: the import is
			// about to create it, and nothing below it exists either.
			return nil
		}

		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("%s is a symbolic link, and an import does not write "+
				"through one", element)
		}
	}

	return nil
}

// Secret is a credential — an API token — and it never appears in output.
//
// PLAN.md M7-S2 asks for tokens read from the environment or a credential
// helper, never from a flag, never written to disk, and redacted from every log
// line and error string. The first three are the caller's to obey; this type is
// how the fourth stops depending on anybody remembering. A Secret formats as
// `<redacted>` through fmt, through %v and %s alike, so the ordinary way of
// building an error message cannot leak one.
type Secret string

// Redacted is what a secret prints as.
const Redacted = "<redacted>"

func (s Secret) String() string { return Redacted }

// GoString is %#v, which is the other spelling that would otherwise print it.
func (s Secret) GoString() string { return Redacted }

// MarshalJSON keeps a secret out of --json, where a struct holding one would
// otherwise serialise it in full.
func (s Secret) MarshalJSON() ([]byte, error) { return []byte(`"` + Redacted + `"`), nil }

// Reveal is the one way to read the value, and it is spelled to be greppable.
func (s Secret) Reveal() string { return string(s) }

// Redact removes every occurrence of a secret from an error's message.
//
// The type above stops a secret being formatted into a message by isu. This
// stops one arriving in a message isu did not write — a transport that echoes
// the request it failed on, a URL somebody put a token in — which is the half
// no type system reaches.
func Redact(err error, secrets ...Secret) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	for _, s := range secrets {
		if value := s.Reveal(); value != "" {
			message = strings.ReplaceAll(message, value, Redacted)
		}
	}

	if message == err.Error() {
		return err
	}

	return errors.New(message)
}
