package issue_test

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
)

func TestDecodeAcceptsTheCurrentSchema(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte(frontmatter("schema: 1", "id: ISU-7f3akq")))
	require.NoError(t, err)

	i, err := issue.Decode(doc)
	require.NoError(t, err)
	require.Equal(t, issue.CurrentSchema, i.Schema)
}

func TestDecodeRefusesASchemaThisBuildDoesNotRead(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "a version from the future",
			lines: []string{"schema: 2", "id: ISU-7f3akq"},
			want: []string{
				"version 2", "version 1", "upgrade to a build of isu that reads version 2",
			},
		},
		{
			name:  "a version from the past",
			lines: []string{"schema: 0", "id: ISU-7f3akq"},
			want:  []string{"version 0", "version 1", "migrate it forward"},
		},
		{
			name:  "no version at all",
			lines: []string{"id: ISU-7f3akq"},
			want:  []string{"schema: required", "version 1"},
		},
		{
			name:  "a version that is not a number",
			lines: []string{"schema: banana", "id: ISU-7f3akq"},
			want:  []string{`"banana" is not a version number`, "version 1"},
		},
		{
			name:  "an empty version",
			lines: []string{"schema:", "id: ISU-7f3akq"},
			want:  []string{`"" is not a version number`},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doc, err := issue.Parse([]byte(frontmatter(tc.lines...)))
			require.NoError(t, err)

			i, err := issue.Decode(doc)
			require.Nil(t, i)

			var serr *issue.SchemaError
			require.ErrorAs(t, err, &serr)

			for _, want := range tc.want {
				require.Contains(t, err.Error(), want)
			}
		})
	}
}

// `schema: banana` is a parse error rather than a panic, and it says so in a
// way a caller can test for rather than by string matching.
func TestASchemaThatIsNotANumberIsAParseError(t *testing.T) {
	t.Parallel()

	doc, err := issue.Parse([]byte(frontmatter("schema: banana", "id: ISU-7f3akq")))
	require.NoError(t, err)

	_, err = issue.Decode(doc)
	require.ErrorIs(t, err, strconv.ErrSyntax)
}

// The registry ships empty: version 1 is the first version, so there is
// nothing yet to migrate from.
func TestTheDefaultRegistryIsEmpty(t *testing.T) {
	t.Parallel()

	require.Zero(t, issue.Migrations.Len())
	require.Empty(t, issue.Migrations.From())
}

// noop is a migration with nothing to do, which is what proves the registry
// rewrites the schema line and nothing else.
type noop struct {
	from    int
	applied *int
}

func (m noop) From() int { return m.from }

// Apply does nothing, which is the point: the file it produces differs from
// the one it read by the schema line and nothing else.
func (m noop) Apply(*issue.Document) error {
	if m.applied != nil {
		*m.applied++
	}

	return nil
}

func TestMigrateANoOpFromVersionZeroIsByteIdenticalApartFromTheSchemaLine(t *testing.T) {
	t.Parallel()

	before, err := os.ReadFile(filepath.Join("testdata", "schema0", "README.md"))
	require.NoError(t, err)

	doc, err := issue.Parse(before)
	require.NoError(t, err)

	applied := 0
	registry := issue.NewRegistry()
	require.NoError(t, registry.Register(noop{from: 0, applied: &applied}))
	require.Equal(t, 1, registry.Len())
	require.Equal(t, []int{0}, registry.From())

	changed, err := registry.Migrate(doc)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 1, applied)

	want := strings.Replace(string(before), "schema: 0", "schema: 1", 1)
	require.Equal(t, want, doc.String(),
		"a migration with nothing to do must move the schema line and not one byte more")

	// And the result is a file this build reads.
	i, err := issue.Decode(doc)
	require.NoError(t, err)
	require.Equal(t, issue.CurrentSchema, i.Schema)
	require.Equal(t, "PROJ-1234", mustGet(t, doc, "jira_key"))
}

