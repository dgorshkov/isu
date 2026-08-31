package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
)

func (a *app) resolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <id>",
		Short: "say this branch fixes an issue",
		Long: `resolve writes state: resolved into the issue on the branch you are on, and
an Isu-Resolves trailer on the commit.

It merges nothing. Trunk is where state is true, and the only way to reach done
is a merged pull request — which is the whole point of a tracker whose issues
are files.

The trailer is the first of the three tiers that link a trunk commit back to the
issue it resolved. It is the one thing file content cannot answer: a squash
merge collapses authorship, but it does not touch the file.

Resolving an issue a claim already flipped writes no change to the file at all,
and that is correct: what this adds is the link, and the code beside it. A
branch that resolves an issue and changes nothing outside issues/ is what the
evidence check in M5-S3 exists to reject.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.resolve(cmd, args[0])
		},
	}
}

func (a *app) resolve(cmd *cobra.Command, id string) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	if err := s.mustNotBeTrunk(ctx, cmd); err != nil {
		return err
	}

	target, err := s.readIssueHere(id)
	if err != nil {
		return err
	}

	if target.Type == issue.TypeEpic {
		return fmt.Errorf(
			"%s is an epic, and an epic has no state of its own: it is finished when "+
				"its children are", id)
	}

	if warning := spikeWarning(s, target); warning != "" {
		_, _ = fmt.Fprintf(a.env.Stderr, "isu: %s\n", warning)
	}

	target.State = issue.StateResolved

	message := fmt.Sprintf("resolve %s\n\n%s\n\n%s: %s\n",
		id, target.Title, model.ResolvesTrailer, id)

	if _, err := s.stage(ctx, []change{{path: readmePath(id), blob: target.Encode()}}); err != nil {
		return err
	}

	commit, err := s.git.CommitAllowingEmpty(ctx, message)
	if err != nil {
		return err
	}

	branch, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	return a.reportWrite(Write{
		ID: id, Branch: branch, Commit: commit, Paths: []string{readmePath(id)},
	})
}

// spikeWarning says so when a spike is being resolved with nothing to show for
// it.
//
// Resolving a spike requires a file in the issue's own folder that is not the
// README — the answer, rather than the assertion that there is one. This warns
// rather than refuses: the check that fails a pull request is M5-S3's, and a
// command that guessed wrong about somebody's folder should not be the thing
// standing between them and a commit.
func spikeWarning(s *session, target *issue.Issue) string {
	if target.Type != issue.TypeSpike {
		return ""
	}

	folder, err := issue.Load(issueDirOf(s, target.ID))
	if err != nil || len(folder.Attachments) > 0 {
		return ""
	}

	return fmt.Sprintf(
		"%s is a spike and its folder holds nothing but the README: resolving one "+
			"needs the answer written down beside it, and M5-S3 will say so",
		target.ID)
}

func (a *app) dropCmd() *cobra.Command {
	var reason, resolution string

	cmd := &cobra.Command{
		Use:   "drop <id>",
		Short: "say an issue will not be fixed, and why",
		Long: `drop writes state: dropped into the issue on the branch you are on, with the
reason and the resolution the schema requires.

dropped on its own cannot tell a duplicate from a won't-fix, and the difference
is the first thing anyone asks — so --resolution is required and is one of
wontfix, duplicate, works-as-intended or fixed-elsewhere.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.drop(cmd, args[0], reason, resolution)
		},
	}

	f := cmd.Flags()
	f.StringVar(&reason, "reason", "", "why, in a sentence (required)")
	f.StringVar(&resolution, "resolution", "",
		"wontfix, duplicate, works-as-intended or fixed-elsewhere (required)")

	return cmd
}

func (a *app) drop(cmd *cobra.Command, id, reason, resolution string) error {
	ctx := cmd.Context()

	if reason == "" {
		return usagef(cmd, "drop needs --reason: an issue nobody explained is an issue "+
			"somebody will file again")
	}

	if resolution == "" {
		return usagef(cmd, "drop needs --resolution: one of %s", oneOfResolutions())
	}

	if !issue.Resolution(resolution).Valid() {
		return usagef(cmd, "%q is not a resolution: one of %s", resolution, oneOfResolutions())
	}

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	if err := s.mustNotBeTrunk(ctx, cmd); err != nil {
		return err
	}

	target, err := s.readIssueHere(id)
	if err != nil {
		return err
	}

	if target.Type == issue.TypeEpic {
		return fmt.Errorf(
			"%s is an epic, and an epic has no state of its own: it is dropped when "+
				"its children are", id)
	}

	target.State = issue.StateDropped
	target.Reason = reason
	target.Resolution = issue.Resolution(resolution)

	message := fmt.Sprintf("drop %s\n\n%s\n\n%s: %s\n",
		id, reason, model.ResolvesTrailer, id)

	commit, err := s.commitHere(ctx, message, []change{
		{path: readmePath(id), blob: target.Encode()},
	})
	if err != nil {
		return err
	}

	branch, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return err
	}

	return a.reportWrite(Write{
		ID: id, Branch: branch, Commit: commit, Paths: []string{readmePath(id)},
	})
}

func oneOfResolutions() string {
	names := make([]string, 0, len(issue.Resolutions))
	for _, r := range issue.Resolutions {
		names = append(names, string(r))
	}

	return join(names)
}
