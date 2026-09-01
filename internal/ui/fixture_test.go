package ui_test

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"

	"github.com/dgorshkov/isu/internal/issue"
	"github.com/dgorshkov/isu/internal/model"
	"github.com/dgorshkov/isu/internal/ui"
)

// The fixtures here are built by hand rather than scripted with gittest, and
// that is the point of this package: internal/ui is a pure function of what it
// was handed, in the way internal/model is a pure function of what internal/repo
// loaded. A test that had to build a repository to assert a frame would be a
// test about the loader.
//
// Dates are still relative to the real clock, for the reason the CLI harness
// gives: an age is the distance between two moments, and pinning one end to the
// calendar makes it grow by a day every midnight. Both ends move together here,
// so "3d ago" stays "3d ago", and golden scrubs the dates themselves.

// now is the moment every fixture is measured against.
func now() time.Time { return time.Now() }

// daysAgo is a fixture date, relative to the clock the frame is rendered
// against.
func daysAgo(n int) time.Time { return now().AddDate(0, 0, -n) }

// item builds one derived issue. The fields a test does not care about are the
// ones a real board would carry anyway — an owner, a priority, a date — because
// a frame rendered from an empty issue is a frame about the empty case.
func item(id, title string, opts ...func(*model.Item)) *model.Item {
	it := &model.Item{
		ID:      id,
		OnTrunk: true,
		Status:  model.StatusOpen,
		Issue: &issue.Issue{
			Schema:   issue.CurrentSchema,
			ID:       id,
			Folder:   id,
			Title:    title,
			Type:     issue.TypeBug,
			State:    issue.StateOpen,
			Owner:    "dmitry",
			Created:  daysAgo(30),
			Priority: issue.PriorityP2,
		},
	}

	for _, opt := range opts {
		opt(it)
	}

	return it
}

func status(s model.Status) func(*model.Item) {
	return func(it *model.Item) { it.Status = s }
}

func kind(t issue.Type) func(*model.Item) {
	return func(it *model.Item) { it.Issue.Type = t }
}

func state(s issue.State) func(*model.Item) {
	return func(it *model.Item) { it.Issue.State = s }
}

func owner(name string) func(*model.Item) {
	return func(it *model.Item) { it.Issue.Owner = name }
}

func priority(p issue.Priority) func(*model.Item) {
	return func(it *model.Item) { it.Issue.Priority = p }
}

func parent(id string) func(*model.Item) {
	return func(it *model.Item) { it.Issue.Parent = id }
}

func field(set func(*issue.Issue)) func(*model.Item) {
	return func(it *model.Item) { set(it.Issue) }
}

func epic(children ...string) func(*model.Item) {
	return func(it *model.Item) {
		it.Issue.Type = issue.TypeEpic
		it.Issue.State = ""
		it.Epic = &model.Epic{Children: children}
	}
}

func claimedBy(who, ref string, days int) func(*model.Item) {
	return func(it *model.Item) {
		it.Claims = append(it.Claims, model.Claim{
			Ref: ref, Claimant: who, Email: who + "@example.com",
			Commit: "0123456789abcdef0123456789abcdef01234567",
			When:   daysAgo(days),
		})
		it.Elsewhere = append(it.Elsewhere, ref)
	}
}

func onBranch(ref string) func(*model.Item) {
	return func(it *model.Item) {
		it.OnTrunk = false
		it.Elsewhere = append(it.Elsewhere, ref)
	}
}

func reopened() func(*model.Item) {
	return func(it *model.Item) { it.Reopened = true }
}

// board is a repository holding an issue in every status the table in PLAN.md
// §1 names, which is what a renderer has to be correct about. It is the same
// seven issues the CLI harness scripts, so that a frame and a board can be read
// against each other.
func board() ui.Input {
	epical := item("ISU-epical", "Make login reliable", epic("ISU-openly"), owner("dmitry"))
	openly := item("ISU-openly", "Login retries drop the second attempt",
		priority(issue.PriorityP1), parent("ISU-epical"),
		field(func(i *issue.Issue) {
			i.Repro = "post twice"
			i.Body = "The second POST is dropped.\n"
		}))
	inprog := item("ISU-inprog", "Board renders epics", kind(issue.TypeStory),
		owner("alice"), status(model.StatusInProgress), state(issue.StateResolved),
		claimedBy("alice", "isu/ISU-inprog", 3),
		field(func(i *issue.Issue) { i.Acceptance = "the epic shows its children" }))
	reopen := item("ISU-reopen", "Upgrade the linter", kind(issue.TypeChore),
		owner("alice"), status(model.StatusReopened), reopened())
	donede := item("ISU-donede", "What does contention cost?", kind(issue.TypeSpike),
		status(model.StatusDone), state(issue.StateResolved),
		field(func(i *issue.Issue) { i.Question = "how much?" }))
	dropit := item("ISU-dropit", "Rewrite it in another language", kind(issue.TypeChore),
		status(model.StatusDropped), state(issue.StateDropped),
		field(func(i *issue.Issue) {
			i.Reason = "we like this one"
			i.Resolution = issue.ResolutionWontfix
		}))
	triage := item("ISU-triage", "Sign-up page 500s on Firefox", priority(issue.PriorityP0),
		owner("support"), status(model.StatusAwaitingTriage), onBranch("report/ISU-triage"),
		field(func(i *issue.Issue) { i.Repro = "sign up on Firefox" }))

	return input(donede, dropit, triage, inprog, reopen, epical, openly)
}

