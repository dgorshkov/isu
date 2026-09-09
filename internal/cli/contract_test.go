package cli

import (
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/gittest"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// jsonCases is one invocation of every command isu registers that must produce
// JSON, and it is the gate M4-S1 asks for: adding a command without
// --json fails the test suite, because a command with no row here fails
// TestEveryCommandIsInTheJSONContract before it ever gets to run.
//
// A table rather than a loop over the registered commands with no arguments,
// because half of them need an issue id and a command that refuses its
// arguments has not shown you its output shape.
type jsonCase struct {
	args []string
	// setup puts the repository in the state this command needs — on a branch
	// for the two that refuse to write on trunk, without a configuration for
	// the one that writes it.
	setup func(*gittest.Repo)
}

var jsonCases = map[string]jsonCase{
	"board":   {args: []string{"board"}},
	"show":    {args: []string{"show", "ISU-openly"}},
	"ready":   {args: []string{"ready"}},
	"new":     {args: []string{"new", "--title", "A new report", "--repro", "run it twice"}},
	"claim":   {args: []string{"claim", "ISU-openly"}},
	"unclaim": {args: []string{"unclaim", "ISU-inprog"}},
	"resolve": {
		args:  []string{"resolve", "ISU-openly"},
		setup: onABranch,
	},
	"drop": {
		args:  []string{"drop", "ISU-openly", "--reason", "no", "--resolution", "wontfix"},
		setup: onABranch,
	},
	"comment": {args: []string{"comment", "ISU-openly", "-m", "a comment"}},
	"triage":  {args: []string{"triage", "ISU-openly", "--owner", "alice"}},
	// `isu ui` opens a terminal interface, and --json is what it says to
	// somebody who has not got one: the board it would open on.
	"ui": {args: []string{"ui"}},
	"check": {
		args: []string{"check"},
		// The fixture board has an epic with a child and nothing wrong with it,
		// so this run finds nothing and exits 0 — which is what `ok` needs of
		// every row in this table.
		setup: onABranch,
	},
	"init": {
		args: []string{"init", "--prefix", "NEW"},
		setup: func(r *gittest.Repo) {
			r.Git("rm", "--quiet", ".isu.yml")
			r.Commit("take the configuration away again")
		},
	},
}

func onABranch(r *gittest.Repo) {
	r.Branch("isu/ISU-openly").Checkout("isu/ISU-openly")
}

// commandNames is every command the root registers, which is the set the
// contract has to cover.
func commandNames(t *testing.T) []string {
	t.Helper()

	var names []string

	var walk func(*cobra.Command)

	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			if child.Name() == "help" || child.Name() == "completion" {
				// Both are cobra's own, print no isu data, and are not part of
				// this contract.
				continue
			}

			names = append(names, child.Name())
			walk(child)
		}
	}

	walk((&app{}).root())

	return names
}

func TestEveryCommandIsInTheJSONContract(t *testing.T) {
	t.Parallel()

	for _, name := range commandNames(t) {
		require.Containsf(t, jsonCases, name,
			"%s has no row in jsonCases: every command supports --json, because half "+
				"the users of this tool are agents, and a command nobody proved that of "+
				"is a command that does not", name)
	}

	require.Len(t, jsonCases, len(commandNames(t)),
		"jsonCases names a command that is not registered")
}

func TestEveryCommandSpeaksJSON(t *testing.T) {
	t.Parallel()

	for name, test := range jsonCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// Every case runs against its own repository: these are the write
			// commands as well as the read ones, and a shared fixture would
			// make the result depend on which test ran first.
			r := board(t).WithRemote()
			if test.setup != nil {
				test.setup(r)
			}

			got := isu(t, r.Dir(), append(test.args, "--json")...).ok(t)

			require.NotEmpty(t, got.stdout, "--json must print something")

			for line := range strings.SplitSeq(strings.TrimSpace(got.stdout), "\n") {
				require.True(t, json.Valid([]byte(line)),
					"every line of --json output is one JSON value: %q", line)
			}
		})
	}
}

// documented is every payload type docs/json.md has to describe. Adding one and
// forgetting the document is the drift this test exists to prevent.
var documented = []any{
	Failure{},
	Issue{},
	Claim{},
	Epic{},
	Broken{},
	Freshness{},
	BoardPayload{},
	Group{},
	ShowPayload{},
	Text{},
	Link{},
	Write{},
	CheckPayload{},
	Finding{},
}

func TestJSONDocumentsEveryField(t *testing.T) {
	t.Parallel()

	doc, err := os.ReadFile("../../docs/json.md")
	require.NoError(t, err, "the JSON contract is documented in docs/json.md")

	text := string(doc)

	for _, value := range documented {
		typ := reflect.TypeOf(value)

		require.Containsf(t, text, "## "+typ.Name(),
			"docs/json.md does not describe %s", typ.Name())

		for i := range typ.NumField() {
			tag, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")

			require.Containsf(t, text, "`"+tag+"`",
				"docs/json.md does not document %s.%s", typ.Name(), tag)
		}
	}
}

