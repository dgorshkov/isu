package issue

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// stubRandom makes the generator deterministic for the length of one test, so
// that two ids can be compared for having been built from the same inputs.
func stubRandom(t *testing.T) {
	t.Helper()

	restore := randRead
	t.Cleanup(func() { randRead = restore })

	randRead = func(b []byte) (int, error) {
		for i := range b {
			b[i] = byte(i)
		}

		return len(b), nil
	}
}

// NewToken's one failure is the one it cannot do anything about, and a failure
// path with no test is a failure path that has never run.
func TestNewTokenReportsAFailureToReadRandomness(t *testing.T) {
	restore := randRead
	t.Cleanup(func() { randRead = restore })

	empty := errors.New("the entropy pool is empty")
	randRead = func([]byte) (int, error) { return 0, empty }

	created := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	_, err := NewToken("Login retries stop", "dmitry", created)
	require.ErrorIs(t, err, empty)

	_, err = NewID("ISU", "Login retries stop", "dmitry", created)
	require.ErrorIs(t, err, empty)
}

// Generating an id needs neither the network nor a read of issues/.
//
// The proof is in two halves, because "it did not read anything" is not
// something a return value can say. First: with the randomness pinned, the id
// generated inside a 5,000-issue repository is identical to the one generated
// in an empty directory, so nothing about the repository reached the digest.
// Second: the same id comes back with the process's working directory deleted
// out from under it, where any relative read would fail rather than quietly
// succeed.
//
// This test is not parallel. It moves the working directory, which belongs to
// the process rather than to the test.
func TestNewTokenReadsNothing(t *testing.T) {
	stubRandom(t)

	repo := fiveThousandIssues(t)
	created := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	generate := func() string {
		tok, err := NewToken("Login retries stop", "dmitry", created)
		require.NoError(t, err)

		return tok
	}

	inRepo := inDir(t, repo, generate)
	require.Equal(t, inRepo, inDir(t, t.TempDir(), generate),
		"a 5,000-issue repository under the process must not change the id")

	gone := filepath.Join(t.TempDir(), "gone")
	require.NoError(t, os.Mkdir(gone, 0o755))

	before, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(gone))
	require.NoError(t, os.Remove(gone))

	tok, genErr := NewToken("Login retries stop", "dmitry", created)

	require.NoError(t, os.Chdir(before))
	require.NoError(t, genErr)
	require.Equal(t, inRepo, tok,
		"an id must be generated with no working directory to read at all")
}

// Every one of the six characters must be able to be any of the thirty-two
// symbols. This is the test that catches a token whose entropy has quietly
// narrowed: a wrong shift that zero-fills the low characters, an alphabet
// indexed with the wrong mask, a position stuck on one symbol. A distinctness
// count over random draws cannot see any of those — a stuck last character
// still leaves 2000 draws almost entirely distinct.
//
// It runs against token() with a counter for entropy rather than against
// NewToken with crypto/rand, so it is not a sample: the inputs are fixed, the
// outputs are fixed, and a run that passes today passes every time.
func TestTokenReachesEverySymbolInEveryPosition(t *testing.T) {
	t.Parallel()

	const (
		alphabet = "0123456789abcdefghjkmnpqrstvwxyz"
		draws    = 2000
	)

	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	seen := make([]map[rune]bool, TokenLength)
	for i := range seen {
		seen[i] = map[rune]bool{}
	}

	entropy := make([]byte, entropyBytes)
	for n := range draws {
		binary.BigEndian.PutUint64(entropy, uint64(n))

		tok := token("Login retries stop", "dmitry", day, entropy)
		require.Len(t, tok, TokenLength)

		for i, r := range tok {
			require.Contains(t, alphabet, string(r), "token %q", tok)
			seen[i][r] = true
		}
	}

	for i, symbols := range seen {
		require.Len(t, symbols, len(alphabet),
			"character %d of the token only ever took %d of the %d symbols "+
				"over %d draws, so the token carries less entropy than it claims",
			i, len(symbols), len(alphabet), draws)
	}
}

// And every bit of the randomness has to reach the digest. Passing eight bytes
// to crypto/rand and folding four of them into the hash is a halving nothing
// else here would notice: the alphabet still fills, the tokens still differ,
// and the collision rate quietly goes up by a factor of sixty-five thousand.
func TestTokenDependsOnEveryEntropyBit(t *testing.T) {
	t.Parallel()

	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	base := []byte{0x5a, 0xa5, 0x0f, 0xf0, 0x33, 0xcc, 0x69, 0x96}

	want := token("Login retries stop", "dmitry", day, base)

	for bit := range entropyBytes * 8 {
		flipped := make([]byte, len(base))
		copy(flipped, base)
		flipped[bit/8] ^= 1 << (bit % 8)

		require.NotEqual(t, want, token("Login retries stop", "dmitry", day, flipped),
			"flipping bit %d of the entropy left the token unchanged, "+
				"so that bit is not reaching the digest", bit)
	}
}

// The digest covers every input, with the fields separated rather than run
// together: a title ending in the owner's name must not hash the same as a
// shorter title and a longer owner.
func TestTokenCoversEveryInput(t *testing.T) {
	t.Parallel()

	entropy := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	day := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)

	base := token("Login retries stop", "dmitry", day, entropy)

	require.NotEqual(t, base, token("Login retries stops", "dmitry", day, entropy))
	require.NotEqual(t, base, token("Login retries stop", "dmitri", day, entropy))
	require.NotEqual(t, base, token("Login retries stop", "dmitry", day.AddDate(0, 0, 1), entropy))
	require.NotEqual(t, base, token("Login retries stop", "dmitry", day,
		[]byte{1, 2, 3, 4, 5, 6, 7, 9}))

	require.Equal(t, base, token("Login retries stop", "dmitry", day, entropy),
		"the same inputs must give the same token")

	// The separator is what makes the boundary between two fields real.
	require.NotEqual(t,
		token("Login retries stopdmitry", "", day, entropy),
		token("Login retries stop", "dmitry", day, entropy))
}

// inDir runs fn with the process's working directory moved, and puts it back.
func inDir[T any](t *testing.T, dir string, fn func() T) T {
	t.Helper()

	before, err := os.Getwd()
	require.NoError(t, err)

	require.NoError(t, os.Chdir(dir))
	defer func() { require.NoError(t, os.Chdir(before)) }()

	return fn()
}

// fiveThousandIssues builds a repository big enough that reading it would be
// obvious, and returns its root.
func fiveThousandIssues(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	issues := filepath.Join(root, "issues")

	if err := os.Mkdir(issues, 0o755); err != nil {
		t.Fatalf("building the fixture: %v", err)
	}

	for n := range 5000 {
		id := fmt.Sprintf("ISU-%06d", n)

		dir := filepath.Join(issues, id)
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatalf("building the fixture: %v", err)
		}

		readme := fmt.Sprintf(
			"---\nschema: 1\nid: %s\ntitle: %s\ntype: chore\nstate: open\n"+
				"owner: tester\ncreated: 2026-08-24\n---\n", id, id)
		if err := os.WriteFile(filepath.Join(dir, ReadmeName), []byte(readme), 0o644); err != nil {
			t.Fatalf("building the fixture: %v", err)
		}
	}

	return root
}
