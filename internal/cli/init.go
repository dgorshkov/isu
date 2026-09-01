package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/config"
)

// PLAN.md §1 left one question open and asked for it to be answered before
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

func (a *app) initCmd() *cobra.Command {
	var prefix string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "write .isu.yml, so that everything else works",
		Long: `init writes .isu.yml at the top of the repository.

The prefix is the first half of every issue id — ISU-7f3akq — and it is part of
every issue's folder name, so it has to be a legal one. There is no default: the
half of an id that says which project this is has no sensible guess.

Nothing else in .isu.yml is written. Every other key has a default, and a
configuration file full of the defaults is a file nobody can tell they have
changed.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.initRepo(cmd, prefix)
		},
	}

	cmd.Flags().StringVar(&prefix, "prefix", "", "the issue id prefix, such as ISU (required)")

	return cmd
}

const configTemplate = `# Configuration for isu. See https://github.com/dgorshkov/isu.
#
# prefix is the first half of every issue id, and part of every issue's folder
# name. It is permanent: ids are never rewritten.
prefix: %s
`

func (a *app) initRepo(cmd *cobra.Command, prefix string) error {
	ctx := cmd.Context()

	if prefix == "" {
		return usagef(cmd, "init needs --prefix: it is the half of an issue id that "+
			"says which project this is, and there is no guessing it")
	}

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

	contents := fmt.Sprintf(configTemplate, prefix)

	// Validating by parsing what is about to be written is the only way to be
	// sure the file this produces is one isu will accept: a second opinion
	// about what a legal prefix is would be a second definition of it.
	if _, err := config.Parse([]byte(contents)); err != nil {
		return err
	}

	path := filepath.Join(root, config.FileName)
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf(
			"%s already has a %s: isu will not overwrite it, because the prefix in "+
				"it is part of every id already written", root, config.FileName)
	}

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil { //nolint:gosec // configuration is not a secret
		return fmt.Errorf("writing %s: %w", config.FileName, err)
	}

	return a.reportWrite(Write{Paths: []string{config.FileName}})
}
