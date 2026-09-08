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

	card, err := OpenGraph(palette)
	require.NoError(t, err)
	require.Equal(t, image.Rect(0, 0, 1200, 630), card.Bounds(),
		"1200×630 is what every card scraper crops to")

	ground, err := RGB(palette["terminal"])
	require.NoError(t, err)
	require.Equal(t, ground, card.At(2, 2), "the corner is the terminal's own ground")

	encoded, err := Card(palette)
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

		_, err := OpenGraph(partial)
		require.ErrorContains(t, err, "declares no --"+missing)

		_, err = Card(partial)
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