func TestHelpIsOnStdoutAndIsNotAnError(t *testing.T) {
	t.Parallel()

	r := configured(t)

	for _, args := range [][]string{{"--help"}, {"-h"}} {
		got := isu(t, r.Dir(), args...).ok(t)

		require.Contains(t, got.stdout, "Usage:")
		require.Empty(t, got.stderr)
	}
}

func TestGoldenHelp(t *testing.T) {
	t.Parallel()

	r := configured(t)

	golden(t, "help/root.txt", isu(t, r.Dir(), "--help").ok(t).stdout)

	for _, name := range commandNames(t) {
		got := isu(t, r.Dir(), name, "--help").ok(t)

		golden(t, "help/"+name+".txt", got.stdout)
	}
}

func TestABareInvocationIsAUsageError(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir())

	require.Equal(t, 2, got.code)
	require.Empty(t, got.stdout, "usage is not data")
	require.Contains(t, got.stderr, "isu needs a command")
	require.Contains(t, got.stderr, "Usage:")
}

func TestAnUnknownFlagIsAUsageError(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "board", "--nope")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "unknown flag")
	require.Contains(t, got.stderr, "Usage:")
}

func TestAnUnknownCommandIsAUsageError(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "borad")

	require.Equal(t, 2, got.code)
	require.Contains(t, got.stderr, "borad")
}

func TestArgumentsAreCheckedBeforeTheRepositoryIsOpened(t *testing.T) {
	t.Parallel()

	r := configured(t)

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"board takes none", []string{"board", "extra"}, "takes no arguments"},
		{"show needs one", []string{"show"}, "exactly one issue id"},
		{"show takes one", []string{"show", "a", "b"}, "exactly one issue id"},
		{"and it must be an id", []string{"show", "not/an/id"}, "is not an issue id"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := isu(t, r.Dir(), tt.args...)

			require.Equal(t, 2, got.code)
			require.Contains(t, got.stderr, tt.want)
		})
	}
}

func TestAFailureInJSONIsJSONAndNothingElse(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "--json", "show", "ISU-nobody")

	require.Equal(t, 1, got.code)
	require.Empty(t, got.stdout)
	require.NotContains(t, got.stderr, "Usage:",
		"an agent parsing stderr should find one JSON object, not a page of usage")

	var failure Failure

	require.NoError(t, json.Unmarshal([]byte(got.stderr), &failure))
	require.Contains(t, failure.Error, "ISU-nobody")
}

func TestAUsageErrorInJSONIsAlsoJSON(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "--json", "board", "extra")

	require.Equal(t, 2, got.code)

	var failure Failure

	require.NoError(t, json.Unmarshal([]byte(got.stderr), &failure))
	require.Contains(t, failure.Error, "takes no arguments")
}

func TestVersionIsASemverOnStdout(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir(), "--version").ok(t)

	require.Equal(t, Version+"\n", got.stdout)
	require.Empty(t, got.stderr)
}

func TestFetchAgainstARepositoryWithNoRemoteIsANoOp(t *testing.T) {
	t.Parallel()

	r := board(t)

	// Asking to be up to date in a repository whose refs are all local is a
	// command doing nothing, not a command failing.
	got := isu(t, r.Dir(), "--fetch", "board").ok(t)

	require.Contains(t, got.stdout, "no remote refs")
}

func TestFetchUpdatesBeforeReading(t *testing.T) {
	t.Parallel()

	r := board(t).WithRemote()
	r.Git("update-ref", "-d", "refs/remotes/origin/"+gittest.DefaultBranch)

	got := isu(t, r.Dir(), "--fetch", "board").ok(t)

	require.Contains(t, got.stdout, "remote refs",
		"the freshness line reads what the fetch just wrote")
}

func TestFetchReportsARemoteThatDoesNotAnswer(t *testing.T) {
	t.Parallel()

	r := board(t).WithRemote().DetachRemote()

	got := isu(t, r.Dir(), "--fetch", "board")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "fetching origin")
}

func TestARepositoryWithNoConfigurationSaysHowToWriteOne(t *testing.T) {
	t.Parallel()

	r := gittest.New(t).File("README.md", "no isu here\n").Commit("first")

	got := isu(t, r.Dir(), "board")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, ".isu.yml")
	require.Contains(t, got.stderr, "isu init")
}

func TestSomewhereThatIsNotARepository(t *testing.T) {
	t.Parallel()

	got := isu(t, t.TempDir(), "board")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "not inside a git working tree")
}

func TestARepositoryPathThatIsNotADirectory(t *testing.T) {
	t.Parallel()

	r := configured(t)

	got := isu(t, r.Dir()+"/README.md", "board")

	require.Equal(t, 1, got.code)
	require.Contains(t, got.stderr, "not a directory")
}

func TestTheWorkingDirectoryIsUsedWhenNoRepositoryIsNamed(t *testing.T) {
	t.Parallel()

	r := board(t)

	var stdout, stderr strings.Builder

	code := Run(Env{
		Args:   []string{"board"},
		Stdout: &stdout,
		Stderr: &stderr,
		Dir:    r.Dir(),
		Now:    now,
		Getenv: func(string) string { return "" },
	})

	require.Equalf(t, 0, code, "isu failed: %s", stderr.String())
	require.Contains(t, stdout.String(), "ISU-openly")
}
