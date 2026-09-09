package cli

import (
	"context"
	"fmt"

	"github.com/dgorshkov/isu/internal/ui"
)

// actions is what `isu ui` can do to a repository, and it is this package's
// commands rather than a second implementation of them.
//
// M6-S5 asks that the interface share command implementations with the
// CLI, and internal/ui is built so that it has no choice: it holds one
// derivation and nothing that could load another, so every read that is not on
// the board and every write at all comes back through here. `c` is the function
// `isu claim` runs, `n` is the draft-and-commit `isu new` runs with an editor in
// front of it, and the re-read afterwards is the view every read command starts
// from.
type actions struct {
	app     *app
	session *session
}

// Folder is what lives beside an issue, which is `isu show`'s own loader.
func (a *actions) Folder(id string) (ui.Folder, error) {
	ctx := context.Background()

	v, err := a.session.view(ctx)
	if err != nil {
		return ui.Folder{}, err
	}

	item, ok := v.board.Get(id)
	if !ok {
		return ui.Folder{}, fmt.Errorf("no issue %s on %s", id, v.trunkName)
	}

	attachments, comments, err := a.session.folder(ctx, item)
	if err != nil {
		return ui.Folder{}, err
	}

	held := ui.Folder{Attachments: attachments, Comments: make([]ui.Comment, 0, len(comments))}
	for _, comment := range comments {
		held.Comments = append(held.Comments, ui.Comment{Name: comment.Name, Body: comment.Body})
	}

	return held, nil
}

// Claim is `isu claim`.
func (a *actions) Claim(id string) (string, error) {
	ctx := context.Background()

	v, err := a.session.view(ctx)
	if err != nil {
		return "", err
	}

	written, err := a.session.claimIssue(ctx, v, id)
	if err != nil {
		return "", err
	}

	return said(written), nil
}

// Goto checks the claiming branch out.
func (a *actions) Goto(id string) (string, error) {
	return a.session.checkout(context.Background(), id)
}

// New files an issue through the user's editor.
func (a *actions) New(streams ui.Streams) (string, error) {
	written, err := a.app.fileFromEditor(context.Background(), a.session, streams)
	if err != nil {
		return "", err
	}

	return said(written), nil
}

// Reload re-derives the repository, which is what every read command does at
// its first line.
func (a *actions) Reload() (ui.Data, error) {
	v, err := a.session.view(context.Background())
	if err != nil {
		return ui.Data{}, err
	}

	loaded := v.itemGroups()
	groups := make([]ui.Group, 0, len(loaded))

	for _, group := range loaded {
		groups = append(groups, ui.Group{Status: group.status, Items: group.items})
	}

	return uiData(a.session, v, groups), nil
}

// said is a Write as one line, which is what the interface has room for. It is
// the same facts reportWrite prints, on one line instead of several.
func said(w Write) string {
	line := w.ID

	if w.Branch != "" {
		line += " on " + w.Branch
	}

	if w.Commit != "" {
		line += ", commit " + short(w.Commit)
	}

	if w.Pushed {
		line += ", pushed"
	}

	return line
}
