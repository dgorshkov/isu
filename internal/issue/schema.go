package issue

import (
	"fmt"
	"sort"
	"strconv"
)

// CurrentSchema is the on-disk format version this build of isu reads and
// writes. Adding an optional key does not move it; changing what an existing
// key means does.
//
// The field exists so that v1.0.0 is not a format prison. A reader that meets
// a version it does not know refuses it, naming both versions, rather than
// misparsing the file — because a reader that guesses at a format it has never
// seen writes the guess back.
const CurrentSchema = 1

// SchemaError is a schema field this build cannot act on: absent, not a
// number, or a version it does not read.
type SchemaError struct {
	// Found is the version the file declares. It is meaningless when Missing
	// is set or Raw is non-empty.
	Found int
	// Missing says the file declared no schema at all.
	Missing bool
	// Raw is what the field said when it was not a number, and Err is why it
	// is not one. An empty schema line is this case rather than version zero,
	// so the two are told apart by Err rather than by Raw being non-empty.
	Raw string
	Err error
}

func (e *SchemaError) Error() string {
	switch {
	case e.Missing:
		return fmt.Sprintf(
			"%s: required: this build of isu reads version %d, and a file that does not "+
				"say which version it is cannot be read safely", KeySchema, CurrentSchema)
	case e.Err != nil:
		return fmt.Sprintf("%s: %q is not a version number: this build of isu reads version %d",
			KeySchema, e.Raw, CurrentSchema)
	case e.Found > CurrentSchema:
		return fmt.Sprintf(
			"%s: this issue is version %d and this build of isu reads version %d: "+
				"upgrade to a build of isu that reads version %d",
			KeySchema, e.Found, CurrentSchema, e.Found)
	default:
		return fmt.Sprintf(
			"%s: this issue is version %d and this build of isu reads version %d: "+
				"migrate it forward before reading it",
			KeySchema, e.Found, CurrentSchema)
	}
}

func (e *SchemaError) Unwrap() error { return e.Err }

// Migration rewrites one document from the version it reads to the next one.
//
// A migration moves exactly one version. Chaining several is the registry's
// job, and the registry is also what rewrites the schema line afterwards — so
// a migration that has nothing to do is a method body with nothing in it, and
// the file it produces differs from the one it read by that one line and
// nothing else.
type Migration interface {
	// From is the version this migration reads. It produces From()+1.
	From() int
	// Apply rewrites doc in place. It must change only what the version bump
	// requires: every other key, the order they are written in and the file's
	// spacing are somebody's file, not the migration's.
	Apply(doc *Document) error
}

// Registry dispatches migrations by the version they read.
type Registry struct {
	migrations map[int]Migration
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{migrations: map[int]Migration{}}
}

// Migrations is the registry isu itself uses. It has nothing in it: version 1
// is the first version, so there is nothing yet to migrate from. Migrations
// register themselves here, in the file that defines them.
//
// Reading an issue never migrates it. A reader that rewrote files as a side
// effect of being pointed at them would make `isu board` a command that
// changes the working tree.
var Migrations = NewRegistry()

// Len is how many migrations are registered.
func (r *Registry) Len() int { return len(r.migrations) }

// From lists the versions the registry can migrate from, in order.
func (r *Registry) From() []int {
	out := make([]int, 0, len(r.migrations))
	for version := range r.migrations {
		out = append(out, version)
	}
	sort.Ints(out)

	return out
}

// Register adds a migration. Two migrations reading the same version is a
// programming error rather than a configuration one, and so is a migration
// reading a version that is already current: there is nowhere forward from it.
func (r *Registry) Register(m Migration) error {
	from := m.From()

	switch {
	case from < 0:
		return fmt.Errorf("registering a migration: version %d is not a version", from)
	case from >= CurrentSchema:
		return fmt.Errorf(
			"registering a migration: nothing to migrate from version %d, "+
				"which is already what this build reads", from)
	}

	if _, taken := r.migrations[from]; taken {
		return fmt.Errorf("registering a migration: version %d already has one", from)
	}

	r.migrations[from] = m

	return nil
}

// MustRegister is Register for a migration written into this binary, where a
// duplicate is a bug that should never reach a user's repository.
func (r *Registry) MustRegister(m Migration) {
	if err := r.Register(m); err != nil {
		panic(err)
	}
}

// Migrate brings doc forward to CurrentSchema, one version at a time, and
// reports whether anything changed.
//
// The schema line is written here rather than by each migration, so that a
// migration cannot forget to bump it and cannot bump it twice.
func (r *Registry) Migrate(doc *Document) (bool, error) {
	version, err := schemaVersion(doc)
	if err != nil {
		return false, err
	}
	if version > CurrentSchema {
		return false, &SchemaError{Found: version}
	}

	changed := false

	for version < CurrentSchema {
		m, ok := r.migrations[version]
		if !ok {
			return false, fmt.Errorf(
				"%s: nothing knows how to migrate version %d forward to version %d",
				KeySchema, version, version+1)
		}

		if err := m.Apply(doc); err != nil {
			return false, fmt.Errorf("migrating from version %d: %w", version, err)
		}

		version++
		doc.Set(KeySchema, strconv.Itoa(version))
		changed = true
	}

	return changed, nil
}

// decodeSchema reads the schema field and refuses anything this build does not
// read, in either direction: a newer file needs a newer isu, and an older one
// needs migrating first.
func decodeSchema(doc *Document) (int, error) {
	version, err := schemaVersion(doc)
	if err != nil {
		return 0, err
	}
	if version != CurrentSchema {
		return 0, &SchemaError{Found: version}
	}

	return version, nil
}

// schemaVersion reads the field without judging the version it finds.
func schemaVersion(doc *Document) (int, error) {
	raw, ok := doc.Get(KeySchema)
	if !ok {
		return 0, &SchemaError{Missing: true}
	}

	version, err := strconv.Atoi(raw)
	if err != nil {
		return 0, &SchemaError{Raw: raw, Err: err}
	}

	return version, nil
}
