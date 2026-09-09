package site

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Document is one page of documentation and the repository its commands run
// against.
//
// M8-S3 asks for a test that extracts every fenced shell block from the docs
// and runs it, so the list of documents is data rather than a glob: a page
// nobody added here is a page nothing executes, and the test that reads docs/
// says so.
type Document struct {
	// Name is the file under docs/, without its extension, and the page's own
	// name under docs/ on the site.
	Name string
	// Live says the page's commands write, so it runs against a repository of
	// its own with no issues in it rather than against the sample repository.
	Live bool
	// Dump is a recorded GitHub issue list the page's commands import from,
	// copied into the page's repository under this name from
	// internal/site/testdata/import. It is a recording rather than a request
	// because a page that needs github.com to build is a page that stops
	// building, and because a sample nobody can reproduce is not evidence.
	Dump string
}

// Documents are the documentation pages, in reading order. The order is the
// navigation, and the navigation is the reading order.
var Documents = []Document{
	{Name: "getting-started", Live: true},
	{Name: "data-model"},
	{Name: "statuses"},
	{Name: "checks"},
	{Name: "json"},
	{Name: "importing", Dump: "acme.json"},
	{Name: "not-doing"},
	// Last, and after the reference pages rather than between two of them: this
	// is the maintainer's notebook, and a stranger reading the documentation is
	// not looking for it in the same list as the JSON contract.
	{Name: "field-notes"},
}

// Build produces the whole site: every path it publishes, and the bytes at it.
//
// root is the repository — the content plan, the docs and the templates are
// read from it — and work is a directory the build may fill with the scratch
// repositories the samples come from.
//
// Nothing is written here. A build that returns the site rather than writing it
// is a build a test can hold to the committed one byte for byte, which is what
// M8-S2 asks for: "a test regenerating every sample and failing if the
// committed page differs".
func Build(ctx context.Context, root, work string) (map[string][]byte, error) {
	r, err := NewRenderer(root)
	if err != nil {
		return nil, err
	}

	css, err := os.ReadFile(filepath.Join(root, "web", "assets", "site.css"))
	if err != nil {
		return nil, err
	}

	palette, err := Palette(string(css))
	if err != nil {
		return nil, err
	}

	plan, err := planFrom(ctx, root, work, string(css))
	if err != nil {
		return nil, err
	}

	// The share card sets the same sentence the page opens with, so it is read
	// here rather than written a second time in images.go.
	tagline, err := plan.Get("tagline")
	if err != nil {
		return nil, err
	}

	index, err := r.Index(plan)
	if err != nil {
		return nil, err
	}

	files := map[string][]byte{"index.html": index, "site.css": css}

	pages, err := documents(r, root, files)
	if err != nil {
		return nil, err
	}

	if err := chrome(r, files, pages, palette, tagline); err != nil {
		return nil, err
	}

	if err := Gates(files); err != nil {
		return nil, err
	}

	return files, nil
}

// planFrom reads the content plan, holds every sample in it to what isu
// actually prints, and takes the page's samples from that run rather than from
// the document.
//
// Both halves matter and they are not the same assertion. Taking the output
// from the run is what makes M8's governing constraint true — every sample on
// the site was produced by running the binary in this repository. Holding the
// document to it is what keeps the plan honest, so a reviewer reading
// CONTENT.md is reading what the product does.
func planFrom(ctx context.Context, root, work, css string) (Plan, error) {
	body, err := os.ReadFile(filepath.Join(root, "web", "CONTENT.md"))
	if err != nil {
		return Plan{}, err
	}

	demo, err := Fixture(ctx, work)
	if err != nil {
		return Plan{}, err
	}

	if err = Verify(demo, string(body)); err != nil {
		return Plan{}, fmt.Errorf("web/CONTENT.md: %w", err)
	}

	if err = TokensAgree(string(body), css); err != nil {
		return Plan{}, fmt.Errorf("web/CONTENT.md: %w", err)
	}

	plan, err := ParsePlan(string(body))
	if err != nil {
		return Plan{}, err
	}

	// The output the page shows is the output the run produced, not the output
	// the document claims — Verify above has already established that they are
	// the same thing.
	for i, section := range plan.Sections {
		for j, sample := range section.Samples {
			plan.Sections[i].Samples[j].Want = isu(demo, sample.Args).Output
		}
	}

	return plan, nil
}

