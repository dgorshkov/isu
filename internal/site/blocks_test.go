package site

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The console grammar is the whole of what makes documentation executable, so
// it is asserted the way internal/issue asserts frontmatter: one table of
// documents this package accepts, one of documents it refuses, and a reason for
// each refusal.

func TestBlocksReadsCommandsAndTheOutputClaimedForThem(t *testing.T) {
	t.Parallel()

	doc := "prose\n\n```console\n$ isu board\nmain · 0 issues\n\ntrailing\n$ isu ready\n```\n" +
		"```sh\nnot a console block\n```\n" +
		"```console exit=1\n$ isu check\nfail\n```\n"

	blocks, err := Blocks(doc)
	require.NoError(t, err)
	require.Len(t, blocks, 2)

	require.Len(t, blocks[0].Commands, 2)
	require.Equal(t, []string{"board"}, blocks[0].Commands[0].Args)
	require.Equal(t, "main · 0 issues\n\ntrailing\n", blocks[0].Commands[0].Want)
	require.True(t, blocks[0].Commands[0].Asserted)
	require.Equal(t, 0, blocks[0].Commands[0].Exit)

	require.False(t, blocks[0].Commands[1].Asserted,
		"a command with no lines under it claims no output")
	require.Equal(t, "isu ready", blocks[0].Commands[1].String())

	require.Equal(t, 1, blocks[1].Commands[0].Exit)
}

func TestBlocksRefusesADocumentItCannotRunHonestly(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		doc  string
		want string
	}{
		{
			name: "a prompt loose in the prose",
			doc:  "Try this:\n\n$ isu board\n",
			want: "outside a ```console fence",
		},
		{
			name: "a prompt in another fence",
			doc:  "```sh\n$ isu board\nmain\n```\n",
			want: "a transcript nothing runs",
		},
		{
			name: "a command that is not isu",
			doc:  "```console\n$ rm -rf /\n```\n",
			want: "runs isu and nothing else",
		},
		{
			name: "output before any command",
			doc:  "```console\nmain · 0 issues\n$ isu board\n```\n",
			want: "starts with a command",
		},
		{
			name: "an empty block",
			doc:  "```console\n```\n",
			want: "proves nothing",
		},
		{
			name: "a fence word this package does not know",
			doc:  "```console frozen\n$ isu board\n```\n",
			want: "is not something a console fence says",
		},
		{
			name: "an exit status that is not a number",
			doc:  "```console exit=soon\n$ isu board\n```\n",
			want: "is not an exit status",
		},
		{
			name: "an unclosed quote",
			doc:  "```console\n$ isu new --title \"half a title\n```\n",
			want: "unclosed quote",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := Blocks(tt.doc)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestAFenceThatNeverClosesIsStillRead(t *testing.T) {
	t.Parallel()

	blocks, err := Blocks("```console\n$ isu board\nmain\n")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, "main\n\n", blocks[0].Commands[0].Want,
		"the split on newlines leaves a final empty line, and it is trimmed on comparison")
}

func TestALongerFenceIsClosedOnlyByItsOwnMarker(t *testing.T) {
	t.Parallel()

	blocks, err := Blocks("````console\n$ isu board\n```\nstill inside\n````\n")
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, "```\nstill inside\n", blocks[0].Commands[0].Want)
}

func TestWordsHonoursQuotesSoATitleIsOneArgument(t *testing.T) {
	t.Parallel()

	got, err := words(`isu new --title "Login retries drop" --owner dana`)
	require.NoError(t, err)
	require.Equal(t, []string{"isu", "new", "--title", "Login retries drop", "--owner", "dana"},
		got)

	empty, err := words(`isu new --body ""`)
	require.NoError(t, err)
	require.Equal(t, []string{"isu", "new", "--body", ""}, empty,
		"an empty quoted argument is an argument")
}

func TestVerifyRunsEveryBlockAgainstTheRepository(t *testing.T) {
	t.Parallel()

	dir, err := Fixture(t.Context(), t.TempDir())
	require.NoError(t, err)

	require.NoError(t, Verify(dir, "```console\n$ isu board\n"+isu(dir, []string{"board"}).Output+
		"```\n"))
}

func TestVerifySaysWhatTheDocumentGotWrong(t *testing.T) {
	t.Parallel()

	dir, err := Fixture(t.Context(), t.TempDir())
	require.NoError(t, err)

	t.Run("a document this package will not read", func(t *testing.T) {
		t.Parallel()
		require.ErrorContains(t, Verify(dir, "$ isu board\n"), "outside a ```console fence")
	})

	t.Run("the wrong exit status", func(t *testing.T) {
		t.Parallel()

		err := Verify(dir, "```console exit=1\n$ isu board\n```\n")
		require.ErrorContains(t, err, "exited 0, not 1")
	})

	t.Run("output the product does not produce", func(t *testing.T) {
		t.Parallel()

		err := Verify(dir, "```console\n$ isu board\nsomething else entirely\n```\n")
		require.ErrorContains(t, err, "and the document claims")
		require.ErrorContains(t, err, "something else entirely")
	})
}

func TestAnErrorNamesTheCommandAndBothOutputs(t *testing.T) {
	t.Parallel()

	sample := Sample{Command: "isu board", Output: "one\ntwo\n", Exit: 1}

	require.Equal(t, "`isu board` exited 1, not 0:\none\ntwo\n",
		(&ExitError{Sample: sample, Want: 0}).Error())

	message := (&OutputError{Sample: sample, Want: "three\n"}).Error()
	require.Contains(t, message, "\t| one\n\t| two")
	require.Contains(t, message, "\t| three")
	require.True(t, strings.HasPrefix(message, "`isu board` printed"))
}
