package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dgorshkov/isu/internal/config"
	"github.com/dgorshkov/isu/internal/importer"
	"github.com/dgorshkov/isu/internal/importer/github"
	"github.com/dgorshkov/isu/internal/issue"
)

// `isu import` is one command with the source as its argument rather than a
// subcommand per source, because everything after "which tracker" is the same
// for all of them: the same flags, the same dry run, the same guarded write.
// PLAN.md M7-S1 calls the interface behind it the seam Jira and Linear come
// back through; this is that seam's front.
//
// **A dry run is the default and writing requires --write.** An importer is the
// one command in this product that creates thousands of files, and a tool whose
// first invocation does that is a tool people run once, in the wrong
// repository.

// importOptions is one invocation.
type importOptions struct {
	dump      string
	fetchOnly string
	write     bool
	prefix    string
	owner     string
	state     string
	typeMap   []string
	fields    bool
	scan      bool
	samples   int
	tokenEnv  string
	base      string
}

// DefaultTokenEnv is where an API token is read from when nothing said
// otherwise.
//
// From the environment, never from a flag: a flag is in the shell history, in
// the process list, and in whatever CI log recorded the command line.
const DefaultTokenEnv = "GITHUB_TOKEN"

func (a *app) importCmd() *cobra.Command {
	var opts importOptions

	cmd := &cobra.Command{
		Use:   "import <source> [owner/repo]",
		Short: "read another tracker's issues into this repository",
		Long:  importLong,
		Args:  importArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return a.importFrom(cmd, args, &opts)
		},
	}

	f := cmd.Flags()
	f.StringVar(&opts.dump, "dump", "",
		"read a saved dump instead of the API, so an import is reproducible offline")
	f.StringVar(&opts.fetchOnly, "fetch-only", "",
		"write the dump to this path and stop, mapping nothing")
	f.BoolVar(&opts.write, "write", false,
		"write the issues; without it this is a dry run that writes nothing")
	f.StringVar(&opts.prefix, "id-prefix", "",
		"form ids under this prefix instead of the one in "+config.FileName)
	f.StringVar(&opts.owner, "owner", "",
		"who owns an issue the source left unassigned")
	f.StringVar(&opts.state, "state", github.StateAll,
		"which issues to read: open, closed or all")
	f.StringArrayVar(&opts.typeMap, "type-map", nil,
		"place a label or an issue type onto an isu type, as name=type; repeatable")
	f.BoolVar(&opts.fields, "fields", true,
		"read each issue's field values, which is one request per issue")
	f.BoolVar(&opts.scan, "scan", true,
		"scan this repository's history for the commits that resolved these issues")
	f.IntVar(&opts.samples, "samples", importer.DefaultSamples,
		"how many issues the dry run shows in full")
	f.StringVar(&opts.tokenEnv, "token-env", DefaultTokenEnv,
		"the environment variable holding the API token")
	f.StringVar(&opts.base, "api", github.DefaultBase, "the API root")

	return cmd
}

const importLong = `import reads another tracker's issues and writes them as folders in this
repository. v1.0.0 imports from GitHub Issues and nothing else.

A dry run is the default. It says what would be written — how many issues, of
what types, how many comments, how many attachment links, what it could not
place — and writes nothing at all. Pass --write when the report says what you
expected.

Ids keep the source's key where that key is already a legal id, and are formed
from it where it is not. GitHub numbers issues per repository, from the same
sequence it numbers pull requests, so #1234 becomes <PREFIX>-1234 and the
milestone numbered 3 becomes <PREFIX>-M3 — two sequences, two ids, no collision.
Milestones are epics: an issue belongs to at most one, which is exactly what
isu's single parent field holds.

Everything the schema has no place for — labels, the author, custom field
values, the sub-issue hierarchy, attachment links — goes to source.yml in the
issue's own folder and never into frontmatter. A mature tracker has two hundred
custom fields, and a schema that absorbs them is not a schema.

Attachments are recorded and not downloaded. A GitHub asset wants a browser
session on a private repository, so the links stay in the body byte for byte and
are listed in source.yml, where they still resolve on github.com.

The token is read from $GITHUB_TOKEN, never from a flag, and is redacted from
every message this command prints. A public repository needs none.`

