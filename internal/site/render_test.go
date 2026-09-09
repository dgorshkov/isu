package site

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A template names fields the data has to carry, and the compiler does not
// check that. html/template does, at execution, and this is the assertion that
// the failure reaches a caller rather than a page with a hole in it.
func TestRenderSaysWhichTemplateCouldNotBeExecuted(t *testing.T) {
	t.Parallel()

	r, err := NewRenderer(root)
	require.NoError(t, err)

	_, err = r.render("index", struct{}{})
	require.ErrorContains(t, err, "rendering index")

	_, err = r.page(shell{}, "index", struct{}{})
	require.ErrorContains(t, err, "rendering index",
		"a body that will not render is not wrapped in a shell")
}

func TestNewRendererSaysWhenThereAreNoTemplates(t *testing.T) {
	t.Parallel()

	_, err := NewRenderer(t.TempDir())
	require.ErrorContains(t, err, "reading the site's templates")
}

func TestTheSitemapAndRobotsNameTheSiteAbsolutely(t *testing.T) {
	t.Parallel()

	require.Equal(t,
		`<?xml version="1.0" encoding="UTF-8"?>`+"\n"+
			`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">`+"\n"+
			"<url><loc>"+SiteURL+"/index.html</loc></url>\n"+
			"</urlset>\n",
		string(Sitemap([]string{"index.html"})))

	require.Contains(t, string(Robots()), "Sitemap: "+SiteURL+"/sitemap.xml")
}
