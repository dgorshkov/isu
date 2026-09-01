package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/check"
	"github.com/dgorshkov/isu/internal/gitx"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/repo"
)

// errFindings is `isu check` saying the repository broke a rule.
//
// It carries no message because the report is the message and has already been
// printed. Every other failure in this package is a sentence isu prints on
// stderr; this one would print a second, vaguer copy of a page the user is
// already reading.
var errFindings = errors.New("")

func (a *app) checkCmd() *cobra.Command {
	var (
		scope    string
		worktree bool
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "run the rules over this repository and this branch",
		Long:  checkLong(),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.check(cmd, scope, worktree)
		},
	}

	f := cmd.Flags()
	f.StringVar(&scope, "scope", "all", "which checks to run: tree, branch or all")
	f.BoolVar(&worktree, "worktree", false,
		"read the issues on disk rather than at a ref, uncommitted edits included")

	return cmd
}

const checkIntro = `check runs isu's rules and reports what they found. A failure exits 1 and a
warning does not: two people about to do the same work is worth saying and is
not a reason to refuse a pull request.

There are two kinds of rule. A tree rule is about the repository as it stands —
a parent that names nothing, an epic with no children — and needs no branch. A
branch rule is about what this branch proposes: a resolution with no code beside
it, an owner an agent reassigned. --scope picks one kind, and the pre-commit
hook isu init writes runs the tree rules only, because at pre-commit time the
change being committed is not a commit yet and the branch rules would be
answering from the commits already there.

--worktree reads the issues on disk instead of at a ref, which is what that hook
needs: a check that read a ref before a commit would be answering about the
commit before the one being made, and would refuse the commit that fixed what it
was complaining about. It implies --scope tree, because the working tree is not
a set of commits and there is nothing there to ask the branch rules about.`