// sources is every source `isu import` knows, by name.
var sources = []string{github.Name}

func importArgs(cmd *cobra.Command, args []string) error {
	if len(args) == 0 || len(args) > 2 {
		return usagef(cmd, "import needs a source — %s — and optionally a repository",
			strings.Join(sources, ", "))
	}

	for _, name := range sources {
		if args[0] == name {
			return nil
		}
	}

	return usagef(cmd, "isu imports from %s, and not from %q",
		strings.Join(sources, ", "), args[0])
}

func (a *app) importFrom(cmd *cobra.Command, args []string, opts *importOptions) error {
	ctx := cmd.Context()

	s, err := a.open(ctx)
	if err != nil {
		return err
	}

	if opts.fetchOnly != "" {
		return a.fetchOnly(ctx, cmd, args, opts)
	}

	source, err := a.source(cmd, args, opts)
	if err != nil {
		return err
	}

	batch, err := source.Load(ctx)
	if err != nil {
		return err
	}

	prefix := opts.prefix
	if prefix == "" {
		prefix = s.cfg.Prefix
	}

	mapping, err := importer.Keys(batch, prefix)
	if err != nil {
		return err
	}

	evidence, err := s.evidence(ctx, source, mapping, batch, opts.scan)
	if err != nil {
		return err
	}

	plan, err := importer.Map(batch, mapping, importer.Options{
		Prefix:   prefix,
		Owner:    opts.owner,
		Samples:  opts.samples,
		Evidence: evidence,
	})
	if err != nil {
		return err
	}

	if opts.write {
		if err := s.writeImport(ctx, cmd, plan); err != nil {
			return err
		}
	}

	return a.reportImport(plan, prefix)
}

// source builds the source this invocation asked for.
func (a *app) source(
	cmd *cobra.Command, args []string, opts *importOptions,
) (importer.Source, error) {
	repository := ""
	if len(args) == 2 {
		repository = args[1]
	}

	types, err := parseTypeMap(cmd, opts.typeMap)
	if err != nil {
		return nil, err
	}

	gh := github.Options{
		Repository: repository,
		State:      opts.state,
		TypeMap:    types,
		Fields:     opts.fields,
	}

	if opts.dump == "" {
		gh.Client = a.client(opts)
	} else if gh.Dump, err = os.ReadFile(opts.dump); err != nil {
		return nil, fmt.Errorf("reading the dump: %w", err)
	}

	source, err := github.New(gh)
	if err != nil {
		return nil, &usageError{cmd: cmd, err: err}
	}

	return source, nil
}

func (a *app) client(opts *importOptions) *github.Client {
	return &github.Client{
		Base:  opts.base,
		Token: importer.Secret(a.env.getenv(opts.tokenEnv)),
	}
}

// fetchOnly writes the dump and stops, which is what makes an import
// reproducible without a network and reviewable as a diff.
func (a *app) fetchOnly(
	ctx context.Context, cmd *cobra.Command, args []string, opts *importOptions,
) error {
	switch {
	case opts.dump != "":
		return usagef(cmd, "--fetch-only writes a dump and --dump reads one, so asking "+
			"for both is asking to copy a file")
	case len(args) != 2:
		return usagef(cmd, "--fetch-only needs a repository, as owner/repo")
	}

	body, err := a.client(opts).Fetch(ctx, args[1], opts.state, opts.fields)
	if err != nil {
		return err
	}

	if err := os.WriteFile(opts.fetchOnly, body, 0o644); err != nil { //nolint:gosec // a dump of somebody's issue list is not a secret
		return fmt.Errorf("writing the dump: %w", err)
	}

	return a.reportWrite(Write{Paths: []string{filepath.ToSlash(opts.fetchOnly)}})
}

