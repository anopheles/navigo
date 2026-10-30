package cover

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

func read(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// solid is a w×h PNG of one colour.
func solid(t *testing.T, w, h int, c color.NRGBA) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeFormats(t *testing.T) {
	var jpg bytes.Buffer
	if err := jpeg.Encode(&jpg, image.NewRGBA(image.Rect(0, 0, 20, 20)), nil); err != nil {
		t.Fatal(err)
	}
	for name, b := range map[string][]byte{
		"WebP": read(t, "alpha.webp"), "PNG": read(t, "alpha.png"), "JPEG": jpg.Bytes(),
	} {
		if _, err := Decode(b); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := Decode([]byte("<html>")); err == nil {
		t.Error("an HTML page decoded as a cover")
	}
}

func TestDecodeSquaresAndShrinks(t *testing.T) {
	a, err := Decode(solid(t, 1000, 500, color.NRGBA{200, 0, 0, 255}))
	if err != nil {
		t.Fatal(err)
	}
	if a.Size() != Side {
		t.Fatalf("size = %d, want %d", a.Size(), Side)
	}
	// 2:1 on a square: the middle band is drawn, the top and bottom are clear.
	if got := a.img.NRGBAAt(Side/2, Side/2); got.A != 255 || got.R != 200 {
		t.Errorf("middle = %+v", got)
	}
	if got := a.img.NRGBAAt(Side/2, 5); got.A != 0 {
		t.Errorf("top edge = %+v, want clear", got)
	}
	small, _ := Decode(solid(t, 40, 40, color.NRGBA{1, 2, 3, 255}))
	if small.Size() != 40 {
		t.Errorf("a small cover was scaled to %d", small.Size())
	}
}

func TestShrinkKeepsTheEdgeColourClean(t *testing.T) {
	// Transparent black next to opaque red must not darken the red's edge.
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := 4; x < 8; x++ {
			img.SetNRGBA(x, y, color.NRGBA{255, 0, 0, 255})
		}
	}
	out := shrink(img, 3)
	for y := range 3 {
		for x := range 3 {
			if p := out.NRGBAAt(x, y); p.A > 0 && p.R != 255 {
				t.Fatalf("pixel %d,%d = %+v: red mixed with the hidden colour", x, y, p)
			}
		}
	}
}

func TestBlocksKeepTransparency(t *testing.T) {
	a, _ := Decode(solid(t, 32, 8, color.NRGBA{200, 30, 30, 255}))
	rows := a.Blocks(8, 4, nil)
	if len(rows) != 4 {
		t.Fatalf("%d rows", len(rows))
	}
	for i, r := range rows {
		if w := ansi.StringWidth(r); w != 8 {
			t.Errorf("row %d is %d cells wide: %q", i, w, r)
		}
	}
	// The image is 4:1, centred on a square, so the top and bottom rows are
	// clear: plain spaces with no colour set.
	for _, i := range []int{0, 3} {
		if strings.Contains(rows[i], "\x1b") || strings.TrimSpace(ansi.Strip(rows[i])) != "" {
			t.Errorf("row %d = %q, want clear cells", i, rows[i])
		}
	}
	// Row 2 holds the band's upper half only: ▀ drawn, with no background set.
	if !strings.Contains(rows[2], "▀") || strings.Contains(rows[2], "48;") {
		t.Errorf("row 2 = %q, want ▀ on the terminal's own background", rows[2])
	}
}

func TestBlocksBlendAgainstTheBackground(t *testing.T) {
	half := solid(t, 8, 8, color.NRGBA{255, 0, 0, 128})
	a, _ := Decode(half)
	dark := a.Blocks(2, 1, color.RGBA{20, 20, 40, 255})[0]
	if !strings.Contains(dark, "48;2;138;10;20") { // 255*0.5 + 20*0.5, 0+10, 0+20
		t.Errorf("not blended over the background: %q", dark)
	}
	if !strings.Contains(a.Blocks(2, 1, nil)[0], "255;0;0") {
		t.Error("without a background the colour should stay as it is")
	}
	faint, _ := Decode(solid(t, 8, 8, color.NRGBA{255, 0, 0, 3}))
	if r := faint.Blocks(2, 1, color.RGBA{20, 20, 40, 255})[0]; strings.Contains(r, "\x1b") {
		t.Errorf("an almost clear cover was drawn: %q", r)
	}
}

func TestBlocksSplitAColourEdge(t *testing.T) {
	// Left half red, right half blue: one cell is ▌ in red on blue.
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for y := range 2 {
		img.SetNRGBA(0, y, color.NRGBA{255, 0, 0, 255})
		img.SetNRGBA(1, y, color.NRGBA{0, 0, 255, 255})
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	a, _ := Decode(buf.Bytes())
	got := a.Blocks(1, 1, nil)[0]
	if !strings.Contains(got, "▌") || !strings.Contains(got, "38;2;255;0;0") ||
		!strings.Contains(got, "48;2;0;0;255") {
		t.Errorf("cell = %q, want red ▌ on blue", got)
	}
}

func TestPlaceholders(t *testing.T) {
	rows := Placeholders(0x010203, 3, 2)
	if len(rows) != 2 {
		t.Fatalf("%d rows", len(rows))
	}
	for r, row := range rows {
		if !strings.HasPrefix(row, "\x1b[38;2;1;2;3m") || !strings.HasSuffix(row, "\x1b[39m") {
			t.Errorf("row %d = %q: the id belongs in the foreground colour", r, row)
		}
		if n := strings.Count(row, string(placeholder)); n != 3 {
			t.Errorf("row %d has %d placeholders", r, n)
		}
		if w := ansi.StringWidth(row); w != 3 {
			t.Errorf("row %d is %d cells wide", r, w)
		}
	}
	if !strings.Contains(rows[1], string(diacritics[1])+string(diacritics[2])) {
		t.Errorf("row 1 column 2 lacks its numbers: %q", rows[1])
	}
	if Placeholders(1, MaxCells+1, 1) != nil || Placeholders(0, 1, 1) != nil ||
		Placeholders(1<<24, 1, 1) != nil {
		t.Error("an image kitty can't number was drawn")
	}
	if !utf8.ValidString(rows[0]) {
		t.Error("invalid UTF-8")
	}
}

func TestTransmit(t *testing.T) {
	a, _ := Decode(solid(t, 300, 300, color.NRGBA{9, 9, 9, 255}))
	plain := Transmit(7, 12, 6, a, 128, false)
	if !strings.HasPrefix(plain, "\x1b_Ga=T,U=1,f=100,i=7,c=12,r=6,q=2,m=") ||
		!strings.HasSuffix(plain, "\x1b\\") {
		t.Fatalf("start of %.60q", plain)
	}
	if strings.Count(plain, "\x1b_G") != strings.Count(plain, "\x1b\\") {
		t.Error("unbalanced escape codes")
	}
	if !strings.Contains(plain, ",m=0;") {
		t.Error("the last chunk is not marked")
	}
	wrapped := Transmit(7, 12, 6, a, 128, true)
	if !strings.HasPrefix(wrapped, "\x1bPtmux;\x1b\x1b_G") ||
		!strings.HasSuffix(wrapped, "\x1b\x1b\\\x1b\\") {
		t.Errorf("tmux passthrough: %.40q … %q", wrapped, wrapped[len(wrapped)-12:])
	}
	if got := Delete(7, false); got != "\x1b_Ga=d,d=I,i=7,q=2\x1b\\" {
		t.Errorf("Delete = %q", got)
	}
}

func TestSupported(t *testing.T) {
	env := func(kv ...string) func(string) string {
		return func(k string) string {
			for i := 0; i+1 < len(kv); i += 2 {
				if kv[i] == k {
					return kv[i+1]
				}
			}
			return ""
		}
	}
	for _, c := range []struct {
		name        string
		env         func(string) string
		passthrough bool
		want        bool
	}{
		{"kitty", env("TERM", "xterm-kitty"), false, true},
		{"ghostty", env("TERM_PROGRAM", "ghostty"), false, true},
		{"xterm", env("TERM", "xterm-256color"), true, false},
		{"kitty in tmux, passthrough off", env("KITTY_WINDOW_ID", "1", "TMUX", "/tmp/x"), false, false},
		{"kitty in tmux, passthrough on", env("KITTY_WINDOW_ID", "1", "TMUX", "/tmp/x"), true, true},
		{"tmux in an unknown terminal", env("TMUX", "/tmp/x"), true, false},
	} {
		if got := Supported(c.env, func() bool { return c.passthrough }); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestCacheKeepsAndEvicts(t *testing.T) {
	dir := t.TempDir()
	c := OpenCache(dir, 25)
	c.Put("a", bytes.Repeat([]byte("a"), 10))
	c.Put("b", bytes.Repeat([]byte("b"), 10))
	old := time.Now().Add(-time.Hour)
	_ = os.Chtimes(c.path("a"), old, old)
	if _, ok := c.Get("a"); !ok { // reading counts as use
		t.Fatal("a is gone")
	}
	_ = os.Chtimes(c.path("b"), old.Add(-time.Hour), old.Add(-time.Hour))
	c.Put("c", bytes.Repeat([]byte("c"), 10)) // 30 bytes: b, the least recent, goes
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been dropped")
	}
	for _, id := range []string{"a", "c"} {
		if _, ok := c.Get(id); !ok {
			t.Errorf("%s was dropped", id)
		}
	}
	// A new cache over the same directory trims it to size at once.
	_ = os.WriteFile(dir+"/big", bytes.Repeat([]byte("x"), 100), 0o600)
	OpenCache(dir, 25)
	if _, err := os.Stat(dir + "/big"); err == nil {
		t.Error("an oversized directory was kept")
	}
}

func TestCachePath(t *testing.T) {
	c := OpenCache(t.TempDir(), 1<<20)
	if got := c.Path("a"); got != "" {
		t.Errorf("uncached: got %q, want none", got)
	}
	c.Put("a", []byte("x"))
	b, err := os.ReadFile(c.Path("a"))
	if err != nil || string(b) != "x" {
		t.Errorf("cached: read %q, %v", b, err)
	}
}

func TestNilCacheKeepsNothing(t *testing.T) {
	var c *Cache
	c.Put("a", []byte("x"))
	if got := c.Path("a"); got != "" {
		t.Errorf("a nil cache returned the path %q", got)
	}
	if _, ok := c.Get("a"); ok {
		t.Error("a nil cache returned something")
	}
	if OpenCache("", 1) != nil {
		t.Error("an empty dir should give no cache")
	}
}

func TestAccent(t *testing.T) {
	hue := func(c color.Color) float64 {
		r, g, b, _ := c.RGBA()
		h, _, _ := colorful.Color{
			R: float64(r) / 65535,
			G: float64(g) / 65535,
			B: float64(b) / 65535,
		}.Hsl()
		return h
	}
	// A small saturated blue patch on a large dull grey-brown field.
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := range 100 {
		for x := range 100 {
			img.SetNRGBA(x, y, color.NRGBA{120, 110, 105, 255})
		}
	}
	for y := 40; y < 60; y++ {
		for x := 40; x < 60; x++ {
			img.SetNRGBA(x, y, color.NRGBA{20, 90, 240, 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	a, _ := Decode(buf.Bytes())
	c, ok := a.Accent()
	if !ok {
		t.Fatal("no accent for a cover with a blue patch")
	}
	if h := hue(c); h < 205 || h > 235 {
		t.Errorf("hue = %.0f, want blue", h)
	}
	r, g, b, _ := c.RGBA()
	_, s, l := colorful.Color{
		R: float64(r) / 65535,
		G: float64(g) / 65535,
		B: float64(b) / 65535,
	}.Hsl()
	if s < 0.49 || l < 0.51 || l > 0.67 {
		t.Errorf("saturation %.2f, lightness %.2f: not the evened-out tone", s, l)
	}
}

func TestAccentNeedsColour(t *testing.T) {
	for name, c := range map[string]color.NRGBA{
		"grey":        {128, 128, 128, 255},
		"black":       {0, 0, 0, 255},
		"white":       {255, 255, 255, 255},
		"transparent": {255, 0, 0, 0},
	} {
		a, _ := Decode(solid(t, 50, 50, c))
		if got, ok := a.Accent(); ok {
			t.Errorf("%s gave the accent %v", name, got)
		}
	}
}
