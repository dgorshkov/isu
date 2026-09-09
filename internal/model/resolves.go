package model

import (
	"strings"

	"github.com/dgorshkov/isu/internal/issue"
)

// ResolvesTrailer is the commit trailer `isu resolve` writes to link a trunk
// commit back to the issue it resolved.
const ResolvesTrailer = "Isu-Resolves"

// Tier says which source recovered that link. Recording it matters because the
// tiers are not equally trustworthy: one is what isu wrote, and the other is
// what somebody's commit subject happened to contain.
type Tier int

// The tiers, in the order they are read.
const (
	// TierNone is no link at all, which is the right answer far more often
	// than a guess is.
	TierNone Tier = iota
	// TierTrailer is the Isu-Resolves trailer. It is what isu wrote, so it is
	// read first and nothing overrides it.
	TierTrailer
	// TierSubject is an id in the commit subject. Under a squash merge the
	// subject is whatever the forge composed — often a pull request title —
	// so this is a fallback and not a convention anybody should rely on.
	TierSubject
)

// Resolves recovers the issues a trunk commit says it resolved.
//
// Post-merge questions are answered from file content, never from commit
// metadata — squash collapses authorship and does not touch the file. Linking a
// trunk commit back to the issue it resolved is the one thing file content
// cannot answer, so it is answered from the message, and from nowhere else this
// package can reach.
//
// Two of the plan's three tiers are here, because they are the two that are in a
// commit message. The middle tier — the branch name the merge recorded — is not
// in one: it needs the merge's own refs, so it belongs to whatever loads them,
// and M7-S3 already scans for exactly that when it walks the years of history
// that predate isu.
//
// The subject tier is anchored on the repository's own prefix, and that anchor
// is the whole of its safety. A pull request title is a sentence, and every
// word in a sentence is a legal id — ids are letters, digits, a hyphen, an
// underscore and a full stop, because an imported issue keeps its source key
// verbatim. Without the anchor this would return the first word of every squash
// commit in the repository, confidently.
//
// An empty prefix therefore reads no subjects. Nothing downstream can tell a
// wrong link from a right one, so the answer to "which issue is this about" has
// to be nothing rather than a guess.
func Resolves(prefix, subject, body string) ([]string, Tier) {
	if ids := trailerIDs(body); len(ids) > 0 {
		return ids, TierTrailer
	}

	// A revert quotes the subject it undid, verbatim and in full, so the
	// anchor this tier relies on is present and points the wrong way round: the
	// commit that un-resolved an issue would be read as one that resolved it.
	// M7-S3 scans years of history for exactly these, and a revert is the one
	// commit in that history whose subject means the opposite of what it says.
	//
	// The trailer tier is not guarded, and needs no guard: git writes `This
	// reverts commit <oid>.` as the body and does not carry the original
	// trailers over, so an Isu-Resolves on a revert was put there by a person
	// who meant it.
	if prefix != "" && !reverts(subject) {
		if ids := subjectIDs(prefix, subject); len(ids) > 0 {
			return ids, TierSubject
		}
	}

	return nil, TierNone
}

// reverts reports whether a subject is one git composed to undo another.
//
// `git revert` writes `Revert "<subject>"`, and since 2.36 `git cherry-pick`
// writes `Reapply "<subject>"` for a revert of a revert. Both quote a subject
// that may itself name an issue, and neither is a commit that resolved one.
func reverts(subject string) bool {
	subject = strings.TrimSpace(subject)

	return strings.HasPrefix(subject, `Revert "`) || strings.HasPrefix(subject, `Reapply "`)
}

