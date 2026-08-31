package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
)

type triageOptions struct {
	owner     string
	parent    string
	blockedBy []string
	priority  string
	push      bool
}

func (a *app) triageCmd() *cobra.Command {
	var opts triageOptions

	cmd := &cobra.Command{
		Use:   "triage <id>",
		Short: "set who owns an issue, what blocks it, and how urgent it is",
		Long: `triage sets everything about an issue other than its state: its owner, the
epic it belongs to, what it waits on, and its priority.

It writes a triage/<id> branch for a pull request, like everything else here.
--push writes straight to trunk instead, and is refused unless .isu.yml sets
direct_triage: true — the field is how a team says out loud that triage is not a
code review.

The body and any keys isu does not know survive byte for byte. This rewrites the
lines it was asked to and nothing else.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.triage(cmd, args[0], &opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.owner, "owner", "", "the human answerable for it")
	f.StringVar(&opts.parent, "parent", "", "the epic it belongs to")
	f.StringSliceVar(&opts.blockedBy, "blocked-by", nil,
		"the issues it waits on, replacing what is there")
	f.StringVar(&opts.priority, "priority", "", "p0, p1, p2 or p3")
	f.BoolVar(&opts.push, "push", false, "commit straight to trunk instead of onto a branch")

	return cmd
}

func (a *app) triage(cmd *cobra.Command, id string, opts *triageOptions) error {
	ctx := cmd.Context()

	if opts.owner == "" && opts.parent == "" && opts.priority == "" && opts.blockedBy == nil {
		return usagef(cmd, "triage needs something to set: --owner, --parent, "+
			"--blocked-by or --priority")
	}

	if opts.priority != "" && !issue.Priority(opts.priority).Valid() {
		return usagef(cmd, "%q is not a priority: one of %s",
			opts.priority, join(priorityNames()))
	}

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	if opts.push && !s.cfg.DirectTriage {
		return usagef(cmd,
			"--push writes straight to trunk, and %s does not set `%s: true`: "+
				"that field is how a repository says out loud that triage is not a "+
				"code review",
			config.FileName, config.KeyDirectTriage)
	}

	v, err := s.view(ctx)
	if err != nil {
		return err
	}

	item, ok := v.board.Get(id)
	if !ok {
		return fmt.Errorf("no issue %s on %s or any branch beside it", id, v.trunkName)
	}

	if wrong := checkParent(cmd, v.board, opts.parent); wrong != nil {
		return wrong
	}

	branch, base, err := s.triageTarget(ctx, id, opts.push)
	if err != nil {
		return err
	}

	// The issue is read from wherever it is and written where it is going, and
	// those are not the same ref for the case triage exists for. An untriaged
	// report is a folder on a branch that trunk has never seen — that is what
	// `awaiting triage` means — so reading it at trunk would make the command
	// fail on precisely the issues it was built to answer.
	target, err := s.readIssueAt(ctx, source(item, base), id)
	if err != nil {
		return err
	}

	applyTriage(target, opts)

	if invalid := target.Validate(); invalid != nil {
		return invalid
	}

	commit, err := s.commitOn(ctx, commitSpec{
		branch:  branch,
		base:    base,
		message: fmt.Sprintf("triage %s\n\n%s\n", id, triageSummary(opts)),
		changes: []change{{path: readmePath(id), blob: target.Encode()}},
	})
	if err != nil {
		return err
	}

	pushed := false

	if opts.push {
		if pushed, err = s.pushIfRemote(ctx, branch); err != nil {
			return err
		}
	}

	return a.reportWrite(Write{
		ID: id, Branch: branch, Commit: commit,
		Paths: []string{readmePath(id)}, Pushed: pushed,
	})
}

// source is the ref an issue's current file should be read from: trunk where
// trunk has it, and otherwise the first branch that does.
//
// An issue that is only on a branch is the ordinary case for triage rather than
// an edge of it: `awaiting triage` is defined as a folder trunk has never seen.
func source(item *model.Item, base string) string {
	if item.OnTrunk || len(item.Elsewhere) == 0 {
		return base
	}

	return item.Elsewhere[0]
}

// triageTarget is which branch this edit lands on and where it starts.
//
// --push lands on trunk itself. Everything else lands on triage/<id>, which is
// deliberately outside the isu/ namespace: a triage edit does not flip the
// state, so it is not a claim, and the board says nothing about it until it
// merges.
func (s *session) triageTarget(ctx context.Context, id string, push bool) (string, string, error) {
	if !push {
		return triageBranchPrefix + id, s.trunk, nil
	}

	branch, err := s.trunkBranch(ctx)
	if err != nil {
		return "", "", err
	}

	if branch == "" {
		return "", "", fmt.Errorf(
			"--push writes to trunk, and %s is not a local branch this repository "+
				"can move", s.trunk)
	}

	return branch, s.trunk, nil
}

// pushIfRemote sends trunk on, and says whether there was anywhere to send it.
//
// A repository with no remote is not a failure here, unlike a claim: what
// --push means is "write to trunk rather than open a pull request", and trunk
// has been written whether or not anybody else can see it yet.
func (s *session) pushIfRemote(ctx context.Context, branch string) (bool, error) {
	remotes, err := s.git.Remotes(ctx)
	if err != nil {
		return false, err
	}

	if len(remotes) == 0 {
		return false, nil
	}

	return true, s.push(ctx, branch)
}

func applyTriage(target *issue.Issue, opts *triageOptions) {
	if opts.owner != "" {
		target.Owner = opts.owner
	}

	if opts.parent != "" {
		target.Parent = opts.parent
	}

	if opts.priority != "" {
		target.Priority = issue.Priority(opts.priority)
	}

	if opts.blockedBy != nil {
		target.BlockedBy = opts.blockedBy
	}
}

func triageSummary(opts *triageOptions) string {
	var parts []string

	if opts.owner != "" {
		parts = append(parts, issue.KeyOwner+": "+opts.owner)
	}

	if opts.parent != "" {
		parts = append(parts, issue.KeyParent+": "+opts.parent)
	}

	if opts.priority != "" {
		parts = append(parts, issue.KeyPriority+": "+opts.priority)
	}

	if opts.blockedBy != nil {
		parts = append(parts, issue.KeyBlockedBy+": "+strings.Join(opts.blockedBy, ", "))
	}

	return strings.Join(parts, "\n")
}

func priorityNames() []string {
	names := make([]string, 0, len(issue.Priorities))
	for _, p := range issue.Priorities {
		names = append(names, string(p))
	}

	return names
}
