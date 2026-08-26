package issue

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The frontmatter keys. They are named here rather than spelled at every call
// site so that the decoder, the encoder and the validator cannot drift apart.
const (
	KeySchema     = "schema"
	KeyID         = "id"
	KeyTitle      = "title"
	KeyType       = "type"
	KeyState      = "state"
	KeyOwner      = "owner"
	KeyCreated    = "created"
	KeyPriority   = "priority"
	KeyParent     = "parent"
	KeyBlockedBy  = "blocked_by"
	KeyRepro      = "repro"
	KeyAcceptance = "acceptance"
	KeyQuestion   = "question"
	KeyReason     = "reason"
	KeyResolution = "resolution"
)

// keyOrder is the order a freshly created issue writes its frontmatter in: the
// order the schema is documented in, so that a file isu wrote and a file a
// human wrote read the same way.
var keyOrder = []string{
	KeySchema, KeyID, KeyTitle, KeyType, KeyState, KeyOwner, KeyCreated,
	KeyPriority, KeyParent, KeyBlockedBy, KeyRepro, KeyAcceptance, KeyQuestion,
	KeyReason, KeyResolution,
}

// Type is what an issue is, which is also what closing it requires.
type Type string

// The issue types. An epic is the odd one out: it declares no state, and its
// status is the fold over its children.
const (
	TypeBug   Type = "bug"
	TypeStory Type = "story"
	TypeChore Type = "chore"
	TypeSpike Type = "spike"
	TypeEpic  Type = "epic"
)

// Types lists every type, in the order the schema documents them.
var Types = []Type{TypeBug, TypeStory, TypeChore, TypeSpike, TypeEpic}

// Valid reports whether t is one of the five types.
func (t Type) Valid() bool {
	for _, known := range Types {
		if t == known {
			return true
		}
	}

	return false
}

// State is the stored half of an issue's status. The derived statuses the
// board renders are computed from this read across refs, and are never stored.
type State string

// The issue states.
const (
	StateOpen     State = "open"
	StateResolved State = "resolved"
	StateDropped  State = "dropped"
)

// States lists every state.
var States = []State{StateOpen, StateResolved, StateDropped}

// Valid reports whether s is one of the three states.
func (s State) Valid() bool {
	for _, known := range States {
		if s == known {
			return true
		}
	}

	return false
}

// Terminal reports whether the state is one an issue does not come back from
// on its own. A reopened issue is one that was terminal at an earlier trunk
// commit and is open now.
func (s State) Terminal() bool { return s == StateResolved || s == StateDropped }

// Priority is what `isu ready` orders by.
type Priority string

// The priorities.
const (
	PriorityP0 Priority = "p0"
	PriorityP1 Priority = "p1"
	PriorityP2 Priority = "p2"
	PriorityP3 Priority = "p3"
)

// Priorities lists every priority, most urgent first.
var Priorities = []Priority{PriorityP0, PriorityP1, PriorityP2, PriorityP3}

// DefaultPriority is what an issue that does not declare one is treated as.
const DefaultPriority = PriorityP2

// Valid reports whether p is one of the four priorities.
func (p Priority) Valid() bool {
	for _, known := range Priorities {
		if p == known {
			return true
		}
	}

	return false
}

// Resolution says why a dropped issue was dropped. `dropped` on its own cannot
// tell a duplicate from a won't-fix, and the difference is the first thing
// anyone asks.
type Resolution string

// The resolutions.
const (
	ResolutionWontfix         Resolution = "wontfix"
	ResolutionDuplicate       Resolution = "duplicate"
	ResolutionWorksAsIntended Resolution = "works-as-intended"
	ResolutionFixedElsewhere  Resolution = "fixed-elsewhere"
)

// Resolutions lists every resolution.
var Resolutions = []Resolution{
	ResolutionWontfix, ResolutionDuplicate,
	ResolutionWorksAsIntended, ResolutionFixedElsewhere,
}

// Valid reports whether r is one of the four resolutions.
func (r Resolution) Valid() bool {
	for _, known := range Resolutions {
		if r == known {
			return true
		}
	}

	return false
}

