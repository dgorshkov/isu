// Package gitx is the only place in isu that executes git.
//
// PLAN.md §0 shells out to the git binary rather than linking a reimplementation
// of it: the user's git configuration, hooks, credential helpers and LFS all
// apply for free, the plumbing commands give exact control over the read path,
// and the fast load path in M2-S2 is only available through plumbing. All of
// that is true only while there is one door. A second place that builds a git
// command is a second place that has to remember --no-pager, the timeout, the
// environment and the error handling, and it will remember three of the four.
//
// M2-S1 makes the rule testable rather than aspirational:
// TestNothingOutsideGitxExecutesGit walks internal/ and fails on an exec.Command
// anywhere but here.
//
// Every wrapper takes a context and every invocation carries a deadline, so a
// git that hangs on a lock, a filter or an unreachable remote fails the command
// rather than the session.
package gitx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

// Binary is the program gitx looks for on PATH.
const Binary = "git"

// DefaultTimeout bounds one git invocation. It is generous rather than tight:
// the operations here are reads of a local repository, but one of them is a
// push over the network and another may wake an LFS filter.
const DefaultTimeout = 2 * time.Minute

// ErrNotFound is a git binary that is not installed. It is detected once, when
// a Git is opened, rather than discovered as a confusing failure inside
// whichever command the user happened to run first.
var ErrNotFound = errors.New("git was not found")

// ErrUnknownRevision is a revision git cannot resolve: a branch that is not
// there, a bad sha, or HEAD in a repository with no commits.
//
// It is a kind of its own because the callers cannot tell those apart from the
// message and have to. An unborn HEAD means an empty repository, which is a
// board with nothing on it; a named ref that does not resolve is a mistake
// worth reporting.
var ErrUnknownRevision = errors.New("unknown revision")

// environmentVars are the variables gitx sets on every invocation.
//
// LC_ALL is here because this package parses git's output and a locale that
// translates it would be parsed wrong. GIT_OPTIONAL_LOCKS is here because
// reading a repository must not take its index lock away from the human using
// it. GIT_PAGER is belt and braces beside --no-pager.
var environmentVars = []string{
	"LC_ALL=C",
	"GIT_PAGER=cat",
	"GIT_OPTIONAL_LOCKS=0",
}

// repositoryVars are the environment variables that point git at a repository
// other than the one it was handed.
//
// They are dropped rather than overridden. A process that inherits them — one
// run from a git hook, from `git bisect run`, or from any tool that exports
// them — would otherwise have isu quietly read, and in M4 write, a repository
// nobody pointed it at. Everything else in the environment survives: the user's
// configuration, their credential helper and their SSH agent are the reason
// PLAN.md shells out at all.
var repositoryVars = []string{
	"GIT_DIR",
	"GIT_WORK_TREE",
	"GIT_INDEX_FILE",
	"GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR",
	"GIT_NAMESPACE",
	"GIT_CEILING_DIRECTORIES",
}

// Git is the git binary, bound to one repository.
type Git struct {
	dir     string
	binary  string
	timeout time.Duration
	env     func([]string) []string

	processes atomic.Int64
}

// Option configures a Git.
type Option func(*Git)

// WithBinary names the git to run. It exists for the tests that put a script
// where git goes, and for a caller that has to run a git other than the first
// one on PATH.
func WithBinary(path string) Option {
	return func(g *Git) { g.binary = path }
}

// WithTimeout bounds one invocation.
func WithTimeout(d time.Duration) Option {
	return func(g *Git) { g.timeout = d }
}

// WithEnv rewrites the environment git runs under, after gitx has scrubbed it.
//
// It is how the test harness pins an identity and a clock, which is the one
// caller that legitimately needs git to see something other than what the user
// has configured. Production code does not use it: the point of shelling out is
// that the user's own configuration applies.
func WithEnv(fn func(base []string) []string) Option {
	return func(g *Git) { g.env = fn }
}

