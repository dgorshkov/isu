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
}

// Documents are the documentation pages, in reading order. The order is the
// navigation, and the navigation is the reading order.
var Documents = []Document{
	{Name: "getting-started", Live: true},
	{Name: "data-model"},
	{Name: "statuses"},
	{Name: "checks"},
	{Name: "json"},
	{Name: "importing"},
	{Name: "field-notes"},
	{Name: "not-doing"},
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

	plan, err := planFrom(ctx, root, work)
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

	if err := chrome(r, files, pages, palette); err != nil {
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
func planFrom(ctx context.Context, root, work string) (Plan, error) {
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

	plan, err := ParsePlan(string(body))
	if err != nil {
		return Plan{}, err
	}

	for i, section := range plan.Sections {
		if section.Sample == nil {
			continue
		}

		sample := isu(demo, section.Sample.Args)
		plan.Sections[i].Sample.Want = sample.Output
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

		files["docs/"+d.Name+".html"] = html
		pages = append(pages, link{Href: d.Name + ".html", Text: doc.Title})
	}

	return pages, nil
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
func chrome(r *Renderer, files map[string][]byte, pages []link, palette map[string]string) error {
	a := &assembler{files: files}

	contents, err := r.Contents(pages)
	a.put("docs/index.html", contents, err)

	missing, err := r.NotFound(pages)
	a.put("404.html", missing, err)

	favicon, err := Favicon(palette)
	a.put("favicon.svg", favicon, err)

	card, err := Card(palette)
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