// Issue is one issue file: its frontmatter, decoded, and its markdown body.
//
// The document it was decoded from is kept alongside, so that writing an issue
// back rewrites only the lines whose values actually changed and leaves
// unknown keys, spacing and line terminators exactly as they were read.
type Issue struct {
	// Schema is the on-disk format version. A reader refuses one it does not
	// know rather than misparsing it — see schema.go.
	Schema int
	// ID is permanent from creation and must equal the folder name.
	ID string
	// Title is one line, and is what the board, the list and every filter
	// render.
	Title string
	// Type determines which fields below are required, and what resolving the
	// issue requires.
	Type Type
	// State is empty on an epic and required on everything else.
	State State
	// Owner is the human answerable for the issue. It is set at triage, and an
	// agent must never change it. Who is working on it right now is a
	// different question, answered by the claim ref.
	Owner string
	// Created is what issue age is computed from. Deriving it by walking
	// history would cost a git process per issue.
	Created time.Time
	// Priority is optional; an issue that declares none is DefaultPriority.
	// The zero value is kept as the zero value rather than defaulted here, so
	// that writing the issue back does not add a line the author did not
	// write.
	Priority Priority
	// Parent names the epic this issue belongs to. Validate checks its shape
	// and not what it points at: validating one issue never loads another.
	Parent string
	// BlockedBy names the issues this one waits on.
	BlockedBy []string
	// Repro is required on a bug, Acceptance on a story, Question on a spike.
	Repro      string
	Acceptance string
	Question   string
	// Reason and Resolution are required when the state is dropped.
	Reason     string
	Resolution Resolution
	// Body is the markdown below the frontmatter, verbatim.
	Body string
	// Folder is the name of the folder the issue was read from, which its id
	// must equal. It is empty for an issue that is not on disk yet, and the
	// check is skipped then rather than failed.
	Folder string

	// createdRaw is what the file said, kept so that a date the file wrote in
	// a form other than the one we would write survives a round trip, and so
	// that Validate can tell an absent date from an unparseable one.
	createdRaw string
	// doc is the document this issue was decoded from, or nil for an issue
	// built in memory.
	doc *Document
}

// EffectivePriority is Priority with the default applied.
func (i *Issue) EffectivePriority() Priority {
	if i.Priority == "" {
		return DefaultPriority
	}

	return i.Priority
}

// Decode reads an issue out of a parsed document. It refuses a schema version
// it does not understand — that check is the subject of schema.go — and
// otherwise reports no errors: a missing or malformed field is Validate's to
// report, all of them at once, rather than the decoder's to stop on.
func Decode(doc *Document) (*Issue, error) {
	schema, err := decodeSchema(doc)
	if err != nil {
		return nil, err
	}

	i := &Issue{Schema: schema, Body: doc.Body, doc: doc}

	i.ID, _ = doc.Get(KeyID)
	i.Title, _ = doc.Get(KeyTitle)
	i.Owner, _ = doc.Get(KeyOwner)
	i.Parent, _ = doc.Get(KeyParent)
	i.Repro, _ = doc.Get(KeyRepro)
	i.Acceptance, _ = doc.Get(KeyAcceptance)
	i.Question, _ = doc.Get(KeyQuestion)
	i.Reason, _ = doc.Get(KeyReason)

	if v, ok := doc.Get(KeyType); ok {
		i.Type = Type(v)
	}
	if v, ok := doc.Get(KeyState); ok {
		i.State = State(v)
	}
	if v, ok := doc.Get(KeyPriority); ok {
		i.Priority = Priority(v)
	}
	if v, ok := doc.Get(KeyResolution); ok {
		i.Resolution = Resolution(v)
	}

	i.createdRaw, _ = doc.Get(KeyCreated)
	if when, ok := parseDate(i.createdRaw); ok {
		i.Created = when
	}

	if v, ok := doc.Get(KeyBlockedBy); ok {
		i.BlockedBy = splitList(v)
	}

	return i, nil
}

