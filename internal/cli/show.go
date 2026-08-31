package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
)

func (a *app) showCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "one issue in full",
		Long: `show prints one issue: its fields, its derived status, the branches that
differ from trunk on it, who has claimed it, where it sits in its epic, and the
attachments and comments that live in its folder.

The body is printed verbatim. v1.0.0 does not render markdown — glamour arrives
with the TUI.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.show(cmd, args[0])
		},
	}
}

func (a *app) show(cmd *cobra.Command, id string) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	v, err := s.view(ctx)
	if err != nil {
		return err
	}

	item, ok := v.board.Get(id)
	if !ok {
		return fmt.Errorf("no issue %s on %s or any branch beside it", id, v.trunkName)
	}

	payload, err := s.showPayload(ctx, v, item)
	if err != nil {
		return err
	}

	if a.asJSON {
		return emit(a.env.Stdout, payload)
	}

	a.renderShow(s, v, payload)

	return nil
}

func (s *session) showPayload(
	ctx context.Context, v *view, item *model.Item,
) (ShowPayload, error) {
	payload := ShowPayload{
		Issue:       asIssue(item, v.now),
		Attachments: []string{},
		Comments:    []Text{},
		Children:    []Issue{},
		Blockers:    []Link{},
		Freshness:   v.freshness,
	}

	if item.Issue != nil {
		payload.Body = item.Issue.Body

		if item.Issue.Parent != "" {
			parent := asLink(v.board, item.Issue.Parent)
			payload.Parent = &parent
		}

		for _, id := range item.Issue.BlockedBy {
			payload.Blockers = append(payload.Blockers, asLink(v.board, id))
		}
	}

	if item.Epic != nil {
		children := make([]*model.Item, 0, len(item.Epic.Children))

		for _, id := range item.Epic.Children {
			if child, ok := v.board.Get(id); ok {
				children = append(children, child)
			}
		}

		sortItems(children)

		for _, child := range children {
			payload.Children = append(payload.Children, asIssue(child, v.now))
		}
	}

	attachments, comments, err := s.folder(ctx, item)
	if err != nil {
		return payload, err
	}

	payload.Attachments = attachments
	payload.Comments = comments

	return payload, nil
}

// folder reads what lives beside an issue: the attachments, and the comments.
//
// It comes off disk when isu is reading HEAD and the folder is there, and out
// of git otherwise. That is what makes `isu comment` followed by `isu show` do
// the obvious thing — a comment is a file, and it is a file before it is a
// commit — while `--ref` still shows what that ref actually holds.
//
// Neither path is on the read path PLAN.md §0 measures. That path exists
// because five thousand issues cost five thousand lookups; one issue's folder
// costs one listing.
func (s *session) folder(ctx context.Context, item *model.Item) ([]string, []Text, error) {
	dir := filepath.Join(s.root, filepath.FromSlash(issueDir(item.ID)))

	if s.trunk == "HEAD" {
		if _, err := os.Stat(dir); err == nil {
			return loadFolderFromDisk(dir)
		}
	}

	return s.loadFolderFromRef(ctx, item)
}

func loadFolderFromDisk(dir string) ([]string, []Text, error) {
	folder, err := issue.Load(dir)
	if err != nil {
		return nil, nil, err
	}

	comments := make([]Text, 0, len(folder.Comments))
	for _, c := range folder.Comments {
		comments = append(comments, Text{Name: c.Name, Body: c.Body})
	}

	return append([]string{}, folder.Attachments...), comments, nil
}

// loadFolderFromRef reads the folder as a ref holds it, which is the only place
// to read it for an issue reported on a branch nobody has checked out.
func (s *session) loadFolderFromRef(
	ctx context.Context, item *model.Item,
) ([]string, []Text, error) {
	ref := s.trunk
	if !item.OnTrunk && len(item.Elsewhere) > 0 {
		ref = item.Elsewhere[0]
	}

	entries, err := s.git.LsTree(ctx, ref, issueDir(item.ID)+"/")
	if err != nil {
		return nil, nil, err
	}

	attachments := []string{}
	comments := []Text{}
	oids := []string{}
	names := map[string]string{}

	prefix := issueDir(item.ID) + "/"
	commentPrefix := prefix + issue.CommentsDir + "/"

	for _, entry := range entries {
		name := strings.TrimPrefix(entry.Path, prefix)

		switch {
		case name == issue.ReadmeName:
		case strings.HasPrefix(entry.Path, commentPrefix):
			if strings.HasSuffix(name, issue.CommentExt) {
				oids = append(oids, entry.OID)
				names[entry.OID] = strings.TrimPrefix(entry.Path, commentPrefix)
			}
		case !strings.Contains(name, "/"):
			attachments = append(attachments, name)
		}
	}

	err = s.git.CatFileBatch(ctx, oids, func(object gitx.Object) error {
		comments = append(comments, Text{Name: names[object.OID], Body: string(object.Data)})

		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	sort.Strings(attachments)
	sort.Slice(comments, func(i, j int) bool { return comments[i].Name < comments[j].Name })

	return attachments, comments, nil
}

func (a *app) renderShow(s *session, v *view, payload ShowPayload) {
	out := a.env.Stdout
	t := a.themeFor(out)
	item := payload.Issue

	title := item.Title
	if title == "" {
		title = "(no title)"
	}

	_, _ = fmt.Fprintf(out, "%s  %s\n\n", t.bold(item.ID), title)

	for _, line := range columns(showFields(v, payload), "  ") {
		_, _ = fmt.Fprintln(out, line)
	}

	if item.Broken != nil {
		_, _ = fmt.Fprintf(out, "\n  this file will not decode: %s\n", item.Broken.Error)
	}

	for _, claim := range payload.Issue.Claims {
		_, _ = fmt.Fprintf(out, "\n  %s\n", showClaim(claim))
	}

	if len(payload.Children) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s\n", t.bold("children"))

		rows := make([][]string, 0, len(payload.Children))
		for _, child := range payload.Children {
			rows = append(rows, []string{child.ID, child.Status, child.Title})
		}

		for _, line := range columns(rows, "  ") {
			_, _ = fmt.Fprintln(out, line)
		}
	}

	if body := strings.TrimRight(payload.Body, "\n"); body != "" {
		_, _ = fmt.Fprintf(out, "\n%s\n", body)
	}

	if len(payload.Attachments) > 0 {
		_, _ = fmt.Fprintf(out, "\n%s\n", t.bold("attachments"))

		for _, name := range payload.Attachments {
			_, _ = fmt.Fprintf(out, "  %s\n", name)
		}
	}

	for _, comment := range payload.Comments {
		_, _ = fmt.Fprintf(out, "\n%s\n%s\n",
			t.bold("comment "+comment.Name), strings.TrimRight(comment.Body, "\n"))
	}

	_, _ = fmt.Fprintf(out, "\n%s\n",
		t.dim(freshnessLine(payload.Freshness, s.cfg.FetchWarnAfter())))
}

// showFields is the block of key/value lines under the title. A field the file
// does not carry is not printed: an issue is not a form, and an empty row for
// every optional key would bury the three that are filled in.
func showFields(v *view, payload ShowPayload) [][]string {
	item := payload.Issue

	rows := [][]string{{"status", statusLine(item)}}

	rows = appendField(rows, "type", item.Type)
	rows = appendField(rows, "state", item.State)
	rows = appendField(rows, "owner", item.Owner)
	rows = appendField(rows, "created", item.Created)
	rows = appendField(rows, "priority", item.Priority)
	rows = appendField(rows, "repro", item.Repro)
	rows = appendField(rows, "acceptance", item.Acceptance)
	rows = appendField(rows, "question", item.Question)
	rows = appendField(rows, "reason", item.Reason)
	rows = appendField(rows, "resolution", item.Resolution)

	if payload.Parent != nil {
		rows = append(rows, []string{"parent", linkText(*payload.Parent)})
	}

	for i, blocker := range payload.Blockers {
		key := "blocked_by"
		if i > 0 {
			key = ""
		}

		rows = append(rows, []string{key, linkText(blocker)})
	}

	for i, ref := range item.Elsewhere {
		key := "branches"
		if i > 0 {
			key = ""
		}

		rows = append(rows, []string{key, ref})
	}

	rows = append(rows, []string{"trunk", v.trunkName})

	return rows
}

func appendField(rows [][]string, key, value string) [][]string {
	if value == "" {
		return rows
	}

	return append(rows, []string{key, value})
}

// statusLine is the status plus the annotations that survive losing to it.
func statusLine(item Issue) string {
	line := item.Status

	var notes []string

	// `reopened` is an annotation as well as a status, so that the fact
	// survives losing the precedence contest to in progress. When it won that
	// contest, saying it twice is not saying it more clearly.
	if item.Reopened && item.Status != string(model.StatusReopened) {
		notes = append(notes, "reopened")
	}

	if item.Contended {
		notes = append(notes, "contended")
	}

	if item.Stale {
		notes = append(notes, "stale")
	}

	if len(notes) > 0 {
		line += " (" + strings.Join(notes, ", ") + ")"
	}

	return line
}

func linkText(link Link) string {
	if !link.Known {
		return link.ID + "  (no issue with this id)"
	}

	return link.ID + "  " + link.Title + " (" + link.Status + ")"
}

func showClaim(claim Claim) string {
	who := claim.Claimant
	if who == "" {
		who = "someone"
	}

	line := "claimed by " + who + " on " + claim.Ref
	if claim.When != "" {
		line += ", " + claim.When
	}

	if claim.Stale {
		line += " — stale"
	}

	return line
}
