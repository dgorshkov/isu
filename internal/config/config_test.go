package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/issue"
)

func TestParseEveryKeyAtItsSetValue(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte(strings.Join([]string{
		"prefix: PROJ",
		"agents:",
		"  - claude",
		"  - dependabot[bot]",
		"direct_triage: true",
		"stale_days: 3",
		"fetch_warn_hours: 6",
		"attachment_max_bytes: 1048576",
	}, "\n") + "\n"))
	require.NoError(t, err)

	require.Equal(t, "PROJ", cfg.Prefix)
	require.Equal(t, []string{"claude", "dependabot[bot]"}, cfg.Agents)
	require.True(t, cfg.DirectTriage)
	require.Equal(t, 3, cfg.StaleDays)
	require.Equal(t, 6, cfg.FetchWarnHours)
	require.Equal(t, 1048576, cfg.AttachmentMaxBytes)

	require.Equal(t, 72*time.Hour, cfg.StaleAfter())
	require.Equal(t, 6*time.Hour, cfg.FetchWarnAfter())
}

// Every key that has a default gets it, and the one that does not is required.
func TestParseAppliesEveryDefault(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("prefix: ISU\n"))
	require.NoError(t, err)

	want := config.Default()
	want.Prefix = "ISU"
	require.Equal(t, &want, cfg)

	require.Empty(t, cfg.Agents)
	require.False(t, cfg.DirectTriage)
	require.Equal(t, config.DefaultStaleDays, cfg.StaleDays)
	require.Equal(t, config.DefaultFetchWarnHours, cfg.FetchWarnHours)
	require.Equal(t, config.DefaultAttachmentMaxBytes, cfg.AttachmentMaxBytes)

	require.Equal(t, 7*24*time.Hour, cfg.StaleAfter())
	require.Equal(t, 24*time.Hour, cfg.FetchWarnAfter())
}

// A zero is a value somebody wrote, not an absent key: attachment_max_bytes: 0
// means no attachments, and quietly replacing it with the default would be
// isu deciding it knew better.
func TestParseKeepsAZeroThatWasWrittenDown(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("prefix: ISU\nstale_days: 0\nattachment_max_bytes: 0\n"))
	require.NoError(t, err)

	require.Equal(t, 0, cfg.StaleDays)
	require.Equal(t, 0, cfg.AttachmentMaxBytes)
	require.Zero(t, cfg.StaleAfter())
}

func TestParseAnEmptyAgentsList(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("prefix: ISU\nagents: []\n"))
	require.NoError(t, err)
	require.Empty(t, cfg.Agents)
}

func TestParseRejects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		keys  []string
		msg   string
	}{
		{
			name:  "an empty file",
			input: "",
			keys:  []string{"prefix"},
			msg:   "prefix: required",
		},
		{
			name:  "a file with no prefix",
			input: "stale_days: 3\n",
			keys:  []string{"prefix"},
			msg:   "prefix: required",
		},
		{
			name:  "an unknown key",
			input: "prefix: ISU\nstale_hours: 3\n",
			keys:  []string{"stale_hours"},
			msg: "stale_hours: unknown key: this file's keys are prefix, agents, " +
				"direct_triage, stale_days, fetch_warn_hours, attachment_max_bytes",
		},
		{
			name:  "a key that is nearly right",
			input: "prefix: ISU\nagent: claude\n",
			keys:  []string{"agent"},
			msg:   "agent: unknown key",
		},
		{
			name:  "a prefix that is not a string",
			input: "prefix: 42\n",
			keys:  []string{"prefix"},
			msg:   "prefix: expected a string, found 42",
		},
		{
			name:  "a prefix that cannot be part of a folder name",
			input: "prefix: my project\n",
			keys:  []string{"prefix"},
			msg:   `"my project" cannot be a prefix`,
		},
		{
			name:  "agents written as one string",
			input: "prefix: ISU\nagents: claude\n",
			keys:  []string{"agents"},
			msg:   `agents: expected a list of strings, found the string "claude"`,
		},
		{
			name:  "an agent that is not a string",
			input: "prefix: ISU\nagents:\n  - claude\n  - 42\n",
			keys:  []string{"agents"},
			msg:   "agents: entry 2: expected a string, found 42",
		},
		{
			name:  "direct_triage that is not a bool",
			input: "prefix: ISU\ndirect_triage: yes please\n",
			keys:  []string{"direct_triage"},
			msg:   `direct_triage: expected true or false, found the string "yes please"`,
		},
		{
			name:  "stale_days that is not a number",
			input: "prefix: ISU\nstale_days: a week\n",
			keys:  []string{"stale_days"},
			msg:   `stale_days: expected a whole number, found the string "a week"`,
		},
		{
			name:  "stale_days that is not whole",
			input: "prefix: ISU\nstale_days: 1.5\n",
			keys:  []string{"stale_days"},
			msg:   "stale_days: expected a whole number, found 1.5",
		},
		{
			name:  "a negative fetch_warn_hours",
			input: "prefix: ISU\nfetch_warn_hours: -1\n",
			keys:  []string{"fetch_warn_hours"},
			msg:   "fetch_warn_hours: expected a whole number that is not negative, found -1",
		},
		{
			name:  "a key set to nothing",
			input: "prefix: ISU\nattachment_max_bytes:\n",
			keys:  []string{"attachment_max_bytes"},
			msg:   "attachment_max_bytes: expected a whole number, found nothing",
		},
		{
			name:  "a mapping where a scalar belongs",
			input: "prefix:\n  name: ISU\n",
			keys:  []string{"prefix"},
			msg:   "prefix: expected a string, found a mapping",
		},
		{
			name:  "a list where a scalar belongs",
			input: "prefix: [ISU]\n",
			keys:  []string{"prefix"},
			msg:   "prefix: expected a string, found a list",
		},
		{
			name:  "several problems at once",
			input: "stale_days: -1\nnonsense: 1\ndirect_triage: 3\n",
			keys:  []string{"direct_triage", "nonsense", "prefix", "stale_days"},
			msg:   "4 problems:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg, err := config.Parse([]byte(tc.input))
			require.Nil(t, cfg)
			require.Error(t, err)

			var cerr *config.Error
			require.ErrorAs(t, err, &cerr)

			got := make([]string, 0, len(cerr.Problems))
			for _, p := range cerr.Problems {
				got = append(got, p.Key)
			}

			require.Equal(t, tc.keys, got)
			require.Contains(t, err.Error(), tc.msg)
		})
	}
}