// Encode renders the issue back to a file.
//
// A key whose value has not changed is not rewritten, which is what keeps an
// unmodified issue a zero-length diff: unknown keys, the order the author
// wrote their frontmatter in, the spacing around a colon and the file's line
// terminators all survive untouched.
func (i *Issue) Encode() []byte {
	doc := i.doc
	if doc == nil {
		doc = NewDocument()
		i.doc = doc
	}

	values := map[string]string{
		KeySchema:     strconv.Itoa(i.Schema),
		KeyID:         i.ID,
		KeyTitle:      i.Title,
		KeyType:       string(i.Type),
		KeyState:      string(i.State),
		KeyOwner:      i.Owner,
		KeyCreated:    i.created(),
		KeyPriority:   string(i.Priority),
		KeyParent:     i.Parent,
		KeyBlockedBy:  strings.Join(i.BlockedBy, ", "),
		KeyRepro:      i.Repro,
		KeyAcceptance: i.Acceptance,
		KeyQuestion:   i.Question,
		KeyReason:     i.Reason,
		KeyResolution: string(i.Resolution),
	}

	for _, key := range keyOrder {
		sync(doc, key, values[key])
	}

	doc.Body = i.Body

	return doc.Bytes()
}

// created is the date as it should be written: what the file said, when that
// still names the same day, and the canonical form otherwise.
func (i *Issue) created() string {
	if i.Created.IsZero() {
		return i.createdRaw
	}
	if when, ok := parseDate(i.createdRaw); ok && when.Equal(i.Created) {
		return i.createdRaw
	}

	return i.Created.Format(time.DateOnly)
}

// sync writes want into the document only when it differs from what is already
// there. An empty want means the key does not belong in the file at all.
func sync(doc *Document, key, want string) {
	got, present := doc.Get(key)

	switch {
	case want == "" && present:
		doc.Unset(key)
	case want == "":
	case !present, got != want:
		doc.Set(key, want)
	}
}

// Problem is one thing wrong with an issue.
type Problem struct {
	// Field is the frontmatter key at fault, or the empty string for a problem
	// that is not about one key.
	Field string
	// Message says what is wrong with it, in a sentence a human can act on.
	Message string
}

// ValidationError is every problem with one issue, sorted, so that a file with
// four things wrong reports four things rather than the first one four times.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 1 {
		return fmt.Sprintf("1 problem: %s", e.Problems[0])
	}

	var b strings.Builder

	fmt.Fprintf(&b, "%d problems:", len(e.Problems))
	for _, p := range e.Problems {
		fmt.Fprintf(&b, "\n  %s", p)
	}

	return b.String()
}

func (p Problem) String() string {
	if p.Field == "" {
		return p.Message
	}

	return p.Field + ": " + p.Message
}

// Validate reports everything wrong with one issue.
//
// It takes one issue and nothing else: no child index, no sibling lookup, no
// repository. That is why epic-ness is declared in the file rather than
// inferred from who points at it, and why `parent:` is checked for shape and
// not for what it names — nobody else's pull request can invalidate a file you
// own by adding a line to a third file.
func (i *Issue) Validate() error {
	var p problems

	if i.Schema != CurrentSchema {
		p.add(KeySchema, fmt.Sprintf("must be %d, found %d", CurrentSchema, i.Schema))
	}

	i.validateID(&p)
	i.validateTitle(&p)
	i.validateType(&p)
	i.validateState(&p)

	if i.Owner == "" {
		p.add(KeyOwner, "required: name the human answerable for this issue")
	}

	i.validateCreated(&p)
	i.validateLinks(&p)
	i.validateTypeFields(&p)
	i.validateDropped(&p)

	return p.err()
}

func (i *Issue) validateID(p *problems) {
	switch {
	case i.ID == "":
		p.add(KeyID, "required")
	case !ValidID(i.ID):
		p.add(KeyID, fmt.Sprintf("%q is not an id: ids are letters, digits, -, _ and .", i.ID))
	case i.Folder != "" && i.ID != i.Folder:
		p.add(KeyID, fmt.Sprintf("must equal the folder name, which is %q", i.Folder))
	}
}

func (i *Issue) validateTitle(p *problems) {
	switch {
	case i.Title == "":
		p.add(KeyTitle, "required")
	case strings.ContainsAny(i.Title, "\r\n"):
		p.add(KeyTitle, "must be one line")
	}
}

func (i *Issue) validateType(p *problems) {
	switch {
	case i.Type == "":
		p.add(KeyType, "required: "+oneOf(Types))
	case !i.Type.Valid():
		p.add(KeyType, fmt.Sprintf("%q is not a type: %s", i.Type, oneOf(Types)))
	}
}

