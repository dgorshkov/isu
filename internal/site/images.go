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

// The card's typeface: five wide, nine tall, the last two rows for descenders.
//
// It used to be four glyphs, because the card used to be a wordmark and nothing
// else — 1200×630 of dark ground, a chevron and three letters nobody has heard
// of. That is the one asset that reaches a reader *before* the page does, in a
// Slack unfurl or a timeline, and on a site whose whole argument is "we show
// the bytes rather than promise things" it was the one surface making no
// argument at all. So the card now sets the tagline, and the tagline needs an
// alphabet.
//
// A bitmap is still the honest way to do it. The alternative is a font file — a
// third-party download, a subsetting step and a hundred kilobytes — to fill in
// some squares at one size, on an image the reader sees at 600px wide.
const (
	glyphWidth  = 5
	glyphHeight = 9
)

var glyphs = map[rune][9]string{
	' ':  {".....", ".....", ".....", ".....", ".....", ".....", ".....", ".....", "....."},
	'>':  {"#....", ".#...", "..#..", "...#.", "..#..", ".#...", "#....", ".....", "....."},
	'-':  {".....", ".....", ".....", ".####", ".....", ".....", ".....", ".....", "....."},
	'.':  {".....", ".....", ".....", ".....", ".....", ".##..", ".##..", ".....", "....."},
	',':  {".....", ".....", ".....", ".....", ".....", ".##..", ".##..", ".#...", "....."},
	'\'': {"..#..", "..#..", ".....", ".....", ".....", ".....", ".....", ".....", "....."},
	'a':  {".....", ".....", ".###.", "....#", ".####", "#...#", ".####", ".....", "....."},
	'b':  {"#....", "#....", "####.", "#...#", "#...#", "#...#", "####.", ".....", "....."},
	'c':  {".....", ".....", ".####", "#....", "#....", "#....", ".####", ".....", "....."},
	'd':  {"....#", "....#", ".####", "#...#", "#...#", "#...#", ".####", ".....", "....."},
	'e':  {".....", ".....", ".###.", "#...#", "#####", "#....", ".###.", ".....", "....."},
	'f':  {"..##.", ".#...", "####.", ".#...", ".#...", ".#...", ".#...", ".....", "....."},
	'g':  {".....", ".....", ".####", "#...#", "#...#", ".####", "....#", "#...#", ".###."},
	'h':  {"#....", "#....", "####.", "#...#", "#...#", "#...#", "#...#", ".....", "....."},
	'i':  {"..#..", ".....", ".##..", "..#..", "..#..", "..#..", ".###.", ".....", "....."},
	'j':  {"...#.", ".....", "..##.", "...#.", "...#.", "...#.", "...#.", "#..#.", ".##.."},
	'k':  {"#....", "#....", "#..#.", "#.#..", "##...", "#.#..", "#..#.", ".....", "....."},
	'l':  {".##..", "..#..", "..#..", "..#..", "..#..", "..#..", ".###.", ".....", "....."},
	'm':  {".....", ".....", "##.#.", "#.#.#", "#.#.#", "#.#.#", "#.#.#", ".....", "....."},
	'n':  {".....", ".....", "####.", "#...#", "#...#", "#...#", "#...#", ".....", "....."},
	'o':  {".....", ".....", ".###.", "#...#", "#...#", "#...#", ".###.", ".....", "....."},
	'p':  {".....", ".....", "####.", "#...#", "#...#", "####.", "#....", "#....", "#...."},
	'q':  {".....", ".....", ".####", "#...#", "#...#", ".####", "....#", "....#", "....#"},
	'r':  {".....", ".....", "#.##.", "##...", "#....", "#....", "#....", ".....", "....."},
	's':  {".....", ".....", ".####", "#....", ".###.", "....#", "####.", ".....", "....."},
	't':  {".#...", ".#...", "###..", ".#...", ".#...", ".#..#", "..##.", ".....", "....."},
	'u':  {".....", ".....", "#...#", "#...#", "#...#", "#...#", ".####", ".....", "....."},
	'v':  {".....", ".....", "#...#", "#...#", "#...#", ".#.#.", "..#..", ".....", "....."},
	'w':  {".....", ".....", "#...#", "#.#.#", "#.#.#", "#.#.#", ".#.#.", ".....", "....."},
	'x':  {".....", ".....", "#...#", ".#.#.", "..#..", ".#.#.", "#...#", ".....", "....."},
	'y':  {".....", ".....", "#...#", "#...#", "#...#", ".####", "....#", "#...#", ".###."},
	'z':  {".....", ".....", "#####", "...#.", "..#..", ".#...", "#####", ".....", "....."},
}

