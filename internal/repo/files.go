package repo

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// File is one file that lives beside an issue's README.
//
// The README is not one of these. It is the issue, and every other loader in
// this package is about reading it; this is about everything else in the
// folder, which the issue file cannot describe and two rules in M5 are about.
type File struct {
	// ID is the issue whose folder it sits in.
	ID string
	// Name is where it sits inside that folder: `repro.har`, or
	// `comments/2026-08-24-support-01.md`.
	Name string
	// Path is relative to the repository root.
	Path string
	// Size is the blob's size in bytes.
	Size int64
	// Comment says it is one of the issue's comments rather than an attachment.
	// The two are counted apart because they are written by different things:
	// a comment is prose isu appends, and an attachment is whatever somebody
	// dropped beside the issue — which is the one of the two that can be a
	// forty-megabyte core dump.
	Comment bool
}

// Files is what lives beside each issue's README, keyed by issue id.
type Files map[string][]File

// Attachments are one issue's files that are not comments, in name order.
func (f Files) Attachments(id string) []File {
	var out []File

	for _, file := range f[id] {
		if !file.Comment {
			out = append(out, file)
		}
	}

	return out
}

// LoadFiles lists what lives beside every issue's README at a ref.
//
// One `ls-tree -r -l`, whatever the repository holds. The sizes are the whole
// reason for the call — the attachment cap in M5-S2 has no other source, and
// reading every attachment to measure it would be the slow read path the plan
// exists to forbid, over the largest files in the repository rather than the
// smallest.
func (r *Repo) LoadFiles(ctx context.Context, ref string) (Files, error) {
	entries, err := r.git.LsTreeLong(ctx, ref, IssuesDir)
	if err != nil {
		if ref == unbornHEAD && errors.Is(err, gitx.ErrUnknownRevision) {
			return Files{}, nil
		}

		return nil, err
	}

	files := Files{}

	for _, entry := range entries {
		// A gitlink or a tree has no content to weigh and is not something
		// anybody attached. `-r` flattens directories away, so what is left
		// here is a submodule.
		if entry.Type != "blob" {
			continue
		}

		id, name, ok := issueFile(entry.Path)
		if !ok || name == issue.ReadmeName {
			continue
		}

		files[id] = append(files[id], File{
			ID:      id,
			Name:    name,
			Path:    entry.Path,
			Size:    entry.Size,
			Comment: strings.HasPrefix(name, issue.CommentsDir+"/"),
		})
	}

	for id := range files {
		sort.SliceStable(files[id], func(a, b int) bool {
			return files[id][a].Name < files[id][b].Name
		})
	}

	return files, nil
}

// LoadWorktreeFiles lists what lives beside every issue's README on disk,
// uncommitted files included.
//
// It is LoadFiles' answer for the tree nobody has committed yet, and it exists
// for the pre-commit hook: a hook that read a ref would be answering about the
// commit before the one being made, which is not a slower answer but a wrong
// one — the file it complained about would be the file you were fixing.
//
// One git process, spent on .gitignore, for the same reason LoadWorktree spends
// it: an ignored file is not tracked, so it is not in this repository, and
// reimplementing the ignore rules to avoid asking would be a second opinion
// about what .gitignore means.
func (r *Repo) LoadWorktreeFiles(ctx context.Context) (Files, error) {
	ignored, err := r.ignored(ctx)
	if err != nil {
		return nil, err
	}

	files := Files{}

	entries, err := os.ReadDir(filepath.Join(r.root, IssuesDir))
	if errors.Is(err, fs.ErrNotExist) {
		return files, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", IssuesDir, err)
	}

	for _, entry := range entries {
		id := entry.Name()
		if !entry.IsDir() || !issue.ValidID(id) || ignored[IssuesDir+"/"+id+"/"] {
			continue
		}

		if found := r.folderFiles(id, ignored); len(found) > 0 {
			files[id] = found
		}
	}

	return files, nil
}

// folderFiles walks one issue's folder, in name order.
//
// Nothing here fails. A path that vanished between being listed and being
// looked at is a path this cannot weigh, and what is beside an issue is not a
// fact about the issue: a whole check run that refused to answer because one
// attachment moved would be worse than an answer that does not mention it. The
// same reasoning is why `isu show` walks a folder rather than going through
// issue.Load.
func (r *Repo) folderFiles(id string, ignored map[string]bool) []File {
	dir := filepath.Join(r.root, IssuesDir, id)
	prefix := dir + string(filepath.Separator)

	var found []File

	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		// A directory is walked into rather than weighed, and anything that is
		// not a regular file — a symlink, a socket — is not an attachment.
		// `ls-tree` calls a symlink a blob and this does not; what a size cap
		// is about is bytes in everybody's clone.
		if err != nil || entry.IsDir() || !entry.Type().IsRegular() {
			return nil
		}

		name := filepath.ToSlash(strings.TrimPrefix(path, prefix))
		if name == issue.ReadmeName || ignored[IssuesDir+"/"+id+"/"+name] {
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return nil //nolint:nilerr // a file that has gone is a file with no size
		}

		found = append(found, File{
			ID:      id,
			Name:    name,
			Path:    IssuesDir + "/" + id + "/" + name,
			Size:    info.Size(),
			Comment: strings.HasPrefix(name, issue.CommentsDir+"/"),
		})

		return nil
	})

	sort.SliceStable(found, func(a, b int) bool { return found[a].Name < found[b].Name })

	return found
}

// issueFile reads the issue and the folder-relative name out of a path under
// issues/, and reports whether the path is inside an issue folder at all.
func issueFile(path string) (id, name string, ok bool) {
	rest, ok := strings.CutPrefix(path, IssuesDir+"/")
	if !ok {
		return "", "", false
	}

	id, name, ok = strings.Cut(rest, "/")
	if !ok || !issue.ValidID(id) {
		// `issues/README.md` explaining the directory to a newcomer is not an
		// issue, and neither is a folder nobody could have named through isu.
		return "", "", false
	}

	return id, name, true
}
