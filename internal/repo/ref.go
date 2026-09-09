package repo

import (
	"context"
	"errors"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// unbornHEAD is the one revision that fails to resolve without anybody having
// made a mistake. HEAD always exists as a symbolic ref; it resolves to nothing
// only in a repository with no commits, which is an empty board rather than a
// failure. Every other name that does not resolve is a typo, and reporting it
// is the point.
const unbornHEAD = "HEAD"

// LoadRef reads every issue at a ref.
//
// This is the read path the plan mandates, and it is not an optimisation to
// do later. Measured on 5,000 issues: a `git show` per file takes 13.7 s,
// `cat-file --batch` fed `ref:path` takes 4.9 s because a path costs a tree
// walk per lookup, and `cat-file --batch` fed object ids takes 0.6 s. So:
//
//  1. `ls-tree -r` for the object id of every issues/*/README.md;
//  2. one `cat-file --batch`, fed those object ids;
//  3. parse the blobs out of the single output stream.
//
// Two git processes, whatever the repository holds. M2-S5 fails the build if
// that stops being true.
//
// Comments and attachments are not read. They are files beside the issue rather
// than part of it, nothing on the board or in a status derivation reads them,
// and holding every screenshot in a repository in memory to render a list would
// be a strange way to spend the 0.6 s above.
func (r *Repo) LoadRef(ctx context.Context, ref string) (*Set, error) {
	entries, err := r.git.LsTree(ctx, ref, IssuesDir)
	if err != nil {
		if ref == unbornHEAD && errors.Is(err, gitx.ErrUnknownRevision) {
			return newSet(), nil
		}

		return nil, err
	}

	set := newSet()

	// One object id may be several issues: two issue files that happen to be
	// byte-identical are one blob in git, and a batch keyed by object id would
	// otherwise lose all but one of them.
	paths := map[string][]string{}

	var oids []string

	for _, entry := range entries {
		id, ok := issueID(entry.Path)
		if !ok || entry.Type != "blob" {
			continue
		}
		if !issue.ValidID(id) {
			set.Broken = append(set.Broken, Broken{ID: id, Path: entry.Path, Err: errNotAnID(id)})
			continue
		}

		if _, seen := paths[entry.OID]; !seen {
			oids = append(oids, entry.OID)
		}
		paths[entry.OID] = append(paths[entry.OID], id)
	}

	err = r.git.CatFileBatch(ctx, oids, func(o gitx.Object) error {
		for _, id := range paths[o.OID] {
			set.add(id, readmePath(id), o.Data)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	set.sortBroken()

	return set, nil
}
