// Package repo loads issues out of a git repository.
//
// It is the layer between internal/gitx, which knows how to run git and
// nothing about issues, and internal/model, which derives statuses and is a
// pure function over what this package hands it. Everything that costs a git
// process happens here, so that M3's "no git calls inside" rule stays a rule
// rather than something M3-S4 has to break.
//
// Three sources, one shape:
//
//   - LoadRef reads a commit, using the read path PLAN.md §0 mandates;
//   - LoadWorktree reads what is on disk, uncommitted edits included;
//   - LoadHistory reads the states an issue's file has held at trunk.
package repo

import (
	"fmt"
	"sort"
	"strings"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// IssuesDir is the directory issues live in, at the repository root.
const IssuesDir = "issues"

// Repo is a git repository isu can read.
type Repo struct {
	root string
	git  *gitx.Git
}

// Option configures a Repo.
type Option func(*options)

type options struct {
	git []gitx.Option
}

// WithGit passes options through to the git binding.
func WithGit(opts ...gitx.Option) Option {
	return func(o *options) { o.git = append(o.git, opts...) }
}

// Open binds to the repository rooted at root.
//
// It runs no git process: a command that is about to fail because the user is
// not in a repository should fail with the message of the thing it was actually
// asked to do.
func Open(root string, opts ...Option) (*Repo, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	g, err := gitx.New(root, o.git...)
	if err != nil {
		return nil, err
	}

	return &Repo{root: root, git: g}, nil
}

// Root is the directory this Repo reads.
func (r *Repo) Root() string { return r.root }

// Git is the binding every git process here goes through.
func (r *Repo) Git() *gitx.Git { return r.git }

// Processes is how many git processes this Repo has spawned. M2-S5 holds the
// read path to a process count, because that is what separates the fast path
// from the slow ones.
func (r *Repo) Processes() int64 { return r.git.Processes() }

// Broken is one issue folder that could not be read at all.
//
// PLAN.md M2-S2 describes the result as a plain map. It cannot be one: M2-S3
// requires that a half-written issue is reported rather than fatal, and a map
// of the issues that loaded has nowhere to say which ones did not. So the
// result is a Set, and this is the half of it a map cannot carry.
type Broken struct {
	// ID is the folder name, which is the id whatever the file says.
	ID string
	// Path is where the unreadable file is, relative to the repository root.
	Path string
	// Err is why it could not be read.
	Err error
}

func (b Broken) String() string { return b.Path + ": " + b.Err.Error() }

// Set is every issue found at one source.
//
// Issues holds what decoded. Decoding is not validating: a bug with no repro
// and an issue whose id disagrees with its folder are both in here, because
// `isu check` cannot report what the loader refused to hand it. Only a file
// that could not be parsed or decoded at all — no frontmatter, a schema this
// build does not read — is left out, and it is in Broken instead.
type Set struct {
	// Issues are the issues that decoded, keyed by folder name.
	Issues map[string]*issue.Issue
	// Broken are the folders that did not, sorted by id.
	Broken []Broken
}

func newSet() *Set { return &Set{Issues: map[string]*issue.Issue{}} }

// Get returns one issue by id.
func (s *Set) Get(id string) (*issue.Issue, bool) {
	i, ok := s.Issues[id]

	return i, ok
}

// Len is how many issues decoded.
func (s *Set) Len() int { return len(s.Issues) }

// IDs lists the issues in id order, so that two runs over the same repository
// render in the same order.
func (s *Set) IDs() []string {
	ids := make([]string, 0, len(s.Issues))
	for id := range s.Issues {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	return ids
}

// add decodes one issue file into the set, filing a failure under Broken rather
// than throwing it: a half-written issue must not blind the whole board.
func (s *Set) add(id, path string, data []byte) {
	i, err := decode(id, data)
	if err != nil {
		s.Broken = append(s.Broken, Broken{ID: id, Path: path, Err: err})

		return
	}

	s.Issues[id] = i
}

// sortBroken puts the unreadable folders in id order, so that a repository with
// three bad issues complains about them the same way twice running.
func (s *Set) sortBroken() {
	sort.SliceStable(s.Broken, func(a, b int) bool { return s.Broken[a].ID < s.Broken[b].ID })
}

// decode parses one issue file and attaches the folder it came from.
func decode(id string, data []byte) (*issue.Issue, error) {
	doc, err := issue.Parse(data)
	if err != nil {
		return nil, err
	}

	i, err := issue.Decode(doc)
	if err != nil {
		return nil, err
	}

	// Validate holds the id against the folder, so the folder has to be on the
	// issue before anything validates it.
	i.Folder = id

	return i, nil
}

// readmePath is where an issue's file sits inside its folder.
func readmePath(id string) string {
	return IssuesDir + "/" + id + "/" + issue.ReadmeName
}

// issueID reads the id out of a path, and reports whether the path is an
// issue's README at all.
//
// Attachments sit beside the README and comments sit under it; neither is an
// issue, and `issues/README.md` explaining the directory to a newcomer is not
// one either.
func issueID(path string) (string, bool) {
	rest, ok := strings.CutPrefix(path, IssuesDir+"/")
	if !ok {
		return "", false
	}

	id, name, ok := strings.Cut(rest, "/")
	if !ok || name != issue.ReadmeName {
		return "", false
	}

	return id, true
}

// errNotAnID is a folder under issues/ that carries a README but cannot be an
// id. It is reported rather than skipped: a folder somebody named wrong is a
// mistake, and a loader that silently ignores it makes the issue disappear.
func errNotAnID(id string) error {
	return fmt.Errorf(
		"%q cannot be an issue id, so this folder is not an issue: "+
			"an id is letters, digits, a hyphen, an underscore and a full stop", id)
}
