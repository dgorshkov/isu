package site

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strconv"
	"strings"
)

// The two images the site ships, drawn here rather than committed as binaries.
//
// Both take their colours from web/assets/site.css, so the stylesheet is the
// one place the palette is declared — which is what M8-S2 asks for, and what
// stops a favicon quietly going on saying "burnt orange" after somebody changed
// the brand. Drawing them also keeps them tiny: the card is four glyphs and two
// rectangles, and it compresses to a few kilobytes.

// Palette reads the custom properties declared in the stylesheet's first token
// block. It is the site's one source of colour.
func Palette(css string) (map[string]string, error) {
	_, after, found := strings.Cut(css, ":root {")
	if !found {
		return nil, fmt.Errorf("the stylesheet declares no :root block")
	}

	body, _, _ := strings.Cut(after, "\n}")

	out := map[string]string{}

	for _, line := range strings.Split(body, "\n") {
		name, value, found := strings.Cut(strings.TrimSpace(line), ":")
		if !found || !strings.HasPrefix(name, "--") {
			continue
		}

		out[strings.TrimPrefix(name, "--")] = strings.TrimSuffix(strings.TrimSpace(value), ";")
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("the stylesheet's :root block declares no custom properties")
	}

	return out, nil
}

// RGB is one `#rrggbb` value, decoded.
func RGB(hex string) (color.RGBA, error) {
	value, ok := strings.CutPrefix(strings.TrimSpace(hex), "#")
	if !ok || len(value) != 6 {
		return color.RGBA{}, fmt.Errorf("%q is not a #rrggbb colour", hex)
	}

	n, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return color.RGBA{}, fmt.Errorf("%q is not a #rrggbb colour", hex)
	}

	return color.RGBA{
		R: uint8(n >> 16), //nolint:gosec // masked to a byte by the shift and the mask
		G: uint8(n >> 8 & 0xff),
		B: uint8(n & 0xff),
		A: 0xff,
	}, nil
}

// pick is one named colour from the palette.
func pick(palette map[string]string, name string) (color.RGBA, error) {
	hex, ok := palette[name]
	if !ok {
		return color.RGBA{}, fmt.Errorf("the stylesheet declares no --%s", name)
	}

	return RGB(hex)
}

// glyphs is the whole typeface these images need: the wordmark and the chevron
// in front of it, five wide and seven tall.
//
// A bitmap is the honest way to set four letters at one size. The alternative
// is a font file — a third-party download, a subsetting step and a hundred
// kilobytes — to draw eighty-four filled squares.
var glyphs = map[rune][7]string{
	'i': {"..#..", ".....", ".##..", "..#..", "..#..", "..#..", ".###."},
	's': {".###.", "#...#", "#....", ".###.", "....#", "#...#", ".###."},
	'u': {"#...#", "#...#", "#...#", "#...#", "#...#", "#...#", ".###."},
	'>': {"#....", ".#...", "..#..", "...#.", "..#..", ".#...", "#...."},
}

// OpenGraph draws the 1200×630 card every page names as its og:image.
func OpenGraph(palette map[string]string) (image.Image, error) {
	ground, err := pick(palette, "terminal")
	if err != nil {
		return nil, err
	}

	ink, err := pick(palette, "terminal-ink")
	if err != nil {
		return nil, err
	}

	glow, err := pick(palette, "glow")
	if err != nil {
		return nil, err
	}

	const (
		width, height = 1200, 630
		scale         = 34
		gap           = scale
	)

	card := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(card, card.Bounds(), ground)

	word := []rune("> isu")
	span := len(word)*5*scale + (len(word)-1)*gap
	start := (width - span) / 2
	y := (height - 7*scale) / 2
	x := start

	for _, r := range word {
		if r == ' ' {
			x += 5*scale + gap

			continue
		}

		colour := ink
		if r == '>' {
			colour = glow
		}

		draw(card, glyphs[r], x, y, scale, colour)

		x += 5*scale + gap
	}

	fill(card, image.Rect(start, y+9*scale, start+span, y+9*scale+scale/4), glow)

	return card, nil
}

// draw paints one glyph at scale.
func draw(dst *image.RGBA, glyph [7]string, x, y, scale int, c color.RGBA) {
	for row, bits := range glyph {
		for col, bit := range bits {
			if bit != '#' {
				continue
			}

			fill(dst, image.Rect(
				x+col*scale, y+row*scale, x+(col+1)*scale, y+(row+1)*scale), c)
		}
	}
}

func fill(dst *image.RGBA, r image.Rectangle, c color.RGBA) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst.SetRGBA(x, y, c)
		}
	}
}

// Card is the Open Graph image, encoded.
//
// png.Encode's error is discarded rather than returned, which is the one place
// in this package that happens. It writes to a bytes.Buffer, whose Write never
// fails, and the image is one this file made from an in-memory RGBA — so there
// is no failure to report and no caller who could act on one. The alternative
// is a branch no test can reach, and .golangci.yml says what this project does
// with an error that has nowhere to go.
func Card(palette map[string]string) ([]byte, error) {
	img, err := OpenGraph(palette)
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer

	_ = png.Encode(&out, img)

	return out.Bytes(), nil
}

// Favicon draws the tab icon: the chevron the wordmark opens with, on paper.
//
// SVG rather than a bitmap because it is a hundred and fifty bytes, it is sharp
// at every size a browser asks for, and it is the one image format a text diff
// can review.
func Favicon(palette map[string]string) ([]byte, error) {
	paper, err := pick(palette, "terminal")
	if err != nil {
		return nil, err
	}

	signal, err := pick(palette, "glow")
	if err != nil {
		return nil, err
	}

	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">`+
		`<rect width="32" height="32" rx="6" fill="%s"/>`+
		`<path d="M9 8 L19 16 L9 24" fill="none" stroke="%s" stroke-width="3.5" `+
		`stroke-linecap="round" stroke-linejoin="round"/>`+
		`<rect x="21" y="21" width="4" height="3.5" rx="1" fill="%s"/>`+
		"</svg>\n", hex(paper), hex(signal), hex(signal))), nil
}

func hex(c color.RGBA) string { return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B) }
