package importer

import (
	"fmt"
	"strings"

	"github.com/dgorshkov/isu/internal/issue"
)

// Mapping forms isu ids from a source's own keys and reads them back.
//
// **A key becomes the id verbatim where it is already a legal one, and is
// formed from it where it is not.** `PROJ-1234` stays `PROJ-1234` because ids
// are never rewritten and an imported issue keeps its source key; GitHub's
// `#1234` cannot be a folder name — `#` is not in the id character set — so it
// becomes `<PREFIX>-1234`, which keeps the number every link contains and the
// half a human recognises.
//
// The reverse is recorded rather than computed, and that is not laziness. With
// a prefix of PROJ, the key `#1234` and the key `PROJ-1234` form the same id,
// so no function of the id alone can say which one it came from. A Mapping
// remembers, which makes the round trip exact for the whole of an import and
// makes a collision between two keys something this type refuses rather than
// something the filesystem resolves by overwriting.
type Mapping struct {
	prefix string
	ids    map[string]string
	keys   map[string]string
	order  []string
}

// NewMapping returns an empty mapping forming ids under prefix.
func NewMapping(prefix string) *Mapping {
	return &Mapping{prefix: prefix, ids: map[string]string{}, keys: map[string]string{}}
}

// Prefix is the prefix ids are formed under.
func (m *Mapping) Prefix() string { return m.prefix }

// Len is how many keys have been mapped.
func (m *Mapping) Len() int { return len(m.ids) }

// Keys lists the source keys in the order they were added, which is the order
// the source read them.
func (m *Mapping) Keys() []string { return m.order }

// Add forms the id for one key and records it.
//
// Adding the same key twice is the same id and not an error: a source may name
// an issue once as a ticket and again as somebody's parent. Two different keys
// forming one id is refused, because the alternative is two issues sharing a
// folder and the second one winning.
func (m *Mapping) Add(key string) (string, error) {
	if known, ok := m.ids[key]; ok {
		return known, nil
	}

	id, err := m.form(key)
	if err != nil {
		return "", err
	}

	if taken, clash := m.keys[id]; clash {
		return "", fmt.Errorf(
			"%s and %s both become %s: two imports into one tracker collide, and isu "+
				"refuses rather than overwriting — import under a different --id-prefix",
			taken, key, id)
	}

	m.ids[key] = id
	m.keys[id] = key
	m.order = append(m.order, key)

	return id, nil
}

// ID is the id formed for a key, and whether that key is in the import at all.
//
// The second return is what keeps `blocked_by` honest: a blocker outside the
// import is not written as an id that resolves to nothing, which M5-S2 would
// fail on.
func (m *Mapping) ID(key string) (string, bool) {
	id, ok := m.ids[key]

	return id, ok
}

// Key is the source key an id was formed from.
func (m *Mapping) Key(id string) (string, bool) {
	key, ok := m.keys[id]

	return key, ok
}

// form is the rule: keep a key that is already an id, and build one from the
// part a human recognises when it is not.
//
// What follows the last `#` is that part — `#1234` and `owner/repo#1234` are
// both the issue numbered 1234 — and a key with no `#` is taken whole. A token
// that is still not a legal id after that is refused rather than mangled:
// dropping the illegal runes is how two keys quietly become one folder.
func (m *Mapping) form(key string) (string, error) {
	if issue.ValidID(key) {
		return key, nil
	}

	token := key
	if at := strings.LastIndex(key, "#"); at >= 0 {
		token = key[at+1:]
	}

	if !issue.ValidID(token) {
		return "", fmt.Errorf(
			"%q cannot become an issue id: an id is a folder name, so it is letters, "+
				"digits and the three marks - _ and a full stop", key)
	}

	return m.prefix + "-" + token, nil
}
