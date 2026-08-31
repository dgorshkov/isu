// Package cli is isu's command surface.
//
// Everything here is parameterised over its streams, its working directory, its
// clock and its environment, so that a test drives a whole command in process
// against a repository the harness scripted — no built binary, no mocks, and no
// git that is not the user's own. PLAN.md M4-S1 asks for that harness with the
// first command rather than after the last: a milestone that builds eight
// commands and writes its first end-to-end test at the end has seven commands
// nobody ever ran.
//
// # The output contract
//
// Half the users of this tool are agents, so every command supports --json and
// the shape it prints is a contract rather than a convenience. It is documented
// in docs/json.md and defined in payload.go, and contract_test.go fails the
// build when a command has no row in it.
//
// Output is newline-delimited JSON: one complete JSON value per line. Most
// commands print exactly one; `ready` prints one issue per line, so that
// `isu ready --json | head -1` is the top of the queue rather than an opening
// brace.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/repo"
)

// Version is what --version prints. Release builds override it through
// -ldflags "-X github.com/dgorshkov/isu/internal/cli.Version=..." in M9-S1;
// anything else is a development build and says so.
var Version = "0.1.0-dev"

// The exit codes. They are three because a caller has to tell "you asked for
// something impossible" from "what you asked for did not work" — an agent
// retrying the first will retry forever.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// Env is everything a command reads from outside itself.
//
// It exists so that the tests are end-to-end through the product without being
// end-to-end through the operating system: a command's streams, its working
// directory, its clock and its environment are all arguments, and the only
// thing left ambient is the git binary and the repository on disk, which are
// the two things the tests want real.
type Env struct {
	// Args is the command line, without the program name.
	Args []string
	// Stdout is where output goes. Errors never do.
	Stdout io.Writer
	// Stderr is where errors and warnings go, so that `isu ready --json | jq`
	// never sees a warning as data.
	Stderr io.Writer
	// Dir is the directory isu is being run in. Empty means the process's own.
	Dir string
	// Getenv reads the environment. Nil means the process's own.
	Getenv func(string) string
	// Now is the clock claim ages and freshness are measured against. Nil means
	// the real one.
	Now func() time.Time
}

func (e Env) getenv(name string) string {
	if e.Getenv == nil {
		return os.Getenv(name)
	}

	return e.Getenv(name)
}

func (e Env) now() time.Time {
	if e.Now == nil {
		return time.Now()
	}

	return e.Now()
}

// dir is where isu was run. A working directory the process cannot name is an
// empty one, which fails at the first git command with a message naming the
// path — rather than here, with a message about a system call.
func (e Env) dir() string {
	if e.Dir != "" {
		return e.Dir
	}

	wd, _ := os.Getwd()

	return wd
}

// Run is the whole CLI.
func Run(env Env) int {
	a := &app{env: env}

	root := a.root()
	root.SetArgs(env.Args)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)

	if err := root.ExecuteContext(context.Background()); err != nil {
		return a.fail(err)
	}

	return exitOK
}

// app is one invocation: the global flags, and the environment they act in.
type app struct {
	env Env

	repoPath string
	ref      string
	asJSON   bool
	noColor  bool
	fetch    bool
}

// usageError is a command line that does not make sense, as opposed to one that
// did not work. It exits 2 and prints the usage of the command that could not
// read it.
type usageError struct {
	cmd *cobra.Command
	err error
}

func (e *usageError) Error() string { return e.err.Error() }
func (e *usageError) Unwrap() error { return e.err }

func usagef(cmd *cobra.Command, format string, args ...any) error {
	return &usageError{cmd: cmd, err: fmt.Errorf(format, args...)}
}

// fail reports an error the way the invocation asked to be spoken to, and says
// what it exits with.
func (a *app) fail(err error) int {
	var usage *usageError

	code := exitFailure
	if errors.As(err, &usage) {
		code = exitUsage
	}

	if a.asJSON {
		// An agent parsing stderr should not have to find one JSON object in a
		// page of usage text, so the JSON error is the whole of the output.
		_ = json.NewEncoder(a.env.Stderr).Encode(Failure{Error: err.Error()})

		return code
	}

	_, _ = fmt.Fprintf(a.env.Stderr, "isu: %v\n", err)

	if code == exitUsage {
		_, _ = fmt.Fprint(a.env.Stderr, "\n"+usage.cmd.UsageString())
	}

	return code
}

const rootLong = `isu is an issue tracker with no database.

Issues are folders inside the repository, so the pull request that fixes a bug
also closes it, in the same diff. Status is never stored: it is derived from
what trunk and every branch say about an issue, which is why a claim is a branch
and not a row in a table.

Every command supports --json, because half the people reading this are agents.
The shape is documented in docs/json.md and is a contract.`

