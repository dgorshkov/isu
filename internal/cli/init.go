package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/gitx"
)

// The data model left one question open and asked for it to be answered before
// M4-S1, "which is where the command surface stops being cheap to change":
// every story reads .isu.yml and no story writes one, so adopting isu meant
// hand-writing a file before any command worked at all. The three answers it
// offered were a small `isu init` story in M4, a copy-and-paste block in M8's
// docs, and an entry on the out-of-scope list.
//
// This is the first. It is one command with one required flag, and it is the
// only one of the three that makes the first thing a person types after
// installing isu do something. The other two both leave a tool whose very first
// interaction is an error message about a file they have not heard of.
//
// M5-S6 gave it the other two files a repository needs before the rules in this
// milestone are enforced anywhere: the pre-commit hook and the pipeline. Both
// are written by the same three rules — write what is missing, leave what is
// already right, and refuse to overwrite what is neither without --force — so
// running this twice changes nothing, which is what makes it safe to put in a
// project's own setup script.

type initOptions struct {
	prefix  string
	hooks   bool
	actions bool
	force   bool
}

func (a *app) initCmd() *cobra.Command {
	var opts initOptions

	cmd := &cobra.Command{
		Use:   "init",
		Short: "write .isu.yml, and the hook and pipeline that enforce it",
		Long: `init writes .isu.yml at the top of the repository, and with --hooks or
--actions the two files that make isu check run without anybody remembering to.

The prefix is the first half of every issue id — ISU-7f3akq — and it is part of
every issue's folder name, so it has to be a legal one. There is no default: the
half of an id that says which project this is has no sensible guess. Nothing
else in .isu.yml is written, because a configuration file full of the defaults
is a file nobody can tell they have changed.

Running init twice changes nothing. A file that is already what isu would write
is left alone, and a file that is something else is refused rather than
overwritten — pass --force when replacing it is what you meant.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.initRepo(cmd, &opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.prefix, "prefix", "", "the issue id prefix, such as ISU (required)")
	f.BoolVar(&opts.hooks, "hooks", false, "write a pre-commit hook that runs isu check")
	f.BoolVar(&opts.actions, "actions", false,
		"write .github/workflows/isu.yml, so the rules run on every pull request")
	f.BoolVar(&opts.force, "force", false, "overwrite a file that is not what isu writes")

	return cmd
}

const configTemplate = `# Configuration for isu. See https://github.com/dgorshkov/isu.
#
# prefix is the first half of every issue id, and part of every issue's folder
# name. It is permanent: ids are never rewritten.
prefix: %s
`

// preCommitHook runs the tree rules and only those.
//
// The branch rules cannot be asked here and that is not a shortcut. At
// pre-commit time the change being committed is not a commit yet, so they would
// be answered from the commits already on the branch — and on a claiming branch
// that answer is "you resolved an issue and wrote no code", which would block
// the very commit that writes the code. The pipeline is where the rest runs,
// against a branch that is finished being written.
const preCommitHook = `#!/bin/sh
# Written by ` + "`isu init --hooks`" + `.
#
# Issues are folders in this repository, so a commit can break one. This runs
# the checks that are true of the repository as it stands — the schema, the
# links between issues, the cycles, the epics and the attachment sizes — over
# the working tree, which is what is about to be committed.
#
# It deliberately does not run the checks about what a branch proposes. The
# change being committed is not a commit yet, so those would be answered from
# the commits already on the branch — and on a claiming branch that answer is
# "you resolved an issue and wrote no code", which would block the very commit
# that writes the code. The pipeline runs those, on a finished branch.
#
# Delete this file to stop it, or run git commit --no-verify once.

if ! command -v isu >/dev/null 2>&1; then
	echo "isu: not on PATH, so the issue checks did not run" >&2
	exit 0
fi

exec isu check --worktree
`

// workflow is the pipeline, and the branch rules are the reason it takes some
// care with refs.
//
// A pull request build checks out a merge commit and no branch, and a shallow
// clone has no merge base to measure a branch from — so the base branch is
// fetched by name and named as trunk. Without that, isu compares the repository
// against itself and every branch rule is a no-op that passes.
const workflow = `# Written by ` + "`isu init --actions`" + `.
#
# isu check reads what a branch proposes against trunk, so this fetches the base
# branch and names it: with a shallow clone and no base ref, every rule about
# the branch has nothing to compare and passes silently.
name: isu

on:
  pull_request:
  push:
    branches: [main, master]

permissions:
  contents: read

jobs:
  check:
    name: isu check
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: stable

      - run: go install github.com/dgorshkov/isu/cmd/isu@latest

      - name: isu check
        env:
          BASE: ${{ github.base_ref }}
        run: |
          if [ -n "$BASE" ]; then
            git fetch --quiet origin "+refs/heads/$BASE:refs/remotes/origin/$BASE"
            isu check --ref "origin/$BASE"
          else
            isu check
          fi
`

// plan is one file init is being asked to write.
type plan struct {
	// path is where it goes, absolute.
	path string
	// name is what to call it, relative to the repository root.
	name string
	body []byte
	mode os.FileMode
	// refusal is what to say when something else is already there. Each file
	// has its own, because the reasons are not the same: a workflow somebody
	// edited is theirs, and a prefix somebody changed is every id in the
	// repository.
	refusal string
}

func (a *app) initRepo(cmd *cobra.Command, opts *initOptions) error {
	ctx := cmd.Context()

	start := a.repoPath
	if start == "" {
		start = a.env.dir()
	}

	// This is the one command that runs before there is a configuration to
	// read, so it finds the repository itself rather than going through open.
	root, err := toplevel(ctx, start)
	if err != nil {
		return err
	}

	wanted, err := plans(ctx, cmd, root, opts)
	if err != nil {
		return err
	}

	var wrote []string

	for _, p := range wanted {
		written, err := write(p, opts.force)
		if err != nil {
			return err
		}

		if written {
			wrote = append(wrote, p.name)
		}
	}

	return a.reportWrite(Write{Paths: wrote})
}

// plans is the files this invocation is asking for.
func plans(
	ctx context.Context, cmd *cobra.Command, root string, opts *initOptions,
) ([]plan, error) {
	var wanted []plan

	settings, err := configPlan(cmd, root, opts.prefix)
	if err != nil {
		return nil, err
	}

	if settings != nil {
		wanted = append(wanted, *settings)
	}

	if opts.hooks {
		hook, err := hookPlan(ctx, root)
		if err != nil {
			return nil, err
		}

		wanted = append(wanted, hook)
	}

	if opts.actions {
		wanted = append(wanted, plan{
			path:    filepath.Join(root, ".github", "workflows", "isu.yml"),
			name:    ".github/workflows/isu.yml",
			body:    []byte(workflow),
			mode:    0o644,
			refusal: "a workflow is somebody's, and isu will not rewrite it",
		})
	}

	return wanted, nil
}

// configPlan is .isu.yml, or nothing when the repository already has one and
// this invocation is here for the other files.
func configPlan(cmd *cobra.Command, root, prefix string) (*plan, error) {
	path := filepath.Join(root, config.FileName)

	if prefix == "" {
		if _, err := os.Stat(path); err == nil {
			return nil, nil
		}

		return nil, usagef(cmd, "init needs --prefix: it is the half of an issue id "+
			"that says which project this is, and there is no guessing it")
	}

	contents := fmt.Sprintf(configTemplate, prefix)

	// Validating by parsing what is about to be written is the only way to be
	// sure the file this produces is one isu will accept: a second opinion
	// about what a legal prefix is would be a second definition of it.
	if _, err := config.Parse([]byte(contents)); err != nil {
		return nil, err
	}

	return &plan{
		path: path,
		name: config.FileName,
		body: []byte(contents),
		mode: 0o644,
		refusal: "the prefix in it is part of every id already written, and ids are " +
			"never rewritten",
	}, nil
}

// hookPlan is the pre-commit hook, wherever this repository keeps its hooks.
func hookPlan(ctx context.Context, root string) (plan, error) {
	dir, err := hooksDir(ctx, root)
	if err != nil {
		return plan{}, err
	}

	path := filepath.Join(dir, "pre-commit")

	name, err := filepath.Rel(root, path)
	if err != nil {
		// A hooks directory outside the working tree, which core.hooksPath
		// allows. It has no name relative to the root, so it is called what it
		// is.
		name = path
	}

	return plan{
		path:    path,
		name:    filepath.ToSlash(name),
		body:    []byte(preCommitHook),
		mode:    0o755,
		refusal: "a hook is somebody's, and isu will not rewrite it",
	}, nil
}

// hooksDir is where git looks for hooks: core.hooksPath when it is set, and the
// repository's own hooks directory otherwise.
//
// Both are asked for rather than assumed. `.git` is a directory in a clone, a
// file in a submodule and a file in a linked worktree, and a hook written into
// the wrong one of those is a hook that silently never runs.
func hooksDir(ctx context.Context, root string) (string, error) {
	g, err := gitx.New(root)
	if err != nil {
		return "", err
	}

	custom, err := g.Config(ctx, "core.hooksPath")
	if err != nil {
		return "", err
	}

	if custom != "" {
		if filepath.IsAbs(custom) {
			return custom, nil
		}

		return filepath.Join(root, custom), nil
	}

	dir, err := g.GitDir(ctx)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, "hooks"), nil
}

// write puts one file where it goes, and reports whether it had to.
//
// The three rules are the whole of this command's safety: write what is
// missing, leave what is already right, and refuse to overwrite what is neither
// unless somebody said --force. The middle one is what makes running init twice
// a no-op rather than a diff.
func write(p plan, force bool) (bool, error) {
	switch existing, err := os.ReadFile(p.path); {
	case err == nil && bytes.Equal(existing, p.body):
		return false, nil
	case err == nil && !force:
		return false, fmt.Errorf(
			"%s already has a %s and it is not what isu writes: %s — pass --force to "+
				"replace it", filepath.Dir(p.path), filepath.Base(p.path), p.refusal)
	case err != nil && !os.IsNotExist(err):
		return false, fmt.Errorf("reading %s: %w", p.name, err)
	}

	if err := os.MkdirAll(filepath.Dir(p.path), 0o755); err != nil {
		return false, fmt.Errorf("making %s: %w", filepath.Dir(p.name), err)
	}

	if err := os.WriteFile(p.path, p.body, p.mode); err != nil {
		return false, fmt.Errorf("writing %s: %w", p.name, err)
	}

	// WriteFile leaves the mode of a file that was already there, and a hook
	// that is not executable is a hook git ignores without saying so.
	if err := os.Chmod(p.path, p.mode); err != nil {
		return false, fmt.Errorf("making %s executable: %w", p.name, err)
	}

	return true, nil
}
