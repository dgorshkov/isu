package repo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dgorshkov/isu/internal/issue"
)

// LoadWorktree reads every issue on disk, uncommitted edits included.
//
// `isu ui` renders what the developer is looking at, not what they have
// committed, so this is a different path from LoadRef and a much cheaper one:
// issue folders are one level under issues/, so it is one directory listing and
// one read per issue, with no git process for any of it.
//
// One git process is spent before the walk, on .gitignore. An ignored issue is
// not tracked, so it is not on the board, and answering that from the ignore
// rules by hand would mean reimplementing them — including the ones in
// $GIT_DIR/info/exclude and the user's global excludes file, which is where
// half of them live. `git ls-files` already knows.
//
// PLAN.md M2-S3 says "no git process at all". One is not none, and the
// difference is exactly this: honouring .gitignore was in the same sentence,
// and the two cannot both be true. One constant process is still nothing beside
// the per-blob path, and it is the honest number.
//
// The result is the same shape LoadRef returns, and the two agree exactly on a
// clean checkout — a property the tests hold on 5,000 issues, because
// everything downstream assumes they cannot disagree.
func (r *Repo) LoadWorktree(ctx context.Context) (*Set, error) {
	ignored, err := r.ignored(ctx)
	if err != nil {
		return nil, err
	}

	set := newSet()

	entries, err := os.ReadDir(filepath.Join(r.root, IssuesDir))
	if errors.Is(err, fs.ErrNotExist) {
		return set, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", IssuesDir, err)
	}

	for _, entry := range entries {
		// Only a directory can be an issue. issues/README.md explaining the
		// directory to a newcomer is a file, and not an issue called README.md.
		if !entry.IsDir() {
			continue
		}

		id := entry.Name()
		if ignored[IssuesDir+"/"+id+"/"] || ignored[readmePath(id)] {
			continue
		}

		r.loadFolder(set, id)
	}

	set.sortBroken()

	return set, nil
}

// loadFolder reads one issue folder into the set.
func (r *Repo) loadFolder(set *Set, id string) {
	path := readmePath(id)
	full := filepath.Join(r.root, filepath.FromSlash(path))

	// Lstat rather than Stat: a README that is a symlink reads as its target
	// here and as the target's *path* at a ref, which is the one way these two
	// loaders could disagree about a clean checkout. Refusing it keeps them
	// honest, and says so, rather than quietly resolving it.
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		// A directory under issues/ with no README is not an issue. LoadRef
		// cannot see one either, so neither does this.
		return
	}
	if err != nil {
		set.Broken = append(set.Broken, Broken{ID: id, Path: path, Err: err})

		return
	}
	if !info.Mode().IsRegular() {
		set.Broken = append(set.Broken, Broken{ID: id, Path: path, Err: fmt.Errorf(
			"an issue's %s must be a regular file, and this one is %s",
			issue.ReadmeName, info.Mode().Type())})

		return
	}

	if !issue.ValidID(id) {
		set.Broken = append(set.Broken, Broken{ID: id, Path: path, Err: errNotAnID(id)})

		return
	}

	data, err := os.ReadFile(full) //nolint:gosec // the path is the repository's own
	if err != nil {
		set.Broken = append(set.Broken, Broken{ID: id, Path: path, Err: err})

		return
	}

	set.add(id, path, data)
}

// ignored is every path under issues/ that git would not track.
//
// `--others --ignored --exclude-standard` is git's own answer, rules file by
// rules file, so the loader does not have a second opinion about what
// .gitignore means. `--directory` collapses a wholly ignored folder to one
// entry, which is why the lookup checks the folder as well as the file.
//
// A file that .gitignore matches but git already tracks is not listed, and is
// therefore not ignored here either. That is git's rule and it is the right one:
// an issue somebody committed is on the board whatever a later ignore line says.
func (r *Repo) ignored(ctx context.Context) (map[string]bool, error) {
	out, err := r.git.Output(ctx,
		"ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--directory",
		"--", IssuesDir)
	if err != nil {
		return nil, err
	}

	paths := map[string]bool{}
	for _, path := range strings.Split(strings.TrimSuffix(out, "\x00"), "\x00") {
		if path != "" {
			paths[path] = true
		}
	}

	return paths, nil
}