// parseTypeMap reads the repeated name=type flag.
func parseTypeMap(cmd *cobra.Command, entries []string) (map[string]issue.Type, error) {
	out := map[string]issue.Type{}

	for _, entry := range entries {
		name, want, ok := strings.Cut(entry, "=")

		name = strings.ToLower(strings.TrimSpace(name))
		if !ok || name == "" {
			return nil, usagef(cmd, "--type-map is name=type, and %q is not", entry)
		}

		t := issue.Type(strings.ToLower(strings.TrimSpace(want)))
		if !t.Valid() || t == issue.TypeEpic {
			return nil, usagef(cmd, "--type-map places a label onto bug, story, chore or "+
				"spike, and not onto %q — an epic here is a milestone, and is not "+
				"something a label makes", want)
		}

		out[name] = t
	}

	return out, nil
}

// evidence is what the source and this repository's history together say about
// which commit resolved each issue.
//
// The source's own answer is recorded first and outranks everything the scan
// finds: GitHub already stores which pull request closed an issue, and it
// arrives with the issue rather than through an integration somebody installed.
//
// The scan cannot run before the issue list is in hand. A GitHub key is `#1234`
// and issues and pull requests are numbered from one sequence, so every match
// is checked against the set actually being imported and discarded when it is
// not one of them.
func (s *session) evidence(
	ctx context.Context, source importer.Source, m *importer.Mapping,
	batch *importer.Batch, scan bool,
) (*importer.Evidence, error) {
	e := importer.NewEvidence()

	for _, item := range batch.Items {
		if item.Closing == "" {
			continue
		}

		id, _ := m.ID(item.Key)
		e.Record(importer.Link{ID: id, Commit: item.Closing, Tier: importer.TierClosing})
	}

	if !scan {
		return e, nil
	}

	commits, err := s.repo.LoadCommits(ctx, s.trunk)
	if err != nil {
		return nil, err
	}

	importer.Scan(e, commits, source.Keys, m)

	return e, nil
}

// writeImport puts the folders on disk, through the one guarded path, and
// stages what it wrote.
func (s *session) writeImport(ctx context.Context, cmd *cobra.Command, plan *importer.Plan) error {
	if plan.Report.Unowned > 0 {
		return usagef(cmd,
			"%d issues have no assignee and no --owner: owner is the human answerable "+
				"for an issue, M5-S4 makes it expensive to correct afterwards, and there "+
				"is nothing here to guess it from", plan.Report.Unowned)
	}

	w := importer.Writer{Root: s.root}

	// Empty rather than nil, because an import of nothing that ran with --write
	// still ran with --write, and the report tells a dry run from a write by
	// whether this was set at all.
	paths := []string{}

	for _, folder := range plan.Folders {
		wrote, err := w.Write(folder)
		if err != nil {
			return err
		}

		paths = append(paths, wrote...)
	}

	plan.Report.Wrote = paths

	if len(paths) == 0 {
		return nil
	}

	// Staged rather than committed. An import is a change somebody reviews
	// before it is a commit, and five thousand folders is not a diff for an
	// importer to write a message about on their behalf.
	return s.git.Add(ctx, paths...)
}

// reportImport prints what the import did, or would do.
func (a *app) reportImport(plan *importer.Plan, prefix string) error {
	payload := importPayload(plan, prefix)

	if a.asJSON {
		return emit(a.env.Stdout, payload)
	}

	a.renderImport(payload)

	return nil
}

func importPayload(plan *importer.Plan, prefix string) ImportPayload {
	r := plan.Report

	out := ImportPayload{
		Source:      r.Source,
		Repository:  r.Repository,
		Prefix:      prefix,
		Wrote:       r.Wrote != nil,
		Items:       r.Items,
		Folders:     r.Folders,
		Epics:       r.Epics,
		Comments:    r.Comments,
		Attachments: r.Attachments,
		Fields:      r.Extra,
		Provenance:  r.Provenance,
		Unowned:     r.Unowned,
		Dangling:    r.Dangling,
		Requests:    r.Requests,
		Types:       r.Types,
		States:      r.States,
		Evidence:    r.Evidence,
		Found:       list(r.Found),
		Unplaced:    list(r.Unplaced),
		Notes:       list(r.Notes),
		Skipped:     []ImportSkip{},
		Samples:     []ImportSample{},
		Paths:       list(r.Wrote),
	}

	for _, skip := range r.Skipped {
		out.Skipped = append(out.Skipped, ImportSkip{Ref: skip.Ref, Why: skip.Why})
	}

	for _, sample := range r.Samples {
		out.Samples = append(out.Samples, ImportSample{
			ID:     sample.ID,
			Key:    sample.Key,
			Files:  list(sample.Files),
			README: sample.README,
		})
	}

	return out
}

