// Package gittest builds real git repositories from tests.
//
// Almost every question this project answers — is an issue resolved, who is
// working on it, was it reopened — is a question about history, and the only
// honest way to test an answer about history is to script the history first.
// So tests do not mock git: they build a repository in t.TempDir() with this
// builder and let the code under test read it exactly as it would read a
// developer's clone.
//
// Every method fails the test instead of returning an error, so that scripting
// a fixture reads as one chain of statements rather than a ladder of error
// checks:
//
//	r := gittest.New(t).
//		Issue("AR-7f3akq").Commit("add AR-7f3akq").
//		Branch("isu/AR-7f3akq").Checkout("isu/AR-7f3akq").
//		Issue("AR-7f3akq", gittest.State("resolved")).Commit("resolve AR-7f3akq").
//		Checkout(gittest.DefaultBranch).
//		SquashMerge("isu/AR-7f3akq", "resolve AR-7f3akq")
//
// Repositories live under t.TempDir() and are removed with it. The harness
// runs git with the ambient configuration switched off, so a global template
// directory, a signing key or a different init.defaultBranch on one machine
// cannot change what a test sees.
//
// Try is the one method that hands back an error, for the tests whose subject
// is a git command that must fail.
package gittest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dgorshkov/isu/internal/gitx"
)

// DefaultBranch is the trunk of every repository this harness builds. It is
// fixed here rather than inherited from init.defaultBranch, because a fixture
// whose trunk is named by the developer's git configuration is a fixture that
// passes on one machine and not the next.
const DefaultBranch = "main"

// detachedSuffix is appended to a remote's directory to take it away. The
// remote stays configured in the repository and simply stops answering, which
// is what an unreachable network looks like from inside git.
const detachedSuffix = ".detached"

// localConfig is appended to the repository's own config after init. Writing
// the file beats seven `git config` processes per repository, and every test
// in this project builds at least one.
const localConfig = `
[user]
	name = isu tester
	email = tester@example.invalid
[commit]
	gpgsign = false
[tag]
	gpgsign = false
[core]
	autocrlf = false
[gc]
	auto = 0
[advice]
	detachedHead = false
`

// Repo is a git repository scripted by a test.
type Repo struct {
	t   testing.TB
	dir string
	// git is the one door to the binary. The harness goes through internal/gitx
	// like everything else does, so that M2-S1's rule — nothing outside that
	// package builds a git command — is a rule and not an exemption list.
	git    *gitx.Git
	remote string
	// offset moves the clock every later commit is stamped with. See Backdate.
	offset time.Duration
}

// New creates an empty repository on DefaultBranch under t.TempDir().
//
// It has no commits: an empty ref is a case the loader in M2 has to handle,
// and a harness that starts with a commit cannot produce one. The first
// Commit is what gives the trunk a tip and lets Branch name it.
func New(t testing.TB) *Repo {
	t.Helper()

	r := &Repo{t: t, dir: t.TempDir()}

	g, err := gitx.New(r.dir, gitx.WithEnv(r.env))
	if err != nil {
		t.Fatalf("gittest: %v", err)
	}
	r.git = g

	r.Git("init", "--quiet", "-b", DefaultBranch, ".")

	config, err := os.OpenFile(
		filepath.Join(r.dir, ".git", "config"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("gittest: opening the repository config: %v", err)
	}
	if _, err := config.WriteString(localConfig); err != nil {
		_ = config.Close()
		t.Fatalf("gittest: writing the repository config: %v", err)
	}
	if err := config.Close(); err != nil {
		t.Fatalf("gittest: closing the repository config: %v", err)
	}

	return r
}

// Dir is the path of the working tree.
func (r *Repo) Dir() string { return r.dir }

// RemoteDir is the path of the bare repository origin points at, or the empty
// string before WithRemote has been called.
func (r *Repo) RemoteDir() string { return r.remote }

// Head is the commit id HEAD resolves to.
func (r *Repo) Head() string { return r.Git("rev-parse", "HEAD") }

// Branches lists the repository's local branches.
func (r *Repo) Branches() []string {
	r.t.Helper()

	out := r.Git("for-each-ref", "--format=%(refname:short)", "refs/heads/")
	if out == "" {
		return nil
	}

	return strings.Split(out, "\n")
}

// Git runs git in the repository and returns its standard output with trailing
// newlines removed. A git that exits non-zero fails the test, and its stderr is
// the failure message.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()

	out, err := r.Try(args...)
	if err != nil {
		r.t.Fatalf("gittest: %v", err)
	}

	return out
}

