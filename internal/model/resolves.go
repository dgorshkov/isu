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
// Two of PLAN.md's three tiers are here, because they are the two that are in a
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

	if prefix != "" {
		if ids := subjectIDs(prefix, subject); len(ids) > 0 {
			return ids, TierSubject
		}
	}

	return nil, TierNone
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

	for line := range strings.Lines(body) {
		token, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(token), ResolvesTrailer) {
			continue
		}

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
		name := strings.TrimRight(word, ".")

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
