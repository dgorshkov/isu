package cli

// This is the one file in internal/ outside gitx that starts a process, and the
// process it starts is the user's editor.
//
// M2-S1's rule is that internal/gitx is the only place that may build a *git*
// command, and it is enforced by a grep for exec.Command across internal/. The
// grep was a faithful proxy for the rule right up until M4-S6 asked for
// $EDITOR, which is not a git command and cannot be one. So the test names this
// file and asserts, of it, the thing the rule is actually about: that it does
// not run git. Everything else in internal/ is still held to the grep.
//
// The alternative was dropping $EDITOR, and a tracker whose comments can only
// be written with -m is a tracker people stop commenting on.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// streams are the three an editor is handed.
//
// Every command hands it the process's own. `isu ui` hands it the terminal
// bubbletea has just released, because an editor that cannot read the keyboard
// is an editor nobody can type into.
type streams struct {
	in  io.Reader
	out io.Writer
	err io.Writer
}

// ownStreams is the process's own, which is what every command but `isu ui`
// gives the editor. Output goes to stderr so that an editor drawing on the
// screen never lands in a pipe somebody is reading data out of.
func (a *app) ownStreams() streams {
	return streams{in: os.Stdin, out: a.env.Stderr, err: a.env.Stderr}
}

// defaultEditor is what to run when neither $ISU_EDITOR, $VISUAL nor $EDITOR
// says. It is the editor POSIX guarantees exists.
const defaultEditor = "vi"

// editorText opens the user's editor on an empty file and returns what they
// wrote.
//
// The file has a .md suffix so that an editor configured for markdown behaves
// like one, and it lives in its own temporary directory so that the name is
// predictable without ever colliding.
func (a *app) editorText(ctx context.Context, s *session) (string, error) {
	return a.editText(ctx, s, "COMMENT_EDITMSG.md", "", a.ownStreams())
}

// editText opens the editor on a file seeded with what it is given, and returns
// what came back.
//
// The seed is how `isu ui` asks for a whole issue: it writes the frontmatter a
// new issue starts with, the user fills it in, and what comes back is parsed by
// internal/issue rather than by a format this command invented.
func (a *app) editText(
	ctx context.Context, s *session, name, seed string, io streams,
) (string, error) {
	dir, err := os.MkdirTemp("", "isu-edit-")
	if err != nil {
		return "", fmt.Errorf("making room for the comment: %w", err)
	}

	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, name)
	if wrote := os.WriteFile(path, []byte(seed), 0o600); wrote != nil {
		return "", fmt.Errorf("making room for the comment: %w", wrote)
	}

	if edited := a.runEditor(ctx, s, path, io); edited != nil {
		return "", edited
	}

	written, err := os.ReadFile(path) //nolint:gosec // a path this function made
	if err != nil {
		return "", fmt.Errorf("reading what you wrote: %w", err)
	}

	return string(written), nil
}

// runEditor starts the editor and waits for it.
//
// The command line is split on spaces rather than parsed as a shell would,
// because $EDITOR is conventionally a program and its flags — `code --wait`,
// `emacsclient -nw` — and not a shell script. Anything needing a shell can name
// one.
func (a *app) runEditor(ctx context.Context, s *session, path string, io streams) error {
	fields := strings.Fields(a.editorCommand())

	//nolint:gosec // running the editor the user configured is the feature
	cmd := exec.CommandContext(ctx, fields[0], append(fields[1:], path)...)
	cmd.Dir = s.root
	cmd.Stdin = io.in
	cmd.Stdout = io.out
	cmd.Stderr = io.err

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running %s: %w", fields[0], err)
	}

	return nil
}

// editorCommand is which editor to run. ISU_EDITOR is first so that a person
// whose $EDITOR is something that never exits can still write a comment.
func (a *app) editorCommand() string {
	for _, name := range []string{"ISU_EDITOR", "VISUAL", "EDITOR"} {
		if value := strings.TrimSpace(a.env.getenv(name)); value != "" {
			return value
		}
	}

	return defaultEditor
}
