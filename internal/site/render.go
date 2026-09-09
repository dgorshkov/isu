package site

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"strings"

	"github.com/dgorshkov/isu/internal/cli"
)

// SiteURL is where the site is published, and the only absolute URL in it.
//
// Everything else on the site is relative, so the page works from a file://
// checkout, from a deploy preview at a URL nobody chose, and from a domain of
// its own — without being rebuilt. Only the canonical link, og:url, og:image
// and sitemap.xml need to know the host, and moving it is this constant and a
// `make site`.
const SiteURL = "https://isu-website.netlify.app"

// Source is the repository, linked from the masthead.
const Source = "https://github.com/dgorshkov/isu"

// templates are read from web/templates rather than embedded, because
// //go:embed cannot reach outside its own package directory and the site's
// sources belong in web/ beside the content plan, not in internal/. A build
// therefore reads the repository, which is what `make site` has anyway.
func parseTemplates(root string) (*template.Template, error) {
	t, err := template.ParseGlob(filepath.Join(root, "web", "templates", "*.tmpl"))
	if err != nil {
		return nil, fmt.Errorf("reading the site's templates: %w", err)
	}

	return t, nil
}

// Renderer turns content into pages. It carries the parsed templates and
// nothing else: every word on the site comes from the arguments.
type Renderer struct {
	templates *template.Template
}

// NewRenderer reads the templates under root.
func NewRenderer(root string) (*Renderer, error) {
	t, err := parseTemplates(root)
	if err != nil {
		return nil, err
	}

	return &Renderer{templates: t}, nil
}

// link is one entry in the masthead or in a list of pages.
type link struct {
	Href string
	Text string
	// Current marks the masthead entry for the page being read, which is what
	// aria-current="page" is for: a navigation that does not say where you are
	// is a navigation a screen reader reads identically on all ten pages.
	Current bool
	// Note is the one line saying what a page is for. It is empty everywhere
	// but the documentation index, where a bare list of eight titles tells a
	// reader arriving cold nothing about which two to read.
	Note string
}

// shell is what every page has in common.
type shell struct {
	Title       string
	Heading     string
	Description string
	Canonical   string
	Image       string
	// Rel is the prefix that reaches the site root from this page: empty at the
	// root, "../" one level down.
	Rel    string
	Home   string
	Footer string
	// Version is the build of isu that produced every sample on the site.
	Version string
	Nav     []link
	Body    template.HTML
}

// proofView is one terminal card: a command, and the bytes isu wrote.
type proofView struct {
	Command string
	Output  string
}

// sectionView is one landing-page section, with its copy rendered.
type sectionView struct {
	ID      string
	Title   string
	Claim   template.HTML
	Copy    []template.HTML
	Samples []proofView
}

type indexBody struct {
	Tagline string
	Lede    string
	Install string
	// Requires is what a reader needs before the install line will work. The
	// page shipped a single `go install` and never named a toolchain, which
	// leaves a reader without Go on the machine with no path at all.
	Requires string
	// HeroSample is the first section's proof, shown in the hero rather than
	// after two paragraphs of prose. A page whose whole argument is that the
	// output is real opened with no output on it.
	HeroSample *proofView
	Sections   []sectionView
}

type docBody struct {
	Heading  string
	Headings []Heading
	Content  template.HTML
}

type contentsBody struct {
	Heading string
	Lede    string
	Links   []link
}

type notFoundBody struct {
	Heading string
	Lede    string
	Links   []link
}

// render executes one template into bytes.
//
// The error is kept rather than discarded because html/template reports a field
// a template names and the data does not have, which is the one mistake this
// file can make that the compiler does not catch.
func (r *Renderer) render(name string, data any) ([]byte, error) {
	var out bytes.Buffer

	if err := r.templates.ExecuteTemplate(&out, name, data); err != nil {
		return nil, fmt.Errorf("rendering %s: %w", name, err)
	}

	return out.Bytes(), nil
}

// page wraps a rendered body in the shell.
func (r *Renderer) page(s shell, bodyTemplate string, data any) ([]byte, error) {
	body, err := r.render(bodyTemplate, data)
	if err != nil {
		return nil, err
	}

	s.Body = template.HTML(body) //nolint:gosec // rendered by html/template above
	s.Heading = s.Title

	return r.render("page", s)
}

// footer is the one line at the foot of every page. It says where the samples
// came from, because a site that shows terminal output and does not say where
// it got it is a site asking to be taken on trust.
const footer = "Every sample on this site was produced by running isu against a " +
	"repository the site's build creates from scratch. Apache-2.0."

// nav is the masthead, with hrefs relative to a page at depth rel, and the
// entry for path marked as the one being read.
func nav(rel, path string) []link {
	here := rel + strings.TrimPrefix(path, "/")

	out := []link{
		{Href: rel + "docs/getting-started.html", Text: "Getting started"},
		{Href: rel + "docs/index.html", Text: "Docs"},
		{Href: Source, Text: "Source"},
	}

	for i := range out {
		out[i].Current = out[i].Href == here
	}

	return out
}