// checkLong is the help, with the rules themselves in it.
//
// It is generated from the registry rather than written out, so that a check
// nobody documented is impossible: adding one is a file and a registry line,
// and this is where the line shows up.
func checkLong() string {
	var b strings.Builder

	b.WriteString(checkIntro)
	b.WriteString("\n\nThe checks:\n")

	rows := make([][]string, 0, check.Checks.Len())
	for _, c := range check.Checks.All() {
		rows = append(rows, []string{c.Name(), string(c.Scope()), c.Describe()})
	}

	for _, line := range columns(rows, "  ") {
		b.WriteString(line)
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

func (a *app) check(cmd *cobra.Command, scope string, worktree bool) error {
	ctx := cmd.Context()

	scopes, err := parseScope(cmd, scope)
	if err != nil {
		return err
	}

	if worktree {
		// The working tree is not a set of commits, so there is nothing here
		// for the branch rules to be about. Saying so rather than running them
		// over an empty diff, which would report nothing and look like a pass.
		scopes = []check.Scope{check.ScopeTree}
	}

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	head, err := s.headRef(ctx)
	if err != nil {
		return err
	}

	if worktree {
		head = ""
	}

	// The head is handed to the loader as well as to the checks. On a branch it
	// is one of refs/heads/* and the board already had it; in a pipeline, where
	// the checkout is detached at a merge commit, it is HEAD and nothing else
	// would name it — and a check suite that could not see the issues the pull
	// request adds is a check suite that passes every pull request that adds a
	// broken one.
	v, err := s.view(ctx, head)
	if err != nil {
		return err
	}

	in, err := s.checkInput(ctx, v, head)
	if err != nil {
		return err
	}

	if worktree {
		if err := s.readWorktree(ctx, &in, v); err != nil {
			return err
		}
	}

	report := check.Checks.Run(in, scopes...)

	payload := CheckPayload{
		Trunk:     v.trunkName,
		Head:      shortRef(head),
		Checks:    report.Checks,
		Findings:  findings(report),
		Failures:  report.Count(check.SeverityFail),
		Warnings:  report.Count(check.SeverityWarn),
		OK:        report.OK(),
		Worktree:  worktree,
		Freshness: v.freshness,
	}

	if a.asJSON {
		if err := emit(a.env.Stdout, payload); err != nil {
			return err
		}
	} else {
		a.renderCheck(s, payload)
	}

	if !payload.OK {
		return errFindings
	}

	return nil
}

// checkInput assembles everything the checks read. Nothing below this line runs
// git: that is the rule internal/check lives under, and this is the function
// that makes it affordable.
func (s *session) checkInput(ctx context.Context, v *view, head string) (check.Input, error) {
	in := check.Input{
		Trunk:  v.trunkName,
		Head:   head,
		Board:  v.board,
		Loaded: v.loaded,
		Config: s.cfg,
		Fetch:  check.Fetch{Remote: v.freshness.Remote, Newest: v.newestRemote},
		Now:    v.now,
	}

	// The files beside each issue are read at the ref under review, because
	// that is the tree the pull request is proposing: an attachment somebody
	// added on this branch is this branch's to answer for.
	at := s.trunk
	if head != "" {
		at = head
	}

	files, err := s.repo.LoadFiles(ctx, at)
	if err != nil {
		return in, err
	}

	in.Files = files

	if head == "" {
		return in, nil
	}

	branch, err := s.repo.LoadBranch(ctx, s.trunk, head)
	if err != nil {
		return in, err
	}

	in.Branch = branch

	return in, nil
}

// readWorktree points the tree rules at the issues on disk.
//
// The whole board is rebuilt from what is there, rather than the ref's board
// being patched, because the rules about one repository — a parent that names
// nothing, an epic with no children — are questions about a whole set and half
// a set answers them wrong. What is lost is every status that comes from
// comparing refs, and none of the tree rules reads one.
func (s *session) readWorktree(ctx context.Context, in *check.Input, v *view) error {
	set, err := s.repo.LoadWorktree(ctx)
	if err != nil {
		return err
	}

	files, err := s.repo.LoadWorktreeFiles(ctx)
	if err != nil {
		return err
	}

	loaded := &repo.Board{
		Trunk:   set,
		Refs:    map[string]*repo.Set{},
		Changed: map[string][]string{},
	}

	in.Loaded = loaded
	in.Files = files
	in.Branch = nil
	in.Board = model.Derive(model.Input{Loaded: loaded, Config: s.cfg, Now: v.now})

	return nil
}

// headRef is the ref under review.
//
// It is the branch HEAD is on, or HEAD itself where a pipeline checked out no
// branch — and it is empty when that resolves to trunk, because a branch that
// is trunk proposes nothing and every branch rule would be asking what a
// repository proposes to itself.
func (s *session) headRef(ctx context.Context) (string, error) {
	branch, err := s.git.CurrentBranch(ctx)
	if err != nil {
		return "", err
	}

	head := "HEAD"
	if branch != "" {
		head = repo.DefaultRefPattern + branch
	}

	at, err := s.resolve(ctx, head)
	if err != nil || at == "" {
		return "", err
	}

	trunk, err := s.resolve(ctx, s.trunk)
	if err != nil || at == trunk {
		return "", err
	}

	return head, nil
}

// resolve is RevParse with an unborn ref answering the empty string, which is
// what a repository with no commits gives for every ref it has.
func (s *session) resolve(ctx context.Context, rev string) (string, error) {
	at, err := s.git.RevParse(ctx, rev)
	if err != nil {
		if errors.Is(err, gitx.ErrUnknownRevision) {
			return "", nil
		}

		return "", err
	}

	return at, nil
}

// parseScope reads --scope.
func parseScope(cmd *cobra.Command, scope string) ([]check.Scope, error) {
	if scope == "all" {
		return nil, nil
	}

	for _, known := range check.Scopes {
		if string(known) == scope {
			return []check.Scope{known}, nil
		}
	}

	return nil, usagef(cmd, "%q is not a scope: tree, branch or all", scope)
}

// findings renders a report for the JSON contract.
func findings(report check.Report) []Finding {
	out := make([]Finding, 0, len(report.Findings))

	for _, f := range report.Findings {
		out = append(out, Finding{
			Check:    f.Check,
			Severity: string(f.Severity),
			ID:       f.ID,
			Path:     f.Path,
			Message:  f.Message,
		})
	}

	return out
}

func (a *app) renderCheck(s *session, payload CheckPayload) {
	out := a.env.Stdout
	t := a.themeFor(out)

	where := payload.Trunk
	switch {
	case payload.Worktree:
		where += " · the working tree"
	case payload.Head != "":
		where += " ← " + payload.Head
	}

	_, _ = fmt.Fprintf(out, "%s · %s · %s · %s\n",
		t.bold(where),
		plural(len(payload.Checks), "check"),
		summarise(payload.Failures, payload.Warnings),
		t.dim(freshnessLine(payload.Freshness, s.cfg.FetchWarnAfter())))

	if len(payload.Findings) == 0 {
		return
	}

	rows := make([][]string, 0, len(payload.Findings))

	for _, f := range payload.Findings {
		target := f.ID
		if target == "" {
			target = f.Path
		}

		rows = append(rows, []string{f.Severity, f.Check, target, f.Message})
	}

	_, _ = fmt.Fprintln(out)

	for _, line := range columns(rows, "") {
		_, _ = fmt.Fprintln(out, line)
	}
}

// summarise counts what a run found, in the words a person would use.
func summarise(failures, warnings int) string {
	switch {
	case failures == 0 && warnings == 0:
		return "nothing to report"
	case warnings == 0:
		return plural(failures, "failure")
	case failures == 0:
		return plural(warnings, "warning")
	default:
		return plural(failures, "failure") + ", " + plural(warnings, "warning")
	}
}

// shortRef is a ref name as a person writes it.
func shortRef(ref string) string { return strings.TrimPrefix(ref, repo.DefaultRefPattern) }