// input groups items the way `isu board` groups them: by status, in the
// precedence order of PLAN.md's table, dropping the statuses nothing matched.
// The CLI hands the real thing over; this is the same shape, built by hand.
func input(items ...*model.Item) ui.Input {
	byStatus := map[model.Status][]*model.Item{}
	for _, it := range items {
		byStatus[it.Status] = append(byStatus[it.Status], it)
	}

	var groups []ui.Group

	for _, s := range model.Statuses {
		if found := byStatus[s]; len(found) > 0 {
			groups = append(groups, ui.Group{Status: s, Items: found})
		}
	}

	board := &model.Board{Items: map[string]*model.Item{}}
	for _, it := range items {
		board.Items[it.ID] = it
	}

	return ui.Input{Data: ui.Data{
		Trunk:     "main",
		Board:     board,
		Groups:    groups,
		Freshness: "remote refs 2h ago",
		Now:       now(),
	}}
}

// golden compares a frame with a recorded one, and rewrites it under -update.
// The frames are this package's whole output, so a diff of one is the only
// review that catches a column that no longer lines up.
func golden(t *testing.T, name, got string) {
	t.Helper()

	path := filepath.Join("testdata", name)
	got = scrub(got)

	if updating() {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))

		return
	}

	want, err := os.ReadFile(path)
	require.NoErrorf(t, err, "no golden file: run `go test ./internal/ui -update`")
	require.Equal(t, string(want), got)
}

// dates are the one thing in a frame that cannot be the same twice, for the
// reason the fixtures above give. Everything else is the review.
var dates = regexp.MustCompile(`\d{4}-\d{2}-\d{2}`)

func scrub(s string) string { return dates.ReplaceAllString(s, "<date>") }

// send applies messages to a model the way the program would, one at a time.
func send(t *testing.T, m ui.Model, msgs ...tea.Msg) ui.Model {
	t.Helper()

	for _, msg := range msgs {
		next, _ := m.Update(msg)

		got, ok := next.(ui.Model)
		require.Truef(t, ok, "Update returned %T", next)

		m = got
	}

	return m
}

// sized is a model that has been told how big its terminal is, which is the
// first thing bubbletea tells a program.
func sized(t *testing.T, in ui.Input, width, height int) ui.Model {
	t.Helper()

	return send(t, ui.New(in), resize(width, height))
}

func resize(width, height int) tea.Msg {
	return tea.WindowSizeMsg{Width: width, Height: height}
}

// key is one keypress, spelled the way the key map spells it.
func key(name string) tea.Msg {
	switch name {
	case "enter", "esc", "tab", "up", "down", "left", "right", "home", "end",
		"pgup", "pgdown", "backspace", "ctrl+c", "ctrl+z":
		return tea.KeyMsg{Type: keyTypes[name]}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

// keys types a run of keys.
func keys(t *testing.T, m ui.Model, names ...string) ui.Model {
	t.Helper()

	msgs := make([]tea.Msg, 0, len(names))
	for _, name := range names {
		msgs = append(msgs, key(name))
	}

	return send(t, m, msgs...)
}

var keyTypes = map[string]tea.KeyType{
	"enter": tea.KeyEnter, "esc": tea.KeyEsc, "tab": tea.KeyTab,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd,
	"pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "backspace": tea.KeyBackspace,
	"ctrl+c": tea.KeyCtrlC, "ctrl+z": tea.KeyCtrlZ,
}

// lines splits a frame the way a terminal reads it.
func lines(f string) []string { return strings.Split(f, "\n") }

// updating reports whether -update was passed.
//
// The flag itself is registered by the golden-file helper that arrives with
// teatest, so this package reads it rather than declaring a second flag of the
// same name — which is a panic at init and not a warning. A build where nothing
// registers it rejects -update on the command line, which is loud enough.
func updating() bool {
	found := flag.Lookup("update")

	return found != nil && found.Value.String() == "true"
}