// newShell is the head and chrome of one page.
func newShell(title, description, path, rel string) shell {
	return shell{
		Title:       title,
		Description: description,
		Canonical:   SiteURL + path,
		Image:       SiteURL + "/og.png",
		Rel:         rel,
		Home:        rel + "index.html",
		Footer:      footer,
		Version:     cli.Version,
		Nav:         nav(rel, path),
	}
}

// Index renders the landing page from the content plan.
func (r *Renderer) Index(plan Plan) ([]byte, error) {
	fields := map[string]string{}

	for _, key := range []string{
		"title", "tagline", "description", "lede", "install", "requires",
	} {
		value, err := plan.Get(key)
		if err != nil {
			return nil, err
		}

		fields[key] = value
	}

	body := indexBody{
		Tagline:  fields["tagline"],
		Lede:     fields["lede"],
		Install:  fields["install"],
		Requires: fields["requires"],
	}
	for i, section := range plan.Sections {
		v := view(section)

		// The first section's proof is the hero's. Nothing about a page that
		// argues from real output should put four paragraphs in front of the
		// first of it.
		if i == 0 && len(v.Samples) > 0 {
			body.HeroSample = &v.Samples[0]
			v.Samples = v.Samples[1:]
		}

		body.Sections = append(body.Sections, v)
	}

	return r.page(newShell(fields["title"], fields["description"], "/index.html", ""),
		"index", body)
}

// view renders one section's markdown into the shapes the template takes.
func view(s Section) sectionView {
	out := sectionView{
		ID:    s.ID,
		Title: s.Title,
		Claim: template.HTML(Inline(s.Claim)), //nolint:gosec // Inline escapes everything else
	}

	for _, para := range s.Copy {
		out.Copy = append(out.Copy, template.HTML(Inline(para))) //nolint:gosec // as above
	}

	for _, sample := range s.Samples {
		out.Samples = append(out.Samples, proofView{
			Command: sample.String(),
			Output:  strings.TrimRight(sample.Want, "\n"),
		})
	}

	return out
}

// Page renders one documentation page.
func (r *Renderer) Page(name string, doc Doc) ([]byte, error) {
	s := newShell(doc.Title+" — isu", doc.Lede, "/docs/"+name+".html", "../")

	return r.page(s, "doc", docBody{
		Heading:  doc.Title,
		Headings: doc.Headings,
		Content:  template.HTML(doc.HTML), //nolint:gosec // Markdown escapes everything else
	})
}

// Contents renders the documentation index from the pages beside it.
//
// It goes through a template rather than a strings.Builder, like every other
// page here. Assembling markup in Go was one page's worth of shortcut and it
// was the page that then could not carry a second line per entry.
func (r *Renderer) Contents(pages []link) ([]byte, error) {
	s := newShell("Documentation — isu",
		"Every page of isu's documentation, in reading order, and what each one is for.",
		"/docs/index.html", "../")

	return r.page(s, "contents", contentsBody{
		Heading: "Documentation",
		Lede: "In reading order, not alphabetical. A list sorted by title is one where " +
			"getting started comes fourth, and the first two pages below are the two " +
			"worth reading before deciding anything.",
		Links: pages,
	})
}

// NotFound renders the 404 page.
//
// Its links are absolute rather than relative: a 404 is served for a path
// nobody chose, so there is no depth for a relative link to be relative to.
func (r *Renderer) NotFound(pages []link) ([]byte, error) {
	s := newShell("Not here — isu", "That page is not on this site.", "/404.html", "")
	s.Home = SiteURL + "/index.html"
	s.Nav = []link{
		{Href: SiteURL + "/docs/getting-started.html", Text: "Getting started"},
		{Href: SiteURL + "/docs/index.html", Text: "Docs"},
		{Href: Source, Text: "Source"},
	}

	links := make([]link, 0, len(pages))
	for _, p := range pages {
		links = append(links, link{Href: SiteURL + "/docs/" + p.Href, Text: p.Text})
	}

	return r.page(s, "notfound", notFoundBody{
		Heading: "Not here",
		Lede: "That page is not on this site. These are, and one of them is " +
			"probably the one you meant.",
		Links: links,
	})
}

// Sitemap lists every page, which is the one file on the site that has to name
// absolute URLs.
func Sitemap(paths []string) []byte {
	var b strings.Builder

	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")

	for _, path := range paths {
		b.WriteString("<url><loc>" + SiteURL + "/" + path + "</loc></url>\n")
	}

	b.WriteString("</urlset>\n")

	return []byte(b.String())
}

// Robots is the one line a crawler reads before anything else.
func Robots() []byte {
	return []byte("User-agent: *\nAllow: /\nSitemap: " + SiteURL + "/sitemap.xml\n")
}
