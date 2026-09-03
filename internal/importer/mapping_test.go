package importer_test

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

func TestAKeyThatIsAlreadyAnIDIsKeptVerbatim(t *testing.T) {
	t.Parallel()

	m := importer.NewMapping("ISU")

	id, err := m.Add("PROJ-1234")
	require.NoError(t, err)
	require.Equal(t, "PROJ-1234", id,
		"an imported issue keeps its source key: the thing a team has been writing "+
			"in commit messages for years is still legible in the id")
}

func TestAKeyThatCannotBeAnIDIsFormedIntoOne(t *testing.T) {
	t.Parallel()

	m := importer.NewMapping("ISU")

	for _, tt := range []struct{ key, want string }{
		{"#1234", "ISU-1234"},
		{"acme/widgets#77", "ISU-77"},
		{"#M3", "ISU-M3"},
		{"acme/widgets#notanumber", "ISU-notanumber"},
	} {
		id, err := m.Add(tt.key)
		require.NoErrorf(t, err, "%s", tt.key)
		require.Equal(t, tt.want, id)
	}

	// Cross-repo issues are out of scope, so one import cannot collide with
	// itself; two imports into one tracker can, and isu refuses rather than
	// overwriting.
	_, err := m.Add("other/repo#1234")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--id-prefix")
}

func TestAnIDMapsBackToTheKeyItWasFormedFrom(t *testing.T) {
	t.Parallel()

	// Both halves of PLAN.md M7-S1's rule, and the second is why the reverse is
	// recorded rather than computed: with this prefix, `#1234` and `PROJ-1234`
	// form the same id, so no function of the id alone could say which.
	m := importer.NewMapping("PROJ")

	legal, err := m.Add("PROJ-1234")
	require.NoError(t, err)

	formed, err := m.Add("#77")
	require.NoError(t, err)

	back, ok := m.Key(legal)
	require.True(t, ok)
	require.Equal(t, "PROJ-1234", back)

	back, ok = m.Key(formed)
	require.True(t, ok)
	require.Equal(t, "#77", back, "the key is the half every link contains")

	_, ok = m.Key("PROJ-nobody")
	require.False(t, ok)
}

func TestAKeyAddedTwiceIsTheSameID(t *testing.T) {
	t.Parallel()

	// A source names an issue once as a ticket and again as somebody's parent.
	m := importer.NewMapping("ISU")

	first, err := m.Add("#7")
	require.NoError(t, err)

	again, err := m.Add("#7")
	require.NoError(t, err)

	require.Equal(t, first, again)
	require.Equal(t, 1, m.Len())
	require.Equal(t, []string{"#7"}, m.Keys())
	require.Equal(t, "ISU", m.Prefix())
}

func TestAKeyThatCannotBeMangledIntoAnIDIsRefused(t *testing.T) {
	t.Parallel()

	m := importer.NewMapping("ISU")

	// Dropping the illegal runes is how two keys quietly become one folder, so
	// this refuses instead.
	_, err := m.Add("acme/widgets")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot become an issue id")
}

func TestAnIDPrefixThatIsNotAFolderNameIsRefused(t *testing.T) {
	t.Parallel()

	_, err := importer.Keys(batch(), "not/a/prefix")
	require.Error(t, err)
	require.Contains(t, err.Error(), "cannot be an id prefix")
}

func TestACollisionInsideOneImportStopsIt(t *testing.T) {
	t.Parallel()

	b := batch()
	b.Items = append(b.Items, importer.Item{Key: "other/repo#7", Ref: "other/repo#7"})

	_, err := importer.Keys(b, "ISU")
	require.Error(t, err)
	require.Contains(t, err.Error(), "both become ISU-7")
}

// PLAN.md M7-S1: "dry run writes no files".
func TestMappingWritesNothing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	plan := mapped(t, batch(), importer.Options{})
	require.Len(t, plan.Folders, 2)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries,
		"a dry run is the default, and the default writes nothing at all")

	require.Empty(t, plan.Report.Wrote,
		"an empty Wrote is what makes a dry run one")
}

// PLAN.md M7-S1: "a source with two hundred custom fields produces clean
// frontmatter and a complete source.yml".
func TestTwoHundredCustomFieldsStayOutOfTheFrontmatter(t *testing.T) {
	t.Parallel()

	const fields = 200

	b := batch()
	b.Items = []importer.Item{custom(fields)}

	plan := mapped(t, b, importer.Options{})
	folder := find(t, plan, "ISU-1")

	readme := file(t, folder, issue.ReadmeName)
	front, _, ok := strings.Cut(strings.TrimPrefix(readme, "---\n"), "---")
	require.True(t, ok)

	for line := range strings.Lines(front) {
		if key, _, found := strings.Cut(line, ":"); found {
			require.Containsf(t, []string{
				issue.KeySchema, issue.KeyID, issue.KeyTitle, issue.KeyType,
				issue.KeyState, issue.KeyOwner, issue.KeyCreated,
			}, key, "a custom field reached the frontmatter as %q", key)
		}
	}

	source := file(t, folder, importer.SourceFileName)
	for i := range fields {
		require.Containsf(t, source, "value "+itoa(i), "field %d is missing", i)
	}

	require.Equal(t, fields, plan.Report.Extra)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	return string(digits)
}

func TestSourceYAMLIsOrderedSoThatAnImportIsIdempotent(t *testing.T) {
	t.Parallel()

	// PLAN.md M7-S5 asks that running an import twice produce a zero-length
	// diff, and a file whose key order follows a map's iteration produces a
	// diff every time.
	b := batch()
	b.Items = []importer.Item{custom(40)}

	first := file(t, find(t, mapped(t, b, importer.Options{}), "ISU-1"), importer.SourceFileName)

	for range 5 {
		again := file(t, find(t, mapped(t, batchOf(custom(40)), importer.Options{}), "ISU-1"),
			importer.SourceFileName)
		require.Equal(t, first, again)
	}
}

func batchOf(items ...importer.Item) *importer.Batch {
	b := batch()
	b.Items = items

	return b
}

func TestNestedValuesAreOrderedToo(t *testing.T) {
	t.Parallel()

	item := custom(0)
	item.Extra["tree"] = []any{
		map[string]any{"zulu": 1, "alpha": 2},
		map[string]any{"bravo": []any{map[string]any{"yankee": 3, "xray": 4}}},
	}

	source := file(t, find(t, mapped(t, batchOf(item), importer.Options{}), "ISU-1"),
		importer.SourceFileName)

	require.Positive(t, strings.Index(source, "alpha: 2"))
	require.Less(t, strings.Index(source, "alpha: 2"), strings.Index(source, "zulu: 1"))
	require.Less(t, strings.Index(source, "xray: 4"), strings.Index(source, "yankee: 3"))
}

func TestTheSourceInterfaceIsWhatAnImportReads(t *testing.T) {
	t.Parallel()

	// The seam PLAN.md M7-S1 says Jira and Linear come back through: four
	// methods, and everything after them is the same for every tracker.
	var source importer.Source = fake{batch: batch()}

	require.Equal(t, "fake", source.Name())
	require.NotEmpty(t, source.Describe())
	require.Equal(t, []string{"PROJ-1234"}, source.Keys("fixes PROJ-1234 (again)"))

	loaded, err := source.Load(context.Background())
	require.NoError(t, err)
	require.Len(t, loaded.Items, 2)

	broken := fake{err: errors.New("the tracker is down")}
	_, err = broken.Load(context.Background())
	require.Error(t, err)
}
