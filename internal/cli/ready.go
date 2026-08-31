package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/model"
)

func (a *app) readyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ready",
		Short: "the issues nothing is blocking, most urgent first",
		Long: `ready lists the open issues whose blockers are all finished, ordered by
priority and then by age.

An epic blocker counts as finished when its rollup does, since an epic has no
state of its own. Anything somebody has claimed is not ready: the board is not
going to hand two people the same work.

It prints JSON unless told otherwise, because the caller is usually an agent.
Pass --json=false for the list a person reads.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.ready(cmd)
		},
	}

	return cmd
}

func (a *app) ready(cmd *cobra.Command) error {
	// The one command whose default output is JSON. `isu ready --json | head -1`
	// is meant to be the top of the queue, and an agent should not have to pass
	// a flag to be spoken to in the format it reads.
	if !cmd.Flags().Changed("json") {
		a.asJSON = true
	}

	s, err := a.open(cmd.Context())
	if err != nil {
		return err
	}

	v, err := s.view(cmd.Context())
	if err != nil {
		return err
	}

	items := readyItems(v)

	if a.asJSON {
		// One issue per line, so that `head -1` is an issue and not a brace.
		for _, item := range items {
			if err := emit(a.env.Stdout, asIssue(item, v.now)); err != nil {
				return err
			}
		}

		return nil
	}

	a.renderReady(v, items)

	return nil
}

// readyItems is the whole of what `ready` means: on trunk, open, nobody on it,
// and nothing it waits for still running.
func readyItems(v *view) []*model.Item {
	var ready []*model.Item

	for _, id := range v.board.IDs() {
		item, _ := v.board.Get(id)

		if !workable(item) || !unblocked(v, item) {
			continue
		}

		ready = append(ready, item)
	}

	sortItems(ready)

	return ready
}

// workable is an issue somebody could start now.
//
// `open` and `reopened` are both trunk saying open with no branch claiming it;
// a reopen is an issue that needs doing again, which is exactly the list this
// is. Everything else is excluded by its status: done and dropped are terminal,
// in progress means somebody is on it, and awaiting triage is a report nobody
// has accepted yet — handing an agent work out of the inbox would be handing it
// work nobody agreed to.
//
// An epic is excluded whatever its rollup says. It has no state of its own and
// is finished when its children are, so it is never a thing to pick up: an
// agent handed one would have nothing to do and no way to say it was done.
//
// So is an issue trunk cannot decode. Handing somebody an issue nobody can read
// is handing them a parse error.
func workable(item *model.Item) bool {
	if item.Broken != nil || item.Epic != nil {
		return false
	}

	return item.Status == model.StatusOpen || item.Status == model.StatusReopened
}

// unblocked reports whether everything this issue waits on has finished.
//
// An epic blocker is finished when its rollup is, which needs no special case
// here: an epic's derived status is that rollup, and Terminal reads it the same
// way it reads anything else.
//
// A blocker naming an issue this repository does not have counts as blocking.
// It is M5-S2's to report, and until somebody does, "I am waiting on something
// nobody can find" is not a reason to hand the work out.
func unblocked(v *view, item *model.Item) bool {
	if item.Issue == nil {
		return false
	}

	for _, id := range item.Issue.BlockedBy {
		blocker, ok := v.board.Get(id)
		if !ok || !blocker.Status.Terminal() {
			return false
		}
	}

	return true
}

func (a *app) renderReady(v *view, items []*model.Item) {
	out := a.env.Stdout
	t := a.themeFor(out)

	if len(items) == 0 {
		_, _ = fmt.Fprintln(out, "nothing is ready: everything open is blocked, claimed or untriaged")

		return
	}

	_, _ = fmt.Fprintf(out, "%s\n", t.bold(plural(len(items), "issue")+" ready"))

	rows := make([][]string, 0, len(items))
	for _, item := range items {
		rows = append(rows, boardRow(asIssue(item, v.now)))
	}

	for _, line := range columns(rows, "  ") {
		_, _ = fmt.Fprintln(out, line)
	}
}