func (a *app) root() *cobra.Command {
	root := &cobra.Command{
		Use:     "isu",
		Short:   "an issue tracker with no database",
		Long:    rootLong,
		Version: Version,
		// isu reports its own failures, in its own words and on its own stream.
		SilenceUsage:  true,
		SilenceErrors: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usagef(cmd, "there is no `isu %s`", args[0])
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return usagef(cmd, "isu needs a command")
		},
	}

	root.SetVersionTemplate("{{.Version}}\n")

	// cobra offers to generate shell completions. It is a command surface
	// nothing in PLAN.md asks for and one more thing every `isu --help` has to
	// explain, so it is off until somebody wants it.
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetFlagErrorFunc(func(cmd *cobra.Command, err error) error {
		return &usageError{cmd: cmd, err: err}
	})

	flags := root.PersistentFlags()
	flags.StringVar(&a.repoPath, "repo", "",
		"the repository to read (default: the working directory)")
	flags.StringVar(&a.ref, "ref", "",
		"the ref isu treats as trunk (default: main, else master, else HEAD)")
	flags.BoolVar(&a.asJSON, "json", false,
		"print newline-delimited JSON instead of text")
	flags.BoolVar(&a.noColor, "no-color", false,
		"never colour the output")
	flags.BoolVar(&a.fetch, "fetch", false,
		"update remote refs before reading")

	root.AddCommand(
		a.boardCmd(),
		a.showCmd(),
		a.readyCmd(),
		a.newCmd(),
		a.claimCmd(),
		a.unclaimCmd(),
		a.resolveCmd(),
		a.dropCmd(),
		a.commentCmd(),
		a.triageCmd(),
		a.initCmd(),
	)

	return root
}

// session is a repository opened for one command: the git binding, the loader,
// and the configuration that says what this repository's rules are.
type session struct {
	app  *app
	git  *gitx.Git
	repo *repo.Repo
	cfg  config.Config
	// root is the top of the working tree, which is where issues/ is anchored
	// however deep in it the user was standing.
	root string
	// trunk is the ref this invocation treats as trunk.
	trunk string
}

// toplevel is the root of the working tree a directory sits inside.
//
// Everything is anchored there — issues/ and .isu.yml both — however deep in it
// the user was standing when they typed the command.
func toplevel(ctx context.Context, dir string) (string, error) {
	probe, err := gitx.New(dir)
	if err != nil {
		return "", err
	}

	root, err := probe.Toplevel(ctx)
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git working tree: %w", dir, err)
	}

	return root, nil
}

// open finds the repository, reads its configuration, and fetches first if it
// was asked to.
func (a *app) open(ctx context.Context) (*session, error) {
	start := a.repoPath
	if start == "" {
		start = a.env.dir()
	}

	root, err := toplevel(ctx, start)
	if err != nil {
		return nil, err
	}

	loader, err := repo.Open(root)
	if err != nil {
		return nil, err
	}

	cfg, err := config.Load(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf(
				"%s has no %s: isu needs one line of configuration to know what to "+
					"call an issue, and `isu init` writes it",
				root, config.FileName)
		}

		return nil, err
	}

	s := &session{
		app: a, git: loader.Git(), repo: loader, cfg: *cfg, root: root,
		trunk: defaultTrunk(ctx, loader.Git(), a.ref),
	}

	if a.fetch {
		if err := s.refresh(ctx); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// conventionalTrunks are what a repository's trunk is called when nothing else
// says. Two names rather than a survey: a guessing game with five entries is a
// guessing game that gets one wrong.
var conventionalTrunks = []string{"main", "master"}

// originHEAD is where a clone records what the forge calls its default branch.
// It is the only thing in a repository that knows trunk is called `develop`.
const originHEAD = "refs/remotes/origin/HEAD"

// defaultTrunk decides which ref isu treats as trunk.
//
// HEAD is the last answer rather than the first, and that is the whole point of
// this function. Half of what this product says is how a branch differs from
// trunk — a claim is a branch saying resolved where trunk says open — so a
// trunk that follows you onto your feature branch answers every one of those
// questions with "it does not". It also leaves `isu resolve` unable to tell
// that it is about to write straight onto trunk, which is the one thing it
// refuses to do.
//
// The remote's own answer is read before the conventional names, because a
// repository whose trunk is called `develop` has said so once, to git, and
// making somebody repeat it to isu on every command is not a default.
func defaultTrunk(ctx context.Context, git *gitx.Git, given string) string {
	if given != "" {
		return given
	}

	names := conventionalTrunks

	if remote, err := git.SymbolicRef(ctx, originHEAD); err == nil && remote != "" {
		if _, branch, ok := strings.Cut(remote, "/"); ok {
			names = append([]string{branch}, names...)
		}
	}

	for _, name := range names {
		if _, err := git.RevParse(ctx, "refs/heads/"+name); err == nil {
			return name
		}
	}

	// A repository whose trunk is called something else and has no origin to
	// ask, or one with no commits at all. HEAD is the honest answer to both,
	// and --ref is the fix for the first.
	return "HEAD"
}

// refresh updates remote-tracking refs.
//
// A repository with no remote is a no-op rather than a failure: every derived
// status is a statement about refs, and in a repository whose refs are all
// local there is nothing to be behind on. Asking to be up to date and being
// told there is no remote would be a command failing at doing nothing.
func (s *session) refresh(ctx context.Context) error {
	remotes, err := s.git.Remotes(ctx)
	if err != nil {
		return err
	}

	for _, remote := range remotes {
		if err := s.git.Fetch(ctx, remote, "--prune"); err != nil {
			return fmt.Errorf("fetching %s: %w", remote, err)
		}
	}

	return nil
}