// trailerIDs reads every Isu-Resolves line out of a commit body.
//
// The token is matched without regard to case, as git matches a trailer's, so
// that a human typing the line by hand does not have to match isu's capitals.
// One line may name several issues, because one squash commit may land several
// claims.
//
// A comma separates them, and so does whitespace, but the two are not read the
// same way — because what a value is allowed to contain depends on how many
// things are in it.
//
// A comma-separated element is taken as written, and ValidID is the only rule
// applied to it. That has to stay true: ValidID is permissive on purpose, since
// an imported issue keeps its source key verbatim, so `4821` from a GitHub
// import and `ISU_7f3akq` are both ids and neither looks like one. Demanding
// they look like one is how the first attempt at this broke them, and breaking
// them is not a missing link but a wrong one — see below.
//
// An element holding several words is a list somebody separated by something
// other than a comma, and there the words must each look like a key: something,
// a hyphen, something. Prose is the reason. `the login one` is three perfectly
// valid ids by ValidID's rule, and reading a comma-separated value as a single
// token was the only thing filtering it out — accidentally, because of the
// spaces. So a multi-word element is a list of keys or it is not read at all;
// isu's own `isu resolve` writes commas, so this path is for what a human
// typed, and a human typing a list types keys.
//
// The cost of reading a value wrong is what makes all of this worth a rule
// rather than a guess: a trailer that yields nothing falls through to the
// subject tier, and the subject is whatever the forge composed. So a commit
// whose trailer named two issues would resolve a third one instead, and nothing
// downstream could tell that link from a right one.
func trailerIDs(body string) []string {
	var ids []string

	seen := map[string]bool{}

	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			ids = append(ids, name)
		}
	}

	// Whether the line being read continues the trailer above it. Git's trailer
	// grammar folds a value across lines when the later ones are indented, and
	// a list of several claims is exactly the value long enough for somebody's
	// editor to wrap it — so reading only the first line would drop every claim
	// after it, at the tier this package treats as authoritative.
	folded := false

	for line := range strings.Lines(body) {
		value, ok := trailerValue(line, folded)
		if !ok {
			folded = false

			continue
		}

		folded = true

		for _, element := range strings.Split(value, ",") {
			words := strings.FieldsFunc(element, func(r rune) bool { return !idRune(r) })

			switch {
			case len(words) == 1 && issue.ValidID(words[0]):
				add(words[0])
			case len(words) > 1 && allKeyShaped(words):
				for _, name := range words {
					if issue.ValidID(name) {
						add(name)
					}
				}
			}
		}
	}

	return ids
}

// trailerValue returns the part of a line that is an Isu-Resolves value.
//
// A line opens the trailer when its token matches, without regard to case, as
// git matches a trailer's — so a human typing the line by hand does not have to
// match isu's capitals. A line continues one already open when it begins with
// whitespace, which is git's own folding rule; anything else closes it.
func trailerValue(line string, folded bool) (string, bool) {
	if folded && strings.TrimSpace(line) != "" && (line[0] == ' ' || line[0] == '\t') {
		return line, true
	}

	token, value, ok := strings.Cut(line, ":")
	if !ok || !strings.EqualFold(strings.TrimSpace(token), ResolvesTrailer) {
		return "", false
	}

	return value, true
}

// allKeyShaped reports whether every word looks like an issue key rather than
// like a word: something, a hyphen, something.
//
// It gates only the multi-word case, and deliberately: a lone word is whatever
// the repository named its folder, and this rule would throw away the imported
// keys that ValidID exists to admit.
func allKeyShaped(words []string) bool {
	for _, s := range words {
		if at := strings.Index(s, "-"); at <= 0 || at >= len(s)-1 {
			return false
		}
	}

	return true
}

// subjectIDs reads the ids out of a commit subject, in the order they appear.
func subjectIDs(prefix, subject string) []string {
	var ids []string

	seen := map[string]bool{}

	for _, word := range strings.FieldsFunc(subject, func(r rune) bool { return !idRune(r) }) {
		// A full stop is a legal id character and an id that ends in one is
		// not something isu generates — but a sentence that ends in one is
		// ordinary English, so the two are told apart here rather than by
		// hoping nobody writes a subject with punctuation.
		//
		// A run of two or more is punctuation whatever follows it: `..` is not
		// a legal id on its own and no tracker keys on one, so an ellipsis in
		// `fixes ISU-7f3akq...and more` ends the id rather than continuing it.
		// A single interior stop is left alone, because `ISU-1.2` is a key
		// somebody could have imported and truncating it would turn a right
		// link into a wrong one — the one outcome worth more than a missed id.
		name := strings.TrimRight(word, ".")
		if at := strings.Index(name, ".."); at >= 0 {
			name = strings.TrimRight(name[:at], ".")
		}

		if !strings.HasPrefix(name, prefix+"-") || len(name) <= len(prefix)+1 {
			continue
		}
		if !seen[name] {
			seen[name] = true
			ids = append(ids, name)
		}
	}

	return ids
}

// idRune reports whether a rune can be part of an id, which is what splits a
// subject into candidate words.
func idRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '-', r == '_', r == '.':
		return true
	default:
		return false
	}
}
