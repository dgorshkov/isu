package repo

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
)

// DefaultRefPattern is what the board reads beside trunk: every local branch.
const DefaultRefPattern = "refs/heads/"

// BoardSpec says which refs to load.
type BoardSpec struct {
	// Trunk is the ref the board is anchored on. Empty means HEAD.
	Trunk string
	// Patterns are the ref patterns to load beside it. Empty means every local
	// branch.
	Patterns []string
}

// Board is every ref isu reads, loaded together.
//
// It is the shape M3-S1 derives statuses from: trunk, every branch, and — once
// M3-S3 decides what they mean — the claim refs beside them.
//
// Issues are shared between refs. An issue whose file a branch did not touch is
// the same *issue.Issue at that branch as it is at trunk, because it is the
// same blob in git. A Board is therefore a read model: deriving from it is
// safe, and writing through one issue would change it at every ref that shares
// the file.
type Board struct {
	// Trunk is the issue set at the trunk ref.
	Trunk *Set
	// Refs is the issue set at every other ref that matched, keyed by its full
	// ref name.
	Refs map[string]*Set
	// Changed is, for each of those refs, the ids whose file differs from
	// trunk's — added, modified or gone. It is sorted, and it is the only part
	// of a ref anything downstream has to look at.
	//
	// It exists because derivation is a statement about differences and the
	// map above is a statement about contents. Asking each ref for its whole
	// map is five thousand issues two hundred times over, and the answer is
	// almost always "the same issue trunk has"; the diff that built the ref
	// already knows which handful of files it was not. So the loader hands the
	// difference over rather than making M3 rediscover it — which is the same
	// bargain the read path itself makes, and the reason this field is here
	// and not a helper in internal/model.
	Changed map[string][]string
}

// Names lists the non-trunk refs in name order.
func (b *Board) Names() []string {
	names := make([]string, 0, len(b.Refs))
	for name := range b.Refs {
		names = append(names, name)
	}
	sort.Strings(names)

	return names
}

// LoadBoard reads trunk and every ref matching the spec.
//
// `isu board` never loads one ref, so this is the budget that matters and not
// the single-ref number in PLAN.md §0. It is a different shape from LoadRef for
// a reason worth stating, because the obvious shape does not work.
//
// Listing every ref's whole tree is correct and far too slow: two hundred
// branches over five thousand issues is a million tree entries to parse and a
// million map entries to build, and measured here it took eleven seconds
// against a budget of six. What the board actually wants to know about a branch
// is how it differs from trunk, which is a handful of files — and git answers
// that in time proportional to the difference, because it compares trees by
// object id and skips the subtrees that match.
//
// So: trunk is listed once, each other ref is diffed against it, and one
// `cat-file --batch` reads the union of every blob named by any of them. Each
// ref's set is trunk's with its own changes applied over the top.
//
// The process count is refs + 3: one for-each-ref, one ls-tree for trunk, one
// diff-tree per other ref, and one batch. It grows linearly in refs and not at
// all in issues, which is the assertion M2-S5 makes — the slow paths differ
// from the fast one by process count, not by algorithm.
func (r *Repo) LoadBoard(ctx context.Context, spec BoardSpec) (*Board, error) {
	trunk := spec.Trunk
	if trunk == "" {
		trunk = unbornHEAD
	}

	patterns := spec.Patterns
	if len(patterns) == 0 {
		patterns = []string{DefaultRefPattern}
	}

	refs, err := r.git.ForEachRef(ctx, patterns...)
	if err != nil {
		return nil, err
	}

	cache := newBlobCache()

	trunkIndex, err := r.indexTrunk(ctx, trunk, cache)
	if err != nil {
		return nil, err
	}

	diffs, err := r.diffRefs(ctx, trunk, refs, cache)
	if err != nil {
		return nil, err
	}

	if err := cache.fill(ctx, r.git); err != nil {
		return nil, err
	}

	board := &Board{
		Trunk:   cache.set(trunkIndex),
		Refs:    make(map[string]*Set, len(diffs)),
		Changed: make(map[string][]string, len(diffs)),
	}

	for name, changes := range diffs {
		board.Refs[name] = cache.apply(board.Trunk, changes)
		board.Changed[name] = changedIDs(changes)
	}

	return board, nil
}

// changedIDs names the issues a ref's difference from trunk is about.
func changedIDs(changes []gitx.Change) []string {
	ids := make([]string, 0, len(changes))

	for _, change := range changes {
		if id, ok := issueID(change.Path); ok {
			ids = append(ids, id)
		}
	}

	sort.Strings(ids)

	return ids
}

// diffRefs asks each ref how it differs from trunk, and registers the blobs
// those differences name.
func (r *Repo) diffRefs(
	ctx context.Context, trunk string, refs []gitx.Ref, cache *blobCache,
) (map[string][]gitx.Change, error) {
	diffs := make(map[string][]gitx.Change, len(refs))

	for _, ref := range refs {
		if sameRef(ref.Name, trunk) {
			continue
		}

		changes, err := r.git.DiffTree(ctx, trunk, ref.Name, IssuesDir)
		if err != nil {
			return nil, err
		}

		for _, c := range changes {
			if !c.Deleted() {
				cache.want(c.NewOID)
			}
		}

		diffs[ref.Name] = changes
	}

	return diffs, nil
}

