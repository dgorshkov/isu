package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

func (a *app) boardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "board",
		Short: "every issue, grouped by derived status",
		Long: `board derives a status for every issue from what trunk and every branch say
about it, and groups them.

Nothing here is stored. An issue file carries a state, which is one ref's claim
about one issue; a status is what the repository as a whole says.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.board(cmd)
		},
	}
}

func (a *app) board(cmd *cobra.Command) error {
	s, err := a.open(cmd.Context())
	if err != nil {
		return err
	}

	v, err := s.view(cmd.Context())
	if err != nil {
		return err
	}

	payload := BoardPayload{
		Trunk:     v.trunkName,
		Refs:      v.refNames(),
		Freshness: v.freshness,
		Groups:    v.groups(),
	}

	if a.asJSON {
		return emit(a.env.Stdout, payload)
	}

	a.renderBoard(s, payload)

	return nil
}

func (a *app) renderBoard(s *session, payload BoardPayload) {
	out := a.env.Stdout
	t := a.themeFor(out)

	total := 0
	for _, group := range payload.Groups {
		total += len(group.Issues)
	}

	fresh := freshnessLine(payload.Freshness, s.cfg.FetchWarnAfter())

	_, _ = fmt.Fprintf(out, "%s · %s · %s\n",
		t.bold(payload.Trunk), plural(total, "issue"), t.dim(fresh))

	if total == 0 {
		_, _ = fmt.Fprintln(out, "\nnothing to show: no issues on any ref isu can see")

		return
	}

	for _, group := range payload.Groups {
		_, _ = fmt.Fprintf(out, "\n%s (%d)\n", t.bold(group.Status), len(group.Issues))

		rows := make([][]string, 0, len(group.Issues))
		for _, item := range group.Issues {
			rows = append(rows, boardRow(item))
		}

		for _, line := range columns(rows, "  ") {
			_, _ = fmt.Fprintln(out, line)
		}
	}
}

// boardRow is one issue as a line of the board: what it is, what it is called,
// and everything a person scanning the list needs to not have to open it.
func boardRow(item Issue) []string {
	title := item.Title
	if title == "" {
		title = "(no title)"
	}

	return []string{item.ID, item.Priority, item.Type, title, boardNote(item)}
}

// boardNote is the trailing column: who owns it, who is on it, and the
// annotations that survive the status precedence.
func boardNote(item Issue) string {
	var parts []string

	if item.Owner != "" {
		parts = append(parts, item.Owner)
	}

	if claimed := claimNote(item); claimed != "" {
		parts = append(parts, claimed)
	}

	if item.Reopened {
		parts = append(parts, "reopened")
	}

	if item.Contended {
		parts = append(parts, "contended")
	}

	if item.Stale {
		parts = append(parts, "stale")
	}

	if item.Broken != nil {
		parts = append(parts, "unreadable")
	}

	if item.Epic != nil {
		parts = append(parts, plural(len(item.Epic.Children), "child"))

		if item.Epic.Cycle {
			parts = append(parts, "parent cycle")
		}
	}

	return strings.Join(parts, " · ")
}

func claimNote(item Issue) string {
	if len(item.Claims) == 0 {
		return ""
	}

	names := make([]string, 0, len(item.Claims))

	for _, claim := range item.Claims {
		who := claim.Claimant
		if who == "" {
			// A claim whose first commit was not looked up is still a claim:
			// what the file says is the claim, and the lookup only names who
			// made it.
			who = "someone"
		}

		names = append(names, who)
	}

	return "claimed by " + strings.Join(names, ", ")
}

// plural counts things in a sentence rather than in a log line. "child" is the
// one word here whose plural is not the singular plus an s.
func plural(n int, thing string) string {
	word := thing
	if n != 1 {
		if thing == "child" {
			word = "children"
		} else {
			word = thing + "s"
		}
	}

	return strconv.Itoa(n) + " " + word
}
