package issue_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
)

func TestNewTokenIsSixCharactersOfTheAlphabet(t *testing.T) {
	t.Parallel()

	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

	created := mustDate(t, "2026-08-24")

	for range 2000 {
		tok, err := issue.NewToken("Login retries stop", "dmitry", created)
		require.NoError(t, err)
		require.Len(t, tok, issue.TokenLength)

		for _, r := range tok {
			require.Contains(t, alphabet, string(r), "token %q", tok)
		}
	}
}

// i, l, o and u are absent so that nothing in an id is ambiguous read aloud or
// typed from a screenshot. Two thousand tokens is twelve thousand characters,
// which is enough to notice a leak from any of the thirty-two positions.
func TestNewTokenNeverEmitsAnAmbiguousLetter(t *testing.T) {
	t.Parallel()

	created := mustDate(t, "2026-08-24")

	for range 2000 {
		tok, err := issue.NewToken("Login retries stop", "dmitry", created)
		require.NoError(t, err)
		require.NotContains(t, tok, "i")
		require.NotContains(t, tok, "l")
		require.NotContains(t, tok, "o")
		require.NotContains(t, tok, "u")
	}
}

// The same title, owner and date, different random bytes, different id. That
// is the whole reason allocation needs no coordination: two clones creating
// the same issue at the same moment do not agree on a number, they disagree on
// thirty bits of hash.
func TestNewTokenIsDifferentEveryTime(t *testing.T) {
	t.Parallel()

	created := mustDate(t, "2026-08-24")

	seen := map[string]bool{}
	for range 2000 {
		tok, err := issue.NewToken("Login retries stop", "dmitry", created)
		require.NoError(t, err)
		seen[tok] = true
	}

	// Thirty bits and two thousand draws: the birthday bound puts the expected
	// number of collisions under two, so anything close to two thousand
	// distinct tokens is the generator working and anything far below it is a
	// generator that is not using its randomness.
	require.Greater(t, len(seen), 1900)
}

func TestNewIDIsThePrefixAndAToken(t *testing.T) {
	t.Parallel()

	id, err := issue.NewID("ISU", "Login retries stop", "dmitry", mustDate(t, "2026-08-24"))
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(id, "ISU-"), "id %q", id)
	require.Len(t, id, len("ISU-")+issue.TokenLength)
	require.True(t, issue.ValidID(id), "a generated id must validate as one")
}

// Generated ids are one shape; valid ids are another and wider one. Imported
// issues keep their source key verbatim, so PROJ-1234 has to be readable even
// though nothing here would ever produce it.
func TestAnImportedKeyIsAValidID(t *testing.T) {
	t.Parallel()

	require.True(t, issue.ValidID("PROJ-1234"))

	i := decode(t, frontmatter(
		"schema: 1", "id: PROJ-1234", "title: Imported from Jira",
		"type: chore", "state: open", "owner: dmitry", "created: 2026-07-14"))
	i.Folder = "PROJ-1234"

	require.NoError(t, i.Validate())
}

func TestNewIDDoesNotDependOnTheClock(t *testing.T) {
	t.Parallel()

	// created is an input, not a reading of the clock: an importer generating
	// an id for a 2019 ticket must be able to say so.
	old, err := issue.NewToken("Login retries stop", "dmitry", mustDate(t, "2019-01-01"))
	require.NoError(t, err)
	require.Len(t, old, issue.TokenLength)
}

func BenchmarkNewToken(b *testing.B) {
	created := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	for range b.N {
		if _, err := issue.NewToken("Login retries stop", "dmitry", created); err != nil {
			b.Fatal(err)
		}
	}
}
