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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
	dir, err := os.MkdirTemp("", "isu-comment-")
	if err != nil {
		return "", fmt.Errorf("making room for the comment: %w", err)
	}

	defer func() { _ = os.RemoveAll(dir) }()

	path := filepath.Join(dir, "COMMENT_EDITMSG.md")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return "", fmt.Errorf("making room for the comment: %w", err)
	}

	if err := a.runEditor(ctx, s, path); err != nil {
		return "", err
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
func (a *app) runEditor(ctx context.Context, s *session, path string) error {
	fields := strings.Fields(a.editorCommand())

	//nolint:gosec // running the editor the user configured is the feature
	cmd := exec.CommandContext(ctx, fields[0], append(fields[1:], path)...)
	cmd.Dir = s.root
	cmd.Stdin = os.Stdin
	cmd.Stdout = a.env.Stderr
	cmd.Stderr = a.env.Stderr

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
