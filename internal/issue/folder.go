package issue

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The names inside an issue folder.
const (
	// ReadmeName is the issue itself. It is README.md so that a forge renders
	// the issue when you browse to the folder.
	ReadmeName = "README.md"
	// CommentsDir holds the append-only comment files, one per comment, named
	// <date>-<author>-<nn>.md.
	CommentsDir = "comments"
	// CommentExt is the extension a file in CommentsDir must have to be a
	// comment. Anything else in there is ignored.
	CommentExt = ".md"
)

// Comment is one file under an issue's comments/ directory. Comments are
// append-only and one per file: without the two-digit sequence in the name, a
// second comment by the same author on the same day silently overwrites the
// first.
type Comment struct {
	// Name is the file name, extension included.
	Name string
	// Body is the file's contents, verbatim.
	Body string
}

// Folder is one issue as it sits on disk: the issue itself, the attachments
// beside it, and its comments.
type Folder struct {
	// Path is the directory the folder was read from, which is where Write
	// puts it back.
	Path string
	// Issue is the decoded README.md.
	Issue *Issue
	// Attachments are the names of the files beside the README. They live with
	// the issue, in the same commit as the issue. Their contents are not read:
	// an attachment is arbitrary bytes and there is no reason to hold a
	// screenshot in memory to render a board. The size cap is M5-S2's, and it
	// reads the file it is measuring.
	Attachments []string
	// Comments are the files under comments/, in name order, which is date
	// order given how they are named.
	Comments []Comment
}

// Load reads the issue folder at dir.
//
// It decodes but does not validate: an issue that is on disk and wrong is
// still an issue that has to be readable, or `isu check` could never report
// what is wrong with it. Call Validate for that.
func Load(dir string) (*Folder, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the issue folder: %w", err)
	}

	f := &Folder{Path: dir}
	readme := false

	for _, entry := range entries {
		switch name := entry.Name(); {
		case name == ReadmeName:
			readme = true
		case name == CommentsDir:
			// Deliberately not gated on this being a directory: a plain file
			// named comments where the directory belongs is a mistake, and
			// quietly filing it as an attachment is how a mistake survives.
			if f.Comments, err = loadComments(filepath.Join(dir, name)); err != nil {
				return nil, err
			}
		case entry.IsDir():
			// Some other directory. Nothing in the layout puts one here, so it
			// is not ours to interpret.
		default:
			f.Attachments = append(f.Attachments, name)
		}
	}

	if !readme {
		return nil, fmt.Errorf("reading the issue folder: %s has no %s", dir, ReadmeName)
	}

	if f.Issue, err = loadIssue(filepath.Join(dir, ReadmeName)); err != nil {
		return nil, err
	}
	f.Issue.Folder = filepath.Base(dir)

	return f, nil
}

// Write puts the issue back, creating the folder if it is not there.
//
// Only README.md is written. Comments are append-only files with their own
// names and attachments are bytes nobody read, so rewriting either could only
// ever change something the caller did not ask to change.
func (f *Folder) Write() error {
	if err := os.MkdirAll(f.Path, 0o755); err != nil {
		return fmt.Errorf("creating the issue folder: %w", err)
	}

	path := filepath.Join(f.Path, ReadmeName)
	if err := os.WriteFile(path, f.Issue.Encode(), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}

// loadIssue reads and decodes one README.md.
func loadIssue(path string) (*Issue, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading the issue: %w", err)
	}

	doc, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	i, err := Decode(doc)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return i, nil
}

// loadComments reads the comment files in name order, which is date order
// given that they are named for their date.
func loadComments(dir string) ([]Comment, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading the comments: %w", err)
	}

	var comments []Comment

	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, CommentExt) {
			continue
		}

		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("reading a comment: %w", err)
		}

		comments = append(comments, Comment{Name: name, Body: string(body)})
	}

	return comments, nil
}
