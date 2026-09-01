package repo

import (
	"context"
	"errors"
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
// reading every attachment to measure it would be the slow read path PLAN.md §0
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