func TestMigrateADocumentThatIsAlreadyCurrentChangesNothing(t *testing.T) {
	t.Parallel()

	text := frontmatter("schema: 1", "id: ISU-7f3akq")

	doc, err := issue.Parse([]byte(text))
	require.NoError(t, err)

	changed, err := issue.NewRegistry().Migrate(doc)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, text, doc.String())
}

func TestMigrateRejects(t *testing.T) {
	t.Parallel()

	t.Run("a version with no migration registered", func(t *testing.T) {
		t.Parallel()

		doc, err := issue.Parse([]byte(frontmatter("schema: 0", "id: ISU-7f3akq")))
		require.NoError(t, err)

		changed, err := issue.NewRegistry().Migrate(doc)
		require.False(t, changed)
		require.ErrorContains(t, err,
			"nothing knows how to migrate version 0 forward to version 1")
	})

	t.Run("a version from the future", func(t *testing.T) {
		t.Parallel()

		doc, err := issue.Parse([]byte(frontmatter("schema: 2", "id: ISU-7f3akq")))
		require.NoError(t, err)

		changed, err := issue.NewRegistry().Migrate(doc)
		require.False(t, changed)

		var serr *issue.SchemaError
		require.ErrorAs(t, err, &serr)
		require.Equal(t, 2, serr.Found)
	})

	t.Run("a document with no schema at all", func(t *testing.T) {
		t.Parallel()

		doc, err := issue.Parse([]byte(frontmatter("id: ISU-7f3akq")))
		require.NoError(t, err)

		changed, err := issue.NewRegistry().Migrate(doc)
		require.False(t, changed)
		require.ErrorContains(t, err, "schema: required")
	})

	t.Run("a migration that fails", func(t *testing.T) {
		t.Parallel()

		doc, err := issue.Parse([]byte(frontmatter("schema: 0", "id: ISU-7f3akq")))
		require.NoError(t, err)

		registry := issue.NewRegistry()
		require.NoError(t, registry.Register(failing{}))

		changed, err := registry.Migrate(doc)
		require.False(t, changed)
		require.ErrorIs(t, err, errBrokenMigration)
		require.ErrorContains(t, err, "migrating from version 0")
		require.Equal(t, frontmatter("schema: 0", "id: ISU-7f3akq"), doc.String(),
			"a failed migration must not leave the schema line moved")
	})
}

var errBrokenMigration = errors.New("this migration cannot run")

type failing struct{}

func (failing) From() int { return 0 }

func (failing) Apply(*issue.Document) error { return errBrokenMigration }

func TestRegisterRejects(t *testing.T) {
	t.Parallel()

	t.Run("a second migration from the same version", func(t *testing.T) {
		t.Parallel()

		registry := issue.NewRegistry()
		require.NoError(t, registry.Register(noop{from: 0}))
		require.ErrorContains(t, registry.Register(noop{from: 0}),
			"version 0 already has one")
	})

	t.Run("a migration from the version this build reads", func(t *testing.T) {
		t.Parallel()

		require.ErrorContains(t, issue.NewRegistry().Register(noop{from: issue.CurrentSchema}),
			"nothing to migrate from version 1")
	})

	t.Run("a migration from a version that is not one", func(t *testing.T) {
		t.Parallel()

		require.ErrorContains(t, issue.NewRegistry().Register(noop{from: -1}),
			"-1 is not a version")
	})
}

func TestMustRegister(t *testing.T) {
	t.Parallel()

	registry := issue.NewRegistry()
	require.NotPanics(t, func() { registry.MustRegister(noop{from: 0}) })
	require.Panics(t, func() { registry.MustRegister(noop{from: 0}) })
}

func mustGet(t *testing.T, doc *issue.Document, key string) string {
	t.Helper()

	value, ok := doc.Get(key)
	require.True(t, ok, "key %q is missing", key)

	return value
}