// sameRef reports whether a full ref name is the trunk the caller named, which
// they may have spelled short.
func sameRef(name, trunk string) bool {
	return name == trunk || strings.TrimPrefix(name, DefaultRefPattern) == trunk
}

// refIndex is one ref's issue READMEs, before any blob has been read.
type refIndex struct {
	ids    []string
	oids   []string
	broken []Broken
}

// indexTrunk lists trunk's issue files and registers their blobs with the
// cache. An unborn HEAD is an empty ref rather than a failure, exactly as in
// LoadRef.
func (r *Repo) indexTrunk(ctx context.Context, ref string, cache *blobCache) (*refIndex, error) {
	entries, err := r.git.LsTree(ctx, ref, IssuesDir)
	if err != nil {
		if ref == unbornHEAD && errors.Is(err, gitx.ErrUnknownRevision) {
			return &refIndex{}, nil
		}

		return nil, err
	}

	idx := &refIndex{}

	for _, entry := range entries {
		id, ok := issueID(entry.Path)
		if !ok || entry.Type != "blob" {
			continue
		}

		if !issue.ValidID(id) {
			idx.broken = append(idx.broken, Broken{
				ID: id, Path: entry.Path, Err: errNotAnID(id),
			})

			continue
		}

		idx.ids = append(idx.ids, id)
		idx.oids = append(idx.oids, entry.OID)
		cache.want(entry.OID)
	}

	return idx, nil
}

// blobCache reads every blob the board needs once, and decodes every distinct
// file once.
type blobCache struct {
	// wanted is the union of object ids, in first-seen order so that the batch
	// is fed deterministically.
	wanted []string
	seen   map[string]bool
	data   map[string][]byte
	// decoded is keyed by blob *and* id: the id is what an issue's Folder is,
	// and two different issues whose files are byte-identical are one blob with
	// two folder names.
	decoded map[decodeKey]*issue.Issue
	broken  map[decodeKey]error
}

type decodeKey struct {
	oid string
	id  string
}

func newBlobCache() *blobCache {
	return &blobCache{
		seen:    map[string]bool{},
		data:    map[string][]byte{},
		decoded: map[decodeKey]*issue.Issue{},
		broken:  map[decodeKey]error{},
	}
}

func (c *blobCache) want(oid string) {
	if c.seen[oid] {
		return
	}

	c.seen[oid] = true
	c.wanted = append(c.wanted, oid)
}

// fill reads every wanted blob in one git process.
func (c *blobCache) fill(ctx context.Context, g *gitx.Git) error {
	return g.CatFileBatch(ctx, c.wanted, func(o gitx.Object) error {
		c.data[o.OID] = o.Data

		return nil
	})
}

// set assembles trunk's issues out of the cache.
func (c *blobCache) set(idx *refIndex) *Set {
	s := newSet()
	s.Broken = append(s.Broken, idx.broken...)

	for at, id := range idx.ids {
		c.put(s, id, idx.oids[at])
	}

	s.sortBroken()

	return s
}

// apply is trunk's set with one ref's changes over the top.
//
// Cloning the map is what keeps this affordable: it is one bulk copy per ref
// rather than five thousand inserts, and the issues inside are shared, so a
// branch that touched one file costs one decode and not five thousand.
func (c *blobCache) apply(base *Set, changes []gitx.Change) *Set {
	s := &Set{Issues: maps.Clone(base.Issues), Broken: slices.Clone(base.Broken)}

	for _, change := range changes {
		id, ok := issueID(change.Path)
		if !ok {
			continue
		}

		// Whatever trunk said about this issue, this ref says something else.
		delete(s.Issues, id)
		s.dropBroken(id)

		switch {
		case change.Deleted():
		case !issue.ValidID(id):
			s.Broken = append(s.Broken, Broken{
				ID: id, Path: change.Path, Err: errNotAnID(id),
			})
		default:
			c.put(s, id, change.NewOID)
		}
	}

	s.sortBroken()

	return s
}

// put decodes one blob into the set, reusing the decode when the same file has
// already been read for the same issue at another ref.
func (c *blobCache) put(s *Set, id, oid string) {
	key := decodeKey{oid: oid, id: id}

	if known, ok := c.decoded[key]; ok {
		s.Issues[id] = known

		return
	}
	if err, ok := c.broken[key]; ok {
		s.Broken = append(s.Broken, Broken{ID: id, Path: readmePath(id), Err: err})

		return
	}

	decoded, err := decode(id, c.data[oid])
	if err != nil {
		c.broken[key] = err
		s.Broken = append(s.Broken, Broken{ID: id, Path: readmePath(id), Err: err})

		return
	}

	c.decoded[key] = decoded
	s.Issues[id] = decoded
}