// list is a slice that is never null, because a JSON array that is sometimes
// null makes every consumer write the same defensive branch.
func list(in []string) []string {
	if in == nil {
		return []string{}
	}

	return in
}

func (a *app) renderImport(p ImportPayload) {
	t := a.themeFor(a.env.Stdout)
	out := a.env.Stdout

	headline := "would write"
	if p.Wrote {
		headline = "wrote"
	}

	_, _ = fmt.Fprintf(out, "%s %s → %s, as %s-*\n",
		t.bold("import"), p.Source, p.Repository, p.Prefix)
	_, _ = fmt.Fprintf(out, "  %s %s from %s, %d of them epics\n",
		headline, plural(p.Folders, "folder"), plural(p.Items, "issue"), p.Epics)

	for _, line := range importCounts(p) {
		_, _ = fmt.Fprintf(out, "  %s\n", line)
	}

	importList(out, "found", p.Found)
	importList(out, "could not place", p.Unplaced)

	for _, note := range p.Notes {
		_, _ = fmt.Fprintf(out, "  %s\n", t.dim(note))
	}

	importSkipped(out, t, p.Skipped)
	importSamples(out, t, p.Samples)

	if !p.Wrote {
		_, _ = fmt.Fprintf(out, "\n%s\n",
			t.dim("nothing was written: pass --write when this says what you expected"))
	}
}

// importCounts is the body of the report, with the lines that would say nothing
// left out.
func importCounts(p ImportPayload) []string {
	lines := []string{"types: " + tally(p.Types), "states: " + tally(p.States)}

	add := func(n int, thing, rest string) {
		if n > 0 {
			lines = append(lines, plural(n, thing)+rest)
		}
	}

	add(p.Comments, "comment file", "")
	add(p.Attachments, "attachment link", ", recorded and not fetched")
	add(p.Fields, "value", " with nowhere in the schema, kept in "+importer.SourceFileName)
	add(p.Provenance, "required field", " written as a provenance line, not as content")
	add(p.Dangling, "link", " pointing outside the import, recorded and not written")
	add(p.Unowned, "issue", " with no assignee and no --owner, which --write refuses")
	add(p.Requests, "request", " against the rate-limit budget")

	if len(p.Evidence) > 0 {
		lines = append(lines, "resolving commits: "+tally(p.Evidence))
	}

	return lines
}

func importList(out io.Writer, label string, values []string) {
	if len(values) > 0 {
		_, _ = fmt.Fprintf(out, "  %s: %s\n", label, strings.Join(values, ", "))
	}
}

func importSkipped(out io.Writer, t theme, skipped []ImportSkip) {
	if len(skipped) == 0 {
		return
	}

	_, _ = fmt.Fprintf(out, "\n%s\n", t.bold("skipped"))

	rows := make([][]string, 0, len(skipped))
	for _, skip := range skipped {
		rows = append(rows, []string{skip.Ref, skip.Why})
	}

	for _, line := range columns(rows, "  ") {
		_, _ = fmt.Fprintf(out, "%s\n", line)
	}
}

func importSamples(out io.Writer, t theme, samples []ImportSample) {
	for _, sample := range samples {
		_, _ = fmt.Fprintf(out, "\n%s %s\n", t.bold(sample.ID), t.dim("("+sample.Key+")"))
		_, _ = fmt.Fprintf(out, "  %s\n", t.dim(strings.Join(sample.Files, "  ")))

		for line := range strings.Lines(sample.README) {
			_, _ = fmt.Fprintf(out, "  %s", line)
		}

		_, _ = fmt.Fprintln(out)
	}
}

// tally renders a count by name, in name order.
func tally(counts map[string]int) string {
	if len(counts) == 0 {
		return "none"
	}

	names := make([]string, 0, len(counts))
	for name := range counts {
		names = append(names, name)
	}

	sort.Strings(names)

	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}

	return strings.Join(parts, ", ")
}