// Try is Git for the tests whose subject is a git command that must fail —
// pushing a claim that has already been claimed, fetching a detached remote.
// Everything else should use Git and let the failure be the test's failure.
func (r *Repo) Try(args ...string) (string, error) {
	r.t.Helper()

	return r.git.Output(context.Background(), args...)
}

// env is the environment git runs under: no inherited git configuration of any
// kind, no credential prompt, a fixed identity and locale, and the clock
// Backdate set. gitx has already dropped the variables that would redirect git
// at another repository; this drops the rest of the prefix and writes the
// harness's own.
//
// Every GIT_* variable the test process inherited is dropped rather than
// overridden. Overriding needs a list of the dangerous ones, and the list is
// the problem: GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_OBJECT_DIRECTORY,
// GIT_COMMON_DIR and GIT_NAMESPACE all redirect git at a repository other than
// the one the test built, and the next release of git may add another. A test
// run from a git hook, from `git bisect run`, or from any tool that exports
// those inherits them — and what the harness does then is not fail, it is
// quietly init, commit into and read *the developer's own repository*. Dropping
// the whole prefix cannot miss one.
//
// GIT_TRACE* survives because it changes only what git prints, and
// `GIT_TRACE=1 go test ./...` is how anybody debugs this package.
func (r *Repo) env(base []string) []string {
	when := time.Now().Add(-r.offset).Format(time.RFC3339)

	env := make([]string, 0, len(base)+13)
	for _, entry := range base {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "GIT_") && !strings.HasPrefix(name, "GIT_TRACE") {
			continue
		}

		env = append(env, entry)
	}

	return append(env,
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"GIT_AUTHOR_NAME=isu tester",
		"GIT_AUTHOR_EMAIL=tester@example.invalid",
		"GIT_COMMITTER_NAME=isu tester",
		"GIT_COMMITTER_EMAIL=tester@example.invalid",
		"GIT_AUTHOR_DATE="+when,
		"GIT_COMMITTER_DATE="+when,
		"TZ=UTC",
		"LC_ALL=C",
	)
}

// File writes a file, creating parent directories, and stages it. The path is
// slash-separated and relative to the working tree.
func (r *Repo) File(path, content string) *Repo {
	r.t.Helper()

	r.write(path, content)
	r.Git("add", "--", path)

	return r
}

// WriteFile writes a file without staging it.
//
// What is on disk, what is in the index and what is in a commit are three
// different questions. M2-S3 reads the first of them, so it needs a way to put
// a file there and leave it there — a file matched by .gitignore cannot be
// staged at all, and `git add` on one fails rather than adding it.
func (r *Repo) WriteFile(path, content string) *Repo {
	r.t.Helper()

	r.write(path, content)

	return r
}

// ReadFile returns the contents of a file in the working tree.
func (r *Repo) ReadFile(path string) string {
	r.t.Helper()

	body, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(path)))
	if err != nil {
		r.t.Fatalf("gittest: reading %s: %v", path, err)
	}

	return string(body)
}