func (i *Issue) validateState(p *problems) {
	if i.Type == TypeEpic {
		if i.State != "" {
			p.add(KeyState, "an epic must not declare a state: "+
				"its status is the fold over its children")
		}

		return
	}

	switch {
	case i.State == "":
		p.add(KeyState, "required: "+oneOf(States))
	case !i.State.Valid():
		p.add(KeyState, fmt.Sprintf("%q is not a state: %s", i.State, oneOf(States)))
	}
}

func (i *Issue) validateCreated(p *problems) {
	switch {
	case i.createdRaw == "" && i.Created.IsZero():
		p.add(KeyCreated, "required: an RFC 3339 date, such as 2026-08-24")
	case i.Created.IsZero():
		p.add(KeyCreated, fmt.Sprintf(
			"%q is not an RFC 3339 date, such as 2026-08-24", i.createdRaw))
	}
}

func (i *Issue) validateLinks(p *problems) {
	if i.Priority != "" && !i.Priority.Valid() {
		p.add(KeyPriority, fmt.Sprintf("%q is not a priority: %s", i.Priority, oneOf(Priorities)))
	}
	if i.Parent != "" && !ValidID(i.Parent) {
		p.add(KeyParent, fmt.Sprintf("%q is not an id", i.Parent))
	}

	for _, blocker := range i.BlockedBy {
		if !ValidID(blocker) {
			p.add(KeyBlockedBy, fmt.Sprintf("%q is not an id", blocker))
		}
	}
}

func (i *Issue) validateTypeFields(p *problems) {
	switch i.Type {
	case TypeBug:
		if i.Repro == "" {
			p.add(KeyRepro, "required on a bug: say how to reproduce it")
		}
	case TypeStory:
		if i.Acceptance == "" {
			p.add(KeyAcceptance, "required on a story: say what done looks like")
		}
	case TypeSpike:
		if i.Question == "" {
			p.add(KeyQuestion, "required on a spike: say what is being answered")
		}
	case TypeChore, TypeEpic:
	}
}

func (i *Issue) validateDropped(p *problems) {
	if i.State != StateDropped {
		return
	}

	if i.Reason == "" {
		p.add(KeyReason, "required when state is dropped")
	}

	switch {
	case i.Resolution == "":
		p.add(KeyResolution, "required when state is dropped: "+oneOf(Resolutions))
	case !i.Resolution.Valid():
		p.add(KeyResolution, fmt.Sprintf(
			"%q is not a resolution: %s", i.Resolution, oneOf(Resolutions)))
	}
}

// ValidID reports whether s can be an issue id.
//
// The generated format is <PREFIX>-<token>, but imported issues keep their
// source key verbatim — PROJ-1234 is a valid id — so this governs what may be
// read rather than what is generated. An id is also a folder name, so it may
// not be empty, may not be `.` or `..`, and may not carry a path separator.
func ValidID(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}

	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.':
		default:
			return false
		}
	}

	return true
}

// problems accumulates what is wrong with an issue.
type problems []Problem

func (p *problems) add(field, message string) {
	*p = append(*p, Problem{Field: field, Message: message})
}

// err sorts the problems and returns them as one error, or nil when there are
// none. Sorting is what makes the output stable: a validator whose message
// order depends on the order the checks happen to run in cannot be asserted on
// and cannot be diffed between runs.
func (p problems) err() error {
	if len(p) == 0 {
		return nil
	}

	sorted := make([]Problem, len(p))
	copy(sorted, p)
	sort.SliceStable(sorted, func(a, b int) bool {
		if sorted[a].Field != sorted[b].Field {
			return sorted[a].Field < sorted[b].Field
		}

		return sorted[a].Message < sorted[b].Message
	})

	return &ValidationError{Problems: sorted}
}

// oneOf renders an enum for an error message.
func oneOf[T ~string](values []T) string {
	names := make([]string, len(values))
	for i, v := range values {
		names[i] = string(v)
	}

	return "expected one of " + strings.Join(names, ", ")
}

// parseDate reads the two forms of RFC 3339 an issue file may carry: the date
// on its own, which is what isu writes, and a full timestamp, which is what an
// importer carrying a source system's field may produce.
func parseDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}

	for _, layout := range []string{time.DateOnly, time.RFC3339} {
		if when, err := time.Parse(layout, value); err == nil {
			return when, true
		}
	}

	return time.Time{}, false
}

// splitList reads a comma-separated frontmatter value.
func splitList(value string) []string {
	var out []string

	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}

	return out
}
