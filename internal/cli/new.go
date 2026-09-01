package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// collisionAttempts is how many times `isu new` regenerates a token that
// collides with an id it can already see.
//
// Six characters of a 32-symbol alphabet is thirty bits, so a collision inside
// one repository is already an event; eight in a row is not a repository, it is
// a broken random source, and looping forever on one would hang the command
// rather than report it.
const collisionAttempts = 8

type newOptions struct {
	title      string
	issueType  string
	owner      string
	priority   string
	parent     string
	blockedBy  []string
	repro      string
	acceptance string
	question   string
	body       string
	noBranch   bool
}

func (a *app) newCmd() *cobra.Command {
	var opts newOptions

	cmd := &cobra.Command{
		Use:   "new",
		Short: "file a new issue",
		Long: `new writes an issue folder, generates an id nothing else in the repository
is using, and puts it on a report/<id> branch for a pull request.

The id is a hash of the issue's own fields and eight bytes of randomness, so
allocating one needs no counter, no lock and nobody's agreement — which is the
only thing that works when half the issues in flight are on branches nobody has
pushed yet. It is regenerated if it collides with any id isu can see.

--no-branch writes into the working tree and commits nothing, which is what a
bulk conversion needs.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.create(cmd, &opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.title, "title", "", "one line saying what this is (required)")
	f.StringVar(&opts.issueType, "type", string(issue.TypeBug),
		"bug, story, chore, spike or epic")
	f.StringVar(&opts.owner, "owner", "",
		"the human answerable for it (default: git's user.name)")
	f.StringVar(&opts.priority, "priority", string(issue.DefaultPriority), "p0, p1, p2 or p3")
	f.StringVar(&opts.parent, "parent", "", "the epic this belongs to")
	f.StringSliceVar(&opts.blockedBy, "blocked-by", nil, "issues this one waits on")
	f.StringVar(&opts.repro, "repro", "", "how to reproduce it (required on a bug)")
	f.StringVar(&opts.acceptance, "acceptance", "",
		"what done looks like (required on a story)")
	f.StringVar(&opts.question, "question", "",
		"what is being answered (required on a spike)")
	f.StringVar(&opts.body, "body", "", "the markdown below the frontmatter")
	f.BoolVar(&opts.noBranch, "no-branch", false,
		"write into the working tree instead of onto a report branch")

	return cmd
}

func (a *app) create(cmd *cobra.Command, opts *newOptions) error {
	ctx := cmd.Context()

	if opts.title == "" {
		return usagef(cmd, "new needs --title: an issue nobody can read the title of is a file")
	}

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	owner, err := s.whoami(ctx, opts.owner)
	if err != nil {
		return err
	}

	v, err := s.view(ctx)
	if err != nil {
		return err
	}

	if wrong := checkParent(cmd, v.board, opts.parent); wrong != nil {
		return wrong
	}

	draft, err := s.draft(v, opts, owner)
	if err != nil {
		return usagef(cmd, "%w", err)
	}

	file := change{path: readmePath(draft.ID), blob: draft.Encode()}

	if opts.noBranch {
		if _, wrote := s.stage(ctx, []change{file}); wrote != nil {
			return wrote
		}

		return a.reportWrite(Write{ID: draft.ID, Paths: []string{file.path}})
	}

	written, err := s.report(ctx, draft)
	if err != nil {
		return err
	}

	return a.reportWrite(written)
}

// report puts a new issue on its own branch, which is what `isu new` does when
// it is not asked for a bulk conversion.
//
// It is separate so that `isu ui` files through this and not through something
// that looks like it — the same reason claimIssue is separate.
func (s *session) report(ctx context.Context, draft *issue.Issue) (Write, error) {
	file := change{path: readmePath(draft.ID), blob: draft.Encode()}
	branch := reportBranchPrefix + draft.ID

	// The files are written before the switch so that a switch that cannot
	// happen — a branch of that name already there — leaves the working tree
	// carrying the report rather than losing it.
	if _, wrote := s.stage(ctx, []change{file}); wrote != nil {
		return Write{}, wrote
	}

	if switched := s.git.Switch(ctx, branch, true); switched != nil {
		return Write{}, switched
	}

	commit, err := s.git.Commit(ctx, "report "+draft.ID+"\n\n"+draft.Title+"\n")
	if err != nil {
		return Write{}, err
	}

	return Write{
		ID: draft.ID, Branch: branch, Commit: commit, Paths: []string{file.path},
	}, nil
}

// draft builds the issue file, generating an id nothing else is using.
func (s *session) draft(v *view, opts *newOptions, owner string) (*issue.Issue, error) {
	// `created` is a date and not a moment: it is what issue age is computed
	// from, and an issue filed at 23:59 is not a day older than one filed a
	// minute later.
	year, month, day := s.app.env.now().UTC().Date()
	created := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	draft := &issue.Issue{
		Schema:     issue.CurrentSchema,
		Title:      opts.title,
		Type:       issue.Type(opts.issueType),
		Owner:      owner,
		Created:    created,
		Priority:   issue.Priority(opts.priority),
		Parent:     opts.parent,
		BlockedBy:  opts.blockedBy,
		Repro:      opts.repro,
		Acceptance: opts.acceptance,
		Question:   opts.question,
		Body:       opts.body,
	}

	// An epic must not declare a state; everything else must. This is the only
	// way to create an epic, and it is why epic-ness is a field rather than
	// something inferred from who points at it.
	if draft.Type != issue.TypeEpic {
		draft.State = issue.StateOpen
	}

	id, err := s.allocate(v.taken, draft)
	if err != nil {
		return nil, err
	}

	draft.ID = id
	draft.Folder = id

	if err := draft.Validate(); err != nil {
		return nil, err
	}

	return draft, nil
}

// allocate generates an id no ref isu can see is already using.
//
// Regeneration is this command's job rather than the generator's: detecting a
// collision means loading the repository, and the generator is a pure function
// of the issue's own fields. A duplicate that survives this — two clones
// creating an issue in the same moment — stays a hard CI failure, and is rare
// enough to be an incident rather than a routine.
func (s *session) allocate(taken func(string) bool, draft *issue.Issue) (string, error) {
	for range collisionAttempts {
		id, err := s.cfg.NewID(draft.Title, draft.Owner, draft.Created)
		if err != nil {
			return "", err
		}

		if !taken(id) {
			return id, nil
		}
	}

	return "", fmt.Errorf(
		"generated %d ids in a row that %s already uses: that is not a collision, "+
			"it is a random source that has stopped being random",
		collisionAttempts, s.root)
}

// whoami is who git says the user is, which is who owns what they file.
func (s *session) whoami(ctx context.Context, given string) (string, error) {
	if given != "" {
		return given, nil
	}

	name, err := s.git.Config(ctx, "user.name")
	if err != nil {
		return "", err
	}

	if name == "" {
		return "", fmt.Errorf(
			"git has no user.name here, so isu cannot tell who owns this: " +
				"pass --owner, or set one with `git config user.name`")
	}

	return name, nil
}

// checkParent refuses a parent that is not an epic, at the command rather than
// at CI.
//
// PLAN.md is explicit that this is a repository-level rule and not a field one:
// Validate takes one issue and nothing else, so it checks that a parent is
// shaped like an id and stops. Here the whole board is loaded, so the rule can
// be enforced where the mistake is being made.
func checkParent(cmd *cobra.Command, board *model.Board, parent string) error {
	if parent == "" {
		return nil
	}

	item, ok := board.Get(parent)
	if !ok {
		return usagef(cmd, "no issue %s in this repository, so nothing can belong to it", parent)
	}

	if item.Issue == nil || item.Issue.Type != issue.TypeEpic {
		return usagef(cmd,
			"%s is not an epic, and only an epic has children: an epic declares "+
				"`type: epic` and no state of its own", parent)
	}

	return nil
}

// reportWrite is how every command that changed something says what it did.
func (a *app) reportWrite(w Write) error {
	if w.Paths == nil {
		w.Paths = []string{}
	}

	if a.asJSON {
		return emit(a.env.Stdout, w)
	}

	out := a.env.Stdout
	t := a.themeFor(out)

	var parts []string

	if w.Branch != "" {
		parts = append(parts, "on "+w.Branch)
	}

	if w.Commit != "" {
		parts = append(parts, "commit "+short(w.Commit))
	}

	if w.Pushed {
		parts = append(parts, "pushed")
	}

	head := w.ID
	if head == "" {
		head = strings.Join(w.Paths, ", ")
	}

	if head == "" {
		// `isu init` in a repository that already has everything it writes.
		// Doing nothing is the answer, and a blank line is not a way to say it.
		_, _ = fmt.Fprintln(out, "nothing to do")

		return nil
	}

	line := t.bold(head)
	if len(parts) > 0 {
		line += " · " + strings.Join(parts, " · ")
	}

	_, _ = fmt.Fprintln(out, line)

	for _, path := range w.Paths {
		_, _ = fmt.Fprintf(out, "  %s\n", t.dim(path))
	}

	return nil
}

// short is a commit id at the length a person reads.
func short(oid string) string {
	if len(oid) <= 8 {
		return oid
	}

	return oid[:8]
}

// newIssueTemplate is what `n` in the interface opens the editor on.
//
// It is a whole issue file rather than a form, so that what comes back is
// parsed by internal/issue and validated by the schema every other issue in the
// repository is held to. There is no second format to keep in step, and an
// editor configured for markdown behaves like one.
const newIssueTemplate = `---
schema: %d
title: 
type: bug
state: open
owner: %s
created: %s
priority: %s
repro: 
---
Anything below the frontmatter is the issue's body, in markdown.

Save an empty file to file nothing.
`

// errNothingWritten is somebody changing their mind in the editor, which is not
// a failure and must not read like one.
var errNothingWritten = errors.New("nothing was written, so nothing was filed")

// fileFromEditor is `n` from the interface: the user's editor on a new issue,
// and then the same branch-and-commit `isu new` makes.
//
// The id is allocated after the edit rather than before, because it is a hash
// of the issue's own fields and the fields are what the editor is for.
func (a *app) fileFromEditor(
	ctx context.Context, s *session, io ui.Streams,
) (Write, error) {
	owner, err := s.whoami(ctx, "")
	if err != nil {
		return Write{}, err
	}

	year, month, day := a.env.now().UTC().Date()
	created := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)

	seed := fmt.Sprintf(newIssueTemplate,
		issue.CurrentSchema, owner, created.Format(time.DateOnly), issue.DefaultPriority)

	text, err := a.editText(ctx, s, "NEW_ISSUE.md", seed,
		streams{in: io.In, out: io.Out, err: io.Err})
	if err != nil {
		return Write{}, err
	}

	if strings.TrimSpace(text) == "" {
		return Write{}, errNothingWritten
	}

	draft, err := decodeDraft(text)
	if err != nil {
		return Write{}, err
	}

	v, err := s.view(ctx)
	if err != nil {
		return Write{}, err
	}

	id, err := s.allocate(v.taken, draft)
	if err != nil {
		return Write{}, err
	}

	draft.ID, draft.Folder = id, id

	if err := draft.Validate(); err != nil {
		return Write{}, err
	}

	return s.report(ctx, draft)
}

// decodeDraft reads what came back from the editor, in internal/issue's words.
func decodeDraft(text string) (*issue.Issue, error) {
	doc, err := issue.Parse([]byte(text))
	if err != nil {
		return nil, fmt.Errorf("that is not an issue file: %w", err)
	}

	draft, err := issue.Decode(doc)
	if err != nil {
		return nil, fmt.Errorf("that is not an issue file: %w", err)
	}

	return draft, nil
}
