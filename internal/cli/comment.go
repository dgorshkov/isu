package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/issue"
)

func (a *app) commentCmd() *cobra.Command {
	var message string

	cmd := &cobra.Command{
		Use:   "comment <id>",
		Short: "add a comment to an issue",
		Long: `comment appends comments/<date>-<author>-<nn>.md to an issue's folder.

Comments are append-only, one file each. The two-digit sequence is not
decoration: without it a second comment by the same author on the same day
silently overwrites the first, and this picks the next one by looking at what is
already there.

The issue's own README is never touched. With no -m, your $EDITOR is opened on
an empty file and an empty file means nothing was written.`,
		Args: requireID,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.comment(cmd, args[0], message)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "the comment (default: open $EDITOR)")

	return cmd
}

func (a *app) comment(cmd *cobra.Command, id, message string) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	dir := issueDirOf(s, id)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf(
			"no issue %s in the working tree: a comment is a file in the issue's own "+
				"folder, so the folder has to be here to put one in", id)
	}

	if message == "" {
		message, err = a.editorText(ctx, s)
		if err != nil {
			return err
		}
	}

	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("nothing to say: an empty comment is not a comment")
	}

	author, err := s.whoami(ctx, "")
	if err != nil {
		return err
	}

	name, err := commentName(dir, a.env.now(), author)
	if err != nil {
		return err
	}

	path := issueDir(id) + "/" + issue.CommentsDir + "/" + name

	if _, err := s.stage(ctx, []change{{path: path, blob: []byte(body(message))}}); err != nil {
		return err
	}

	return a.reportWrite(Write{ID: id, Paths: []string{path}})
}

// body is the comment as it goes on disk: what was typed, ending in exactly one
// newline.
//
// Nothing is escaped and nothing is wrapped in frontmatter. A comment that
// starts a line with `---` is markdown, and the only thing that would read it as
// a delimiter is a parser pointed at the wrong file — comments are not
// frontmatter documents and nothing parses them as one.
func body(message string) string {
	return strings.TrimRight(message, "\n") + "\n"
}

// commentName picks the next file name for today's comment by this author.
//
// The sequence comes from what is on disk rather than from a count kept
// anywhere, because two people commenting on two branches keep no shared count
// — and the folder is the only thing that knows.
func commentName(dir string, now time.Time, author string) (string, error) {
	date := now.UTC().Format(time.DateOnly)
	slug := slugify(author)
	prefix := date + "-" + slug + "-"

	entries, err := os.ReadDir(filepath.Join(dir, issue.CommentsDir))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("reading %s: %w", issue.CommentsDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}

	sort.Strings(names)

	next := 1

	for _, name := range names {
		rest, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}

		seq, err := strconv.Atoi(strings.TrimSuffix(rest, issue.CommentExt))
		if err == nil && seq >= next {
			next = seq + 1
		}
	}

	return fmt.Sprintf("%s%02d%s", prefix, next, issue.CommentExt), nil
}

// slugPattern is everything that may not go in a comment's file name. An author
// is a person's name — spaces, accents, full stops — and a file name is not.
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	slug := strings.Trim(slugPattern.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if slug == "" {
		return "anon"
	}

	return slug
}

// issueDirOf is where an issue's folder sits on disk.
func issueDirOf(s *session, id string) string {
	return filepath.Join(s.root, filepath.FromSlash(issueDir(id)))
}

// join renders a list the way a sentence does.
func join(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	default:
		return strings.Join(items[:len(items)-1], ", ") + " or " + items[len(items)-1]
	}
}