// documents renders every page under docs/ and returns the navigation.
func documents(r *Renderer, root string, files map[string][]byte) ([]link, error) {
	var pages []link

	for _, d := range Documents {
		body, err := os.ReadFile(filepath.Join(root, "docs", d.Name+".md"))
		if err != nil {
			return nil, err
		}

		doc, err := Markdown(string(body))
		if err != nil {
			return nil, fmt.Errorf("docs/%s.md: %w", d.Name, err)
		}

		html, err := r.Page(d.Name, doc)
		if err != nil {
			return nil, err
		}

		gloss, err := summary(doc.Lede)
		if err != nil {
			return nil, fmt.Errorf("docs/%s.md: %w", d.Name, err)
		}

		files["docs/"+d.Name+".html"] = html
		pages = append(pages, link{Href: d.Name + ".html", Text: doc.Title, Note: gloss})
	}

	return pages, nil
}

// MinGloss is the shortest opening sentence a documentation page may have.
//
// The figure is not arbitrary and it is not a style rule: docs/field-notes.md
// opened with "M4-S8." — a fine first line for a document nobody arrives at
// cold, and nothing at all to somebody who landed on the index from a search.
// What this refuses is a reference token wearing a sentence's full stop. It is
// deliberately low enough to let a short sentence be a short sentence, because
// "An issue is a folder." is the best summary on this site.
const MinGloss = 20

// summary is the line the documentation index shows beside a page: the first
// sentence of the page's own opening paragraph.
//
// It is taken from the page rather than written a second time in the index,
// because a page that says one thing about itself in two places says two
// different things by the end of the quarter. A first sentence too short to be
// a summary fails the build instead of shipping as one.
func summary(lede string) (string, error) {
	sentence := lede
	if head, _, found := strings.Cut(lede, ". "); found {
		sentence = head + "."
	}

	if len(sentence) < MinGloss {
		return "", fmt.Errorf(
			"opens with %q, which is too short to say what the page is for", sentence)
	}

	return sentence, nil
}

// assembler collects generated files and keeps the first failure.
//
// Everything it holds is a rendering of data this package produced, so the
// alternative is four identical error checks between four one-line
// assignments, which is three more places to get the early return wrong than
// there is interesting behaviour here.
type assembler struct {
	files map[string][]byte
	err   error
}

func (a *assembler) put(path string, body []byte, err error) {
	switch {
	case a.err != nil:
	case err != nil:
		a.err = err
	default:
		a.files[path] = body
	}
}

// chrome is everything on the site that is not a page of prose: the docs
// index, the 404, the icons, the sitemap and robots.txt.
func chrome(
	r *Renderer, files map[string][]byte, pages []link,
	palette map[string]string, tagline string,
) error {
	a := &assembler{files: files}

	contents, err := r.Contents(pages)
	a.put("docs/index.html", contents, err)

	missing, err := r.NotFound(pages)
	a.put("404.html", missing, err)

	favicon, err := Favicon(palette)
	a.put("favicon.svg", favicon, err)

	card, err := Card(palette, tagline)
	a.put("og.png", card, err)

	if a.err != nil {
		return a.err
	}

	// The sitemap names every page, so it is written once everything that is
	// one has been.
	files["sitemap.xml"] = Sitemap(htmlPaths(files))
	files["robots.txt"] = Robots()

	return nil
}

// htmlPaths is every page the sitemap names: the site's HTML, except the 404,
// which is not a page anybody should be sent to.
func htmlPaths(files map[string][]byte) []string {
	var out []string

	for path := range files {
		if strings.HasSuffix(path, ".html") && path != "404.html" {
			out = append(out, path)
		}
	}

	sort.Strings(out)

	return out
}

// Write puts a built site on disk under dir.
//
// It is one loop over one map because that is the whole of the site's contact
// with the filesystem: a build that cannot write is a build that failed in one
// place rather than in fourteen.
func Write(dir string, files map[string][]byte) error {
	for _, path := range sortedPaths(files) {
		full := filepath.Join(dir, filepath.FromSlash(path))

		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			return err
		}

		if err := os.WriteFile(full, files[path], 0o600); err != nil {
			return err
		}
	}

	return nil
}

func sortedPaths(files map[string][]byte) []string {
	out := make([]string, 0, len(files))
	for path := range files {
		out = append(out, path)
	}

	sort.Strings(out)

	return out
}
