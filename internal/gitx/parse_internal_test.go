package gitx

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// These are the shapes real git never produces. They are tested anyway because
// the alternative to a typed error is a panic in the middle of a board render,
// and because a future git that changes one of these formats should fail
// loudly here rather than quietly return half a repository.

func TestParseBatchRefusesMalformedStreams(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stream string
		msg    string
	}{
		{
			name:   "a header that is not three fields",
			stream: "deadbeef blob\n",
			msg:    "is not a batch header",
		},
		{
			name:   "a size that is not a number",
			stream: "deadbeef blob banana\nhello\n",
			msg:    "is not an object size",
		},
		{
			name:   "an object cut short",
			stream: "deadbeef blob 100\nhello\n",
			msg:    "reading object deadbeef",
		},
		{
			name:   "an object with no trailing newline",
			stream: "deadbeef blob 5\nhello",
			msg:    "reading object deadbeef",
		},
		{
			name:   "a missing object",
			stream: "deadbeef missing\n",
			msg:    "is missing from this repository",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := parseBatch(strings.NewReader(tc.stream), func(Object) error { return nil })
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestParseBatchReadsBackToBackObjects(t *testing.T) {
	stream := "aaa blob 2\nhi\n" + "bbb blob 0\n\n" + "ccc blob 3\nbye\n"

	var got []Object
	require.NoError(t, parseBatch(strings.NewReader(stream), func(o Object) error {
		got = append(got, o)

		return nil
	}))

	require.Len(t, got, 3)
	require.Equal(t, "hi", string(got[0].Data))
	require.Empty(t, got[1].Data, "an empty blob is an object, not the end of the stream")
	require.Equal(t, "bye", string(got[2].Data))
}

func TestParseLogRefusesMalformedRecords(t *testing.T) {
	for _, tc := range []struct {
		name string
		out  string
		msg  string
	}{
		{
			name: "too few fields",
			out:  logRecordSeparator + "deadbeef\x00\x00who",
			msg:  "is not a commit",
		},
		{
			name: "an author date that is not a date",
			out: logRecordSeparator + strings.Join([]string{
				"deadbeef", "", "who", "who@example.invalid", "never",
				"who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"subject", "",
			}, "\x00"),
			msg: "is not a date",
		},
		{
			name: "a committer date that is not a date",
			out: logRecordSeparator + strings.Join([]string{
				"deadbeef", "", "who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"who", "who@example.invalid", "never",
				"subject", "",
			}, "\x00"),
			msg: "is not a date",
		},
		{
			name: "a raw entry that is not one",
			out: logRecordSeparator + strings.Join([]string{
				"deadbeef", "", "who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"subject", "", "\nnot a change", "issues/AR-7f3akq/README.md",
			}, "\x00"),
			msg: "is not a change",
		},
		{
			name: "a raw entry with the wrong field count",
			out: logRecordSeparator + strings.Join([]string{
				"deadbeef", "", "who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"subject", "", "\n:100644 100644 aaa M", "issues/AR-7f3akq/README.md",
			}, "\x00"),
			msg: "is not a change",
		},
		{
			name: "a raw entry with no path",
			out: logRecordSeparator + strings.Join([]string{
				"deadbeef", "", "who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"who", "who@example.invalid", "2026-08-26T00:00:00+00:00",
				"subject", "", "\n:100644 100644 aaa bbb M",
			}, "\x00"),
			msg: "has no path",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseLog(tc.out)
			require.ErrorContains(t, err, tc.msg)
		})
	}
}

func TestParseLogOnNothingIsNoCommits(t *testing.T) {
	commits, err := parseLog("")
	require.NoError(t, err)
	require.Empty(t, commits)
}

func TestLsTreeRefusesMalformedEntries(t *testing.T) {
	for _, tc := range []struct{ name, record string }{
		{name: "no tab", record: "100644 blob deadbeef issues/AR-7f3akq/README.md"},
		{name: "too few fields", record: "100644 deadbeef\tissues/AR-7f3akq/README.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTree(tc.record + "\x00")
			require.ErrorContains(t, err, "is not a tree entry")
		})
	}
}

func TestSplitNUL(t *testing.T) {
	require.Nil(t, splitNUL(""))
	require.Nil(t, splitNUL("\x00"))
	require.Equal(t, []string{"a"}, splitNUL("a\x00"))
	require.Equal(t, []string{"a", "b"}, splitNUL("a\x00b\x00"))
	require.Equal(t, []string{"a", "b"}, splitNUL("a\x00b"))
}