// New binds a Git to a repository directory.
//
// The git binary is resolved here, so that a machine without git says so once,
// clearly, rather than failing inside whichever plumbing command ran first.
func New(dir string, opts ...Option) (*Git, error) {
	g := &Git{dir: dir, binary: Binary, timeout: DefaultTimeout}
	for _, opt := range opts {
		opt(g)
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("opening a repository: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("opening a repository: %s is not a directory", dir)
	}

	if _, err := exec.LookPath(g.binary); err != nil {
		return nil, fmt.Errorf(
			"%w on PATH: git is a runtime requirement of isu, which reads your "+
				"repository with it rather than reimplementing it: %w",
			ErrNotFound, err)
	}

	return g, nil
}

// Dir is the repository this Git is bound to.
func (g *Git) Dir() string { return g.dir }

// Processes is how many git processes this Git has spawned.
//
// M2-S5 asserts a process count rather than a wall-clock time alone, because
// the slow read paths in PLAN.md §0 differ from the fast one by process count
// and not by algorithm — so a regression shows up here before it shows up as
// seconds on somebody's laptop.
func (g *Git) Processes() int64 { return g.processes.Load() }

// Error is a git invocation that failed. It carries stderr, because that is
// where git says what went wrong, and the arguments, because "exit status 128"
// on its own names neither the command nor the repository.
type Error struct {
	// Args is the argument list, without the global flags gitx adds.
	Args []string
	// Stderr is what git wrote, trimmed.
	Stderr string
	// ExitCode is git's exit status, or -1 when it never ran or was killed.
	ExitCode int
	// Err is the underlying failure: an *exec.ExitError, a context deadline, or
	// ErrUnknownRevision.
	Err error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("git %s: %v", strings.Join(e.Args, " "), e.Err)
	if e.Stderr != "" {
		msg += ": " + e.Stderr
	}

	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// invocation is one git command: what to run, what to feed it, and where its
// output goes.
type invocation struct {
	args []string
	// stdin is fed to git by a goroutine, so that a command which produces
	// output while it is still being written to — cat-file --batch — cannot
	// deadlock against its own pipe.
	stdin io.Reader
	// stdout receives git's output. A nil stdout captures it.
	stdout io.Writer
	// interactive allows git to prompt on a terminal. Reads never do; a push
	// may, because a human whose credential helper has expired should be asked
	// rather than told the push failed.
	interactive bool
}

// output runs git and returns its standard output with trailing newlines
// removed.
func (g *Git) output(ctx context.Context, args ...string) (string, error) {
	var buf bytes.Buffer

	if err := g.run(ctx, invocation{args: args, stdout: &buf}); err != nil {
		return "", err
	}

	return strings.TrimRight(buf.String(), "\n"), nil
}

// Output runs an arbitrary git command and returns its standard output.
//
// It is the escape hatch for callers that need a command this package has no
// typed wrapper for — the test harness scripting a fixture, mostly. Prefer a
// wrapper: a typed one parses git's output in one place instead of at every
// call site.
func (g *Git) Output(ctx context.Context, args ...string) (string, error) {
	return g.output(ctx, args...)
}

// Run runs an arbitrary git command and discards its output.
func (g *Git) Run(ctx context.Context, args ...string) error {
	return g.run(ctx, invocation{args: args, stdout: io.Discard})
}

// run executes one git process.
func (g *Git) run(ctx context.Context, in invocation) error {
	ctx, cancel := context.WithTimeout(ctx, g.timeout)
	defer cancel()

	// --no-pager first, so a git configured with a pager cannot block on one
	// even if everything after it is wrong. core.quotePath off keeps a path
	// with a non-ASCII byte in it a path rather than an escape sequence.
	args := append([]string{
		"--no-pager",
		"-c", "core.quotePath=false",
		"-C", g.dir,
	}, in.args...)

	cmd := exec.CommandContext(ctx, g.binary, args...) //nolint:gosec // the point of this package
	cmd.Env = g.environ(in.interactive)
	cmd.Stdin = in.stdin
	cmd.Stdout = in.stdout

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	g.processes.Add(1)

	err := cmd.Run()
	if err == nil {
		return nil
	}

	return g.fail(ctx, in.args, stderr.String(), err)
}

// fail turns a failed invocation into an *Error, classifying the two failures
// callers have to tell apart from the rest: a deadline, and a revision git
// could not resolve.
func (g *Git) fail(ctx context.Context, args []string, stderr string, err error) error {
	e := &Error{
		Args:     args,
		Stderr:   strings.TrimSpace(stderr),
		ExitCode: -1,
		Err:      err,
	}

	var exit *exec.ExitError
	if errors.As(err, &exit) {
		e.ExitCode = exit.ExitCode()
	}

	if cause := ctx.Err(); cause != nil {
		e.Err = fmt.Errorf("git did not finish within %s: %w", g.timeout, cause)

		return e
	}

	if isUnknownRevision(e.Stderr) {
		e.Err = fmt.Errorf("%w: %w", ErrUnknownRevision, err)
	}

	return e
}

// unknownRevisionPhrases are how git says a revision does not resolve. It has
// several spellings depending on the command and none of them is a distinct
// exit status, so the message is what there is to read. Matching is
// case-insensitive because git capitalises the same sentence differently from
// one command to the next — `rev-parse --verify` says "Needed a single
// revision" and `log` says "unknown revision".
var unknownRevisionPhrases = []string{
	"unknown revision",
	"bad revision",
	"not a valid object name",
	"ambiguous argument",
	"does not have any commits yet",
	"bad default revision",
	"needed a single revision",
}

func isUnknownRevision(stderr string) bool {
	lower := strings.ToLower(stderr)
	for _, phrase := range unknownRevisionPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	return false
}

// environ is the environment git runs under: the caller's, minus the variables
// that would redirect git at another repository, plus the handful that keep its
// output parseable.
func (g *Git) environ(interactive bool) []string {
	ambient := os.Environ()

	env := make([]string, 0, len(ambient)+len(environmentVars)+1)
	for _, entry := range ambient {
		name, _, _ := strings.Cut(entry, "=")
		if contains(repositoryVars, name) {
			continue
		}

		env = append(env, entry)
	}

	env = append(env, environmentVars...)
	if !interactive {
		env = append(env, "GIT_TERMINAL_PROMPT=0")
	}

	if g.env != nil {
		env = g.env(env)
	}

	return env
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}

	return false
}
