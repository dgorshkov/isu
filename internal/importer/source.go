package importer

import (
	"fmt"
	"sort"

	"github.com/goccy/go-yaml"
)

// SourceFileName is where everything the schema had no place for goes.
//
// It sits in the issue's own folder, beside the README, and **nothing in isu
// reads it**. That is the point: a mature tracker has two hundred custom
// fields, and a schema that absorbs them is not a schema. The dependency
// allowlist in `docs/design.md` names goccy/go-yaml for `.isu.yml` and this file and nothing else —
// frontmatter is still parsed by hand, because Write has to round-trip unknown
// keys, spacing and line endings byte for byte and a YAML serialiser will not.
const SourceFileName = "source.yml"

const sourceHeader = `# Written by ` + "`isu import`" + `. Everything the source said that isu's schema
# has no place for: the key it was imported under, the fields nobody mapped,
# and the links that pointed outside the import.
#
# Nothing in isu reads this file. It is here so that modelling any of it later
# is a story rather than a re-import.
`

// renderSource writes source.yml.
//
// Keys are sorted at every level rather than left to a map's iteration order,
// and that is what makes an import idempotent: M7-S5 asks that running
// it twice produces a zero-length diff, and a file whose key order changes per
// run produces a diff every time.
func renderSource(values map[string]any) ([]byte, error) {
	out, err := yaml.Marshal(ordered(values))
	if err != nil {
		return nil, fmt.Errorf("rendering %s: %w", SourceFileName, err)
	}

	return append([]byte(sourceHeader), out...), nil
}

// ordered turns every mapping into one whose key order is decided here.
func ordered(value any) any {
	switch v := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}

		sort.Strings(keys)

		out := make(yaml.MapSlice, 0, len(keys))
		for _, key := range keys {
			out = append(out, yaml.MapItem{Key: key, Value: ordered(v[key])})
		}

		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = ordered(item)
		}

		return out
	default:
		return v
	}
}
