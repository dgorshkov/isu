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
// Those several are separated by anything that cannot be part of an id, which
// is the same rule the subject tier reads by. Splitting on a comma alone was
// isu's own habit mistaken for a convention: git says nothing about what goes
// inside a trailer's value, and a list somebody typed is as likely to be
// separated by spaces.
//
// The cost of reading that list wrong is not a missing link, which is what made
// it worth fixing. An empty trailer tier falls through to the subject, and the
// subject is whatever the forge composed — so a commit whose trailer named two
// issues resolved a third one instead, and nothing downstream could tell that
// link from a right one.
//
// Splitting on whitespace then needs the shape rule below, because ValidID is
// permissive by design — an imported issue keeps its source key verbatim, so
// every word of an English sentence passes it. Reading a comma-separated value
// as one token was doing that filtering by accident: `the login one` is not an
// id only because of the spaces in it. So a token is taken as an id here when
// it carries an interior hyphen, which is what a key looks like in isu and in
// every tracker one would be imported from — and is strictly weaker than the
// subject tier's rule, which demands this repository's own prefix before it.
func trailerIDs(body string) []string {
	var ids []string

	seen := map[string]bool{}

	for line := range strings.Lines(body) {
		token, value, ok := strings.Cut(line, ":")
		if !ok || !strings.EqualFold(strings.TrimSpace(token), ResolvesTrailer) {
			continue
		}

		for _, name := range strings.FieldsFunc(value, func(r rune) bool { return !idRune(r) }) {
			if keyShaped(name) && issue.ValidID(name) && !seen[name] {
				seen[name] = true
				ids = append(ids, name)
			}
		}
	}

	return ids
}

// keyShaped reports whether a word looks like an issue key rather than like a
// word: something, a hyphen, something. It is what lets a trailer's value be a
// list separated by spaces without every trailer written in prose naming three
// issues.
func keyShaped(s string) bool {
	at := strings.Index(s, "-")

	return at > 0 && at < len(s)-1
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
