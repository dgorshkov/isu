package issue_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
)

// What NewToken itself has to prove is the narrow thing the deterministic
// tests beside it cannot: that the public entry point consults the random
// source at all, and that what comes back is the shape the schema promises.
//
// It deliberately does **not** try to measure how much entropy is in there.
// Two thousand draws over thirty bits expect 0.002 collisions, and two
// thousand draws over *twenty* bits expect 1.9 — so any distinctness threshold
// loose enough not to flake is also loose enough to miss ten lost bits. That
// property is proven against token() directly, where the inputs are chosen
// rather than sampled.
func TestNewToken(t *testing.T) {
	t.Parallel()

	const alphabet = "0123456789abcdefghjkmnpqrstvwxyz"

	created := mustDate(t, "2026-08-24")

	seen := map[string]bool{}

	for range 2000 {
		tok, err := issue.NewToken("Login retries stop", "dmitry", created)
		require.NoError(t, err)
		require.Len(t, tok, issue.TokenLength)

		for _, r := range tok {
			require.Contains(t, alphabet, string(r), "token %q", tok)
			require.NotContains(t, "ilou", string(r),
				"nothing in an id may be ambiguous read aloud: token %q", tok)
		}

		seen[tok] = true
	}

	// The same title, owner and date every time, so a generator that ignored
	// its randomness would produce exactly one distinct token.
	require.Greater(t, len(seen), 1,
		"NewToken must read the random source, not just hash its arguments")
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