// write puts content on disk without staging it.
func (r *Repo) write(path, content string) {
	r.t.Helper()

	full := filepath.Join(r.dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		r.t.Fatalf("gittest: creating the directory for %s: %v", path, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
		r.t.Fatalf("gittest: writing %s: %v", path, err)
	}
}

// Commit commits what is staged with msg as its subject. Committing nothing is
// a failure: a fixture that meant to change a file and did not is a bug in the
// test, and an empty commit hides it.
func (r *Repo) Commit(msg string) *Repo {
	r.t.Helper()

	r.Git("commit", "--quiet", "-m", msg)

	return r
}

// Branch creates a branch at HEAD without switching to it. Checkout is what
// moves.
func (r *Repo) Branch(name string) *Repo {
	r.t.Helper()

	r.Git("branch", name)

	return r
}

// DeleteBranch removes a branch whether or not its commits are reachable from
// anywhere else. It is what a forge does on merge, and the state several
// derivations have to be correct in: a claim is released when the work lands,
// so an issue reads done with the branch that claimed it already gone.
func (r *Repo) DeleteBranch(name string) *Repo {
	r.t.Helper()

	r.Git("branch", "--delete", "--force", name)

	return r
}

// Checkout switches to an existing branch, or to any other committish — a tag
// or a raw commit id, which detaches HEAD.
func (r *Repo) Checkout(name string) *Repo {
	r.t.Helper()

	r.Git("checkout", "--quiet", name)

	return r
}

// Merge merges branch into the current branch as a merge commit. The merge is
// never fast-forwarded, so the shape of the history is what the test wrote
// rather than what git was able to simplify it to.
func (r *Repo) Merge(branch string) *Repo {
	r.t.Helper()

	r.Git("merge", "--quiet", "--no-ff", "--no-edit",
		"-m", fmt.Sprintf("Merge branch '%s'", branch), branch)

	return r
}

// SquashMerge lands branch as a single commit with the given subject, leaving
// the branch's own commits unreachable from here. This is GitLab's default and
// a common GitHub setting, so it is the case the derivation layer has to be
// correct under — see the squash-merge safety section of PLAN.md.
func (r *Repo) SquashMerge(branch, subject string) *Repo {
	r.t.Helper()

	r.Git("merge", "--quiet", "--squash", branch)
	r.Git("commit", "--quiet", "-m", subject)

	return r
}

// Revert commits the inverse of ref. It is how a test writes the history that
// makes an issue `reopened`: resolved at one trunk commit, open at a later one,
// with both facts still in the log.
func (r *Repo) Revert(ref string) *Repo {
	r.t.Helper()

	r.Git("revert", "--no-edit", ref)

	return r
}

// WithRemote gives the repository an origin — a bare repository in a temporary
// directory — and pushes the current branch to it with an upstream set.
func (r *Repo) WithRemote() *Repo {
	r.t.Helper()

	if r.remote != "" {
		r.t.Fatal("gittest: the repository already has a remote")
	}

	r.remote = filepath.Join(r.t.TempDir(), "origin.git")
	r.Git("init", "--quiet", "--bare", r.remote)
	r.Git("remote", "add", "origin", r.remote)
	r.Git("push", "--quiet", "--set-upstream", "origin",
		r.Git("branch", "--show-current"))

	return r
}

// DetachRemote takes the remote away without taking the configuration away:
// origin is still named, still tracked, and no longer answers. That is what an
// unreachable network looks like from inside git, and it is the state M9-S3
// runs a whole session in. Removing the remote instead would model a
// repository that never had one, which is a different test.
func (r *Repo) DetachRemote() *Repo {
	r.t.Helper()

	if r.remote == "" {
		r.t.Fatal("gittest: DetachRemote before WithRemote")
	}
	if err := os.Rename(r.remote, r.remote+detachedSuffix); err != nil {
		r.t.Fatalf("gittest: detaching the remote: %v", err)
	}

	return r
}

// Backdate stamps every later commit days in the past, so that a test can age
// a claim past stale_days or write history spread over weeks without waiting
// for any of it. Backdate(0) returns to the real clock.
func (r *Repo) Backdate(days int) *Repo {
	r.t.Helper()

	r.offset = time.Duration(days) * 24 * time.Hour

	return r
}
