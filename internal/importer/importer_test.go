package importer_test

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/issue"
)

// created is the date every fixture issue is dated, and is a constant rather
// than the clock because nothing here is aged against anything: these tests are
// about a mapping, and a mapping does not know what day it is.
var created = time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)

// fake is a source with nothing behind it, which is the whole point of the
// interface M7-S1 asks for: the mapping, the dry run and the guarded
// write are the same for every tracker, and a test of them should not have to
// be a test of GitHub.
type fake struct {
	batch *importer.Batch
	err   error
}

func (f fake) Name() string     { return "fake" }
func (f fake) Describe() string { return "a source with nothing behind it" }

func (f fake) Load(context.Context) (*importer.Batch, error) {
	return f.batch, f.err
}

// Keys reads a Jira-shaped key, which is the case where the source's own key is
// already a legal id.
func (f fake) Keys(text string) []string {
	var out []string

	for _, word := range strings.Fields(text) {
		word = strings.Trim(word, "().,'\"")
		if at := strings.Index(word, "-"); at > 0 && issue.ValidID(word) {
			if _, err := strconv.Atoi(word[at+1:]); err == nil {
				out = append(out, word)
			}
		}
	}

	return out
}

// batch is a small import: one bug with an owner, one story without.
func batch() *importer.Batch {
	return &importer.Batch{
		Source:     "fake",
		Repository: "acme/widgets",
		Items: []importer.Item{
			{
				Key: "PROJ-1234", Ref: "acme/widgets PROJ-1234",
				URL:   "https://example.invalid/PROJ-1234",
				Title: "Login retries drop the second attempt",
				Type:  issue.TypeBug, State: issue.StateOpen,
				Owner: "dmitry", Created: created,
				Body:  "The second POST is dropped.\n",
				Extra: map[string]any{"labels": []any{"login"}},
			},
			{
				Key: "#7", Ref: "acme/widgets#7",
				Title: "Board renders epics",
				Type:  issue.TypeStory, State: issue.StateResolved,
				Owner: "alice", Created: created,
			},
		},
	}
}

// mapped is the plan a batch produces, which is what most of these assert on.
func mapped(t *testing.T, b *importer.Batch, opts importer.Options) *importer.Plan {
	t.Helper()

	if opts.Prefix == "" {
		opts.Prefix = "ISU"
	}

	m, err := importer.Keys(b, opts.Prefix)
	if err != nil {
		t.Fatalf("forming ids: %v", err)
	}

	plan, err := importer.Map(b, m, opts)
	if err != nil {
		t.Fatalf("mapping: %v", err)
	}

	return plan
}

// find is one folder out of a plan, by id.
func find(t *testing.T, plan *importer.Plan, id string) importer.Folder {
	t.Helper()

	for _, f := range plan.Folders {
		if f.ID == id {
			return f
		}
	}

	t.Fatalf("no folder %s in %v", id, ids(plan))

	return importer.Folder{}
}

func ids(plan *importer.Plan) []string {
	out := make([]string, 0, len(plan.Folders))
	for _, f := range plan.Folders {
		out = append(out, f.ID)
	}

	return out
}

// file is one file out of a folder, by name.
func file(t *testing.T, f importer.Folder, name string) string {
	t.Helper()

	for _, one := range f.Files {
		if one.Name == name {
			return string(one.Body)
		}
	}

	t.Fatalf("%s has no %s", f.ID, name)

	return ""
}

func names(f importer.Folder) []string {
	out := make([]string, 0, len(f.Files))
	for _, one := range f.Files {
		out = append(out, one.Name)
	}

	return out
}

// custom is an item carrying however many fields the schema has no place for,
// which is the two hundred M7-S1 names.
func custom(n int) importer.Item {
	extra := make(map[string]any, n)
	for i := range n {
		extra[fmt.Sprintf("field_%03d", i)] = fmt.Sprintf("value %d", i)
	}

	return importer.Item{
		Key: "#1", Ref: "acme/widgets#1", Title: "A ticket with a mature tracker behind it",
		Type: issue.TypeChore, State: issue.StateOpen, Owner: "dmitry", Created: created,
		Extra: extra,
	}
}