func TestParseRejectsAFileThatIsNotAMapping(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("- prefix\n- ISU\n"))
	require.Nil(t, cfg)
	require.ErrorContains(t, err, "not a YAML mapping")

	cfg, err = config.Parse([]byte("prefix: 'unterminated\n"))
	require.Nil(t, cfg)
	require.ErrorContains(t, err, "not a YAML mapping")
}

func TestErrorRendersOneProblemOnOneLine(t *testing.T) {
	t.Parallel()

	_, err := config.Parse([]byte("stale_days: 3\n"))
	require.EqualError(t, err,
		"1 problem: prefix: required: it is the first half of every issue id")
}

func TestLoad(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, config.FileName), []byte("prefix: ISU\nstale_days: 3\n"), 0o644))

	cfg, err := config.Load(root)
	require.NoError(t, err)
	require.Equal(t, "ISU", cfg.Prefix)
	require.Equal(t, 3, cfg.StaleDays)
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()

	t.Run("a repository with no .isu.yml", func(t *testing.T) {
		t.Parallel()

		cfg, err := config.Load(t.TempDir())
		require.Nil(t, cfg)
		require.ErrorContains(t, err, "reading .isu.yml")
	})

	t.Run("a .isu.yml that does not validate", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		require.NoError(t, os.WriteFile(
			filepath.Join(root, config.FileName), []byte("stale_days: 3\n"), 0o644))

		cfg, err := config.Load(root)
		require.Nil(t, cfg)
		require.ErrorContains(t, err, config.FileName+": 1 problem: prefix: required")
	})
}

// isu's own .isu.yml is the first file this parser will ever be pointed at, so
// it is worth knowing it parses rather than assuming it.
func TestThisRepositorysOwnConfigParses(t *testing.T) {
	t.Parallel()

	cfg, err := config.Load(filepath.Join("..", ".."))
	require.NoError(t, err)
	require.Equal(t, "ISU", cfg.Prefix)
	require.Equal(t, config.DefaultStaleDays, cfg.StaleDays)
}

// The spelling PLAN.md's M1-S4 names, over the pure generator in the issue
// package: the prefix comes from the configuration rather than from the call.
func TestConfigNewID(t *testing.T) {
	t.Parallel()

	cfg, err := config.Parse([]byte("prefix: PROJ\n"))
	require.NoError(t, err)

	id, err := cfg.NewID("Login retries stop", "dmitry", time.Now())
	require.NoError(t, err)

	require.True(t, strings.HasPrefix(id, "PROJ-"), "id %q", id)
	require.True(t, issue.ValidID(id))
}

func TestProblemString(t *testing.T) {
	t.Parallel()

	require.Equal(t, "prefix: required",
		config.Problem{Key: "prefix", Message: "required"}.String())
}