// letters refuses a tagline the card cannot set, rather than drawing a hole
// where the character should have been. A missing glyph would otherwise be a
// blank the build never mentions and nobody sees until it is on a timeline.
func letters(s string) error {
	for _, r := range s {
		if _, ok := glyphs[r]; !ok {
			return fmt.Errorf("the card's typeface cannot set %q", r)
		}
	}

	return nil
}

// wrap breaks a line of text into lines of at most n characters, on words. A
// word longer than n gets a line of its own rather than being cut in half.
func wrap(text string, n int) []string {
	var (
		lines []string
		line  string
	)

	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case len(line)+1+len(word) <= n:
			line += " " + word
		default:
			lines = append(lines, line)
			line = word
		}
	}

	if line != "" {
		lines = append(lines, line)
	}

	return lines
}

// OpenGraph draws the 1200×630 card every page names as its og:image: the
// wordmark, a rule, and the same tagline the page opens with.
//
// The tagline is passed in rather than written here, so the card and the page
// cannot come to disagree — it is the string out of web/CONTENT.md, and the one
// place the site's copy lives is that file.
func OpenGraph(palette map[string]string, tagline string) (image.Image, error) {
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

	// Lower case throughout: the wordmark is lower case, the terminal the card
	// is dressed as is lower case, and it halves the typeface.
	words := strings.ToLower(tagline)

	if err := letters(words); err != nil {
		return nil, err
	}

	const (
		width, height = 1200, 630
		margin        = 90
		markScale     = 9
		perLine       = 18
		leading       = 34
		above, below  = 30, 46
		ruleHeight    = 5
	)

	lines := wrap(words, perLine)

	// The headline is set to the width it is given rather than at a size chosen
	// here, so the block spans the card whatever the tagline turns out to say.
	longest := 1
	for _, line := range lines {
		longest = max(longest, len([]rune(line)))
	}

	scale := max(1, (width-2*margin)/(longest*(glyphWidth+1)-1))
	span := longest*(glyphWidth+1)*scale - scale

	mark := glyphHeight * markScale
	step := glyphHeight*scale + leading
	block := mark + above + ruleHeight + below + len(lines)*step - leading

	card := image.NewRGBA(image.Rect(0, 0, width, height))
	fill(card, card.Bounds(), ground)

	y := (height - block) / 2

	write(card, " isu", write(card, ">", margin, y, markScale, glow), y, markScale, ink)

	y += mark + above
	fill(card, image.Rect(margin, y, margin+span, y+ruleHeight), glow)

	y += ruleHeight + below

	for _, line := range lines {
		write(card, line, margin, y, scale, ink)
		y += step
	}

	return card, nil
}

// write sets one string, and returns the x the next one would start at.
func write(dst *image.RGBA, s string, x, y, scale int, c color.RGBA) int {
	for _, r := range s {
		draw(dst, glyphs[r], x, y, scale, c)
		x += (glyphWidth + 1) * scale
	}

	return x
}

// draw paints one glyph at scale.
func draw(dst *image.RGBA, glyph [glyphHeight]string, x, y, scale int, c color.RGBA) {
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
func Card(palette map[string]string, tagline string) ([]byte, error) {
	img, err := OpenGraph(palette, tagline)
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
