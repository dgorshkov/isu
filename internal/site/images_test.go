package site

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPaletteReadsTheTokensAndNothingElse(t *testing.T) {
	t.Parallel()

	got, err := Palette(":root {\n\t--paper: #fbf9f5;\n\tcolor-scheme: light dark;\n\tnot a rule\n}\n")
	require.NoError(t, err)
	require.Equal(t, map[string]string{"paper": "#fbf9f5"}, got)
}

func TestRGBReadsAColourAndRefusesEverythingElse(t *testing.T) {
	t.Parallel()

	got, err := RGB(" #12ab34 ")
	require.NoError(t, err)
	require.Equal(t, color.RGBA{R: 0x12, G: 0xab, B: 0x34, A: 0xff}, got)

	for _, bad := range []string{"12ab34", "#12ab3", "#zzzzzz", ""} {
		_, err := RGB(bad)
		require.ErrorContains(t, err, "is not a #rrggbb colour", bad)
	}
}

func TestTheImagesTakeTheirColoursFromTheStylesheet(t *testing.T) {
	t.Parallel()

	palette, err := Palette(stylesheet(t))
	require.NoError(t, err)

	icon, err := Favicon(palette)
	require.NoError(t, err)
	require.Contains(t, string(icon), palette["terminal"])
	require.Contains(t, string(icon), palette["glow"])
	require.True(t, strings.HasPrefix(string(icon), "<svg "))

	card, err := OpenGraph(palette, sampleTagline)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 1200, 630), card.Bounds(),
		"1200×630 is what every card scraper crops to")

	ground, err := RGB(palette["terminal"])
	require.NoError(t, err)
	require.Equal(t, ground, card.At(2, 2), "the corner is the terminal's own ground")

	encoded, err := Card(palette, sampleTagline)
	require.NoError(t, err)

	decoded, err := png.Decode(bytes.NewReader(encoded))
	require.NoError(t, err)
	require.Equal(t, card.Bounds(), decoded.Bounds())
}

func TestTheImagesSayWhichTokenTheStylesheetIsMissing(t *testing.T) {
	t.Parallel()

	full, err := Palette(stylesheet(t))
	require.NoError(t, err)

	for _, missing := range []string{"terminal", "terminal-ink", "glow"} {
		partial := map[string]string{}
		for k, v := range full {
			if k != missing {
				partial[k] = v
			}
		}

		_, err := OpenGraph(partial, sampleTagline)
		require.ErrorContains(t, err, "declares no --"+missing)

		_, err = Card(partial, sampleTagline)
		require.ErrorContains(t, err, "declares no --"+missing)
	}

	for _, missing := range []string{"terminal", "glow"} {
		partial := map[string]string{}
		for k, v := range full {
			if k != missing {
				partial[k] = v
			}
		}

		_, err := Favicon(partial)
		require.ErrorContains(t, err, "declares no --"+missing)
	}
}

// sampleTagline is the sentence the card is asked to set. It is the site's own,
// because the shapes below are the shapes the published card has.
const sampleTagline = "Issues that branch, merge and review like code."

func TestTheCardSetsTheTaglineAndNotJustAWordmark(t *testing.T) {
	t.Parallel()

	palette, err := Palette(stylesheet(t))
	require.NoError(t, err)

	card, err := OpenGraph(palette, sampleTagline)
	require.NoError(t, err)

	ink, err := RGB(palette["terminal-ink"])
	require.NoError(t, err)

	glow, err := RGB(palette["glow"])
	require.NoError(t, err)

	// The two things drawn in the accent are the chevron and the rule under the
	// wordmark, in that order.
	accent := bands(card, glow)
	require.Len(t, accent, 2, "the chevron and the rule")

	// Under that rule: the tagline, in the three lines it wraps to. The card
	// used to carry nothing there at all — a share card with a logo and no
	// argument, in front of a page whose argument is the only thing it has.
	var lines int

	for _, y := range bands(card, ink) {
		if y > accent[1] {
			lines++
		}
	}

	require.Equal(t, 3, lines, "the tagline, set under the rule")
}

func TestTheCardRefusesACharacterItCannotSet(t *testing.T) {
	t.Parallel()

	palette, err := Palette(stylesheet(t))
	require.NoError(t, err)

	_, err = OpenGraph(palette, "issues that branch — and merge")
	require.ErrorContains(t, err, "cannot set '—'")
}

func TestWrapBreaksOnWordsAndNeverInsideOne(t *testing.T) {
	t.Parallel()

	require.Equal(t, []string{"issues that", "branch, merge and", "review like code."},
		wrap(strings.ToLower(sampleTagline), 18))

	require.Equal(t, []string{"a", "supercalifragilistic", "word"},
		wrap("a supercalifragilistic word", 4),
		"a word longer than the measure gets a line rather than a hyphen")

	require.Empty(t, wrap("   ", 18))
}

// bands is the number of runs of consecutive rows carrying a colour, which is
// how many lines of type in that colour the card has.
func bands(img image.Image, c color.RGBA) []int {
	var (
		out  []int
		open bool
	)

	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		found := false

		for x := img.Bounds().Min.X; x < img.Bounds().Max.X && !found; x++ {
			found = img.At(x, y) == c
		}

		if found && !open {
			out = append(out, y)
		}

		open = found
	}

	return out
}
