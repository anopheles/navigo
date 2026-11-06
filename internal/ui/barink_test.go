package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

var sgrAt = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// drawnAs is the foreground and background SGR parameters text is drawn
// with in line: "38;2;r;g;b", "30", ... and "" for the terminal's own.
func drawnAs(t *testing.T, line, text string) (fg, bg string) {
	t.Helper()
	at := strings.Index(line, text)
	if at < 0 {
		t.Fatalf("no %q in %q", text, line)
	}
	for _, m := range sgrAt.FindAllStringSubmatchIndex(line[:at], -1) {
		ps := strings.Split(line[m[2]:m[3]], ";")
		for i := 0; i < len(ps); i++ {
			switch p := ps[i]; {
			case p == "" || p == "0":
				fg, bg = "", ""
			case (p == "38" || p == "48") && i+4 < len(ps) && ps[i+1] == "2":
				v := strings.Join(ps[i:i+5], ";")
				if p == "38" {
					fg = v
				} else {
					bg = v
				}
				i += 4
			default:
				n, _ := strconv.Atoi(p)
				switch {
				case n >= 30 && n <= 37, n >= 90 && n <= 97:
					fg = p
				case n >= 40 && n <= 47, n >= 100 && n <= 107:
					bg = p
				}
			}
		}
	}
	return fg, bg
}

// exactOf is the colour of a "38;2;r;g;b" or "48;2;r;g;b" parameter.
func exactOf(t *testing.T, p string) colorful.Color {
	t.Helper()
	ps := strings.Split(p, ";")
	if len(ps) != 5 {
		t.Fatalf("%q is not an exact colour", p)
	}
	v := func(s string) float64 { n, _ := strconv.Atoi(s); return float64(n) / 255 }
	return colorful.Color{R: v(ps[2]), G: v(ps[3]), B: v(ps[4])}
}

func themeHarness(t *testing.T, theme string, bg color.Color) *harness {
	t.Helper()
	h := setupHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Theme = config.Theme{Name: theme}
	}, func(hs *harness) {
		hs.m.deps.TrueColor = true
		applyColors(hs.m.deps.Config.RoleColors(), true)
	})
	h.send(tea.BackgroundColorMsg{Color: bg})
	h.toAlbums()
	return h
}

// The reported bug: on catppuccin-mocha the Visual block was the
// terminal's black (#45475a there) on the selection (#45475a), 1:1.
func TestBarModeReadsOnTheThemesSelection(t *testing.T) {
	h := themeHarness(t, "catppuccin-mocha", lipgloss.Color("#1e1e2e"))
	h.keys("3j<CR>j<CR>V")
	line := lineWith(t, h.m.render(), " VISUAL ")
	fg, bg := drawnAs(t, line, " VISUAL ")
	if !strings.HasPrefix(fg, "38;2;") || !strings.HasPrefix(bg, "48;2;") {
		t.Fatalf("the Visual block should be exact colours, is fg %q bg %q: %q", fg, bg, line)
	}
	if r := contrast(exactOf(t, fg), exactOf(t, bg)); r < minContrast {
		t.Errorf("the Visual block's contrast is %.1f, want %.1f: %q", r, minContrast, line)
	}
}

// Every role of every theme, on a dark and a light terminal: the mode,
// the middle and the title read.
func TestBarReadsForEveryTheme(t *testing.T) {
	for _, name := range config.ThemeNames() {
		for _, bg := range []string{"#1e1e2e", "#fafafa"} {
			h := themeHarness(t, name, lipgloss.Color(bg))
			h.keys("3j<CR>j<CR>")
			for _, step := range []struct{ keys, mode string }{
				{"", " NORMAL "}, {"V", " VISUAL "}, {"<Esc>:", " COMMAND "},
			} {
				if step.keys != "" {
					h.keys(step.keys)
				}
				line := lineWith(t, h.m.render(), step.mode)
				pos := fmt.Sprintf("/%d ", len(h.m.state.Queue))
				// The title fades letter by letter: "w" and "d" are its own.
				for _, part := range []string{step.mode, pos, " ▶ ", "w", "d"} {
					fg, back := drawnAs(t, line, part)
					if back == "" {
						back = "48;2;" + strings.Join(rgbParams(bg), ";")
					}
					if r := contrast(exactOf(t, fg), exactOf(t, back)); r < minContrast {
						t.Errorf("%s on %s, %q in %s: contrast %.1f", name, bg, part, step.mode, r)
					}
				}
			}
			h.keys("<Esc>")
		}
	}
}

func rgbParams(hex string) []string {
	c, _ := colorful.Hex(hex)
	r, g, b := c.RGB255()
	return []string{strconv.Itoa(int(r)), strconv.Itoa(int(g)), strconv.Itoa(int(b))}
}

// Any colour at all, a cover's say, on any background: the ink, the middle
// and the title keep minContrast.
func TestBarInkForAnyColour(t *testing.T) {
	m := &Model{deps: Deps{TrueColor: true}}
	var roles []colorful.Color
	for _, l := range []float64{0, 0.15, 0.3, 0.45, 0.55, 0.6, 0.7, 0.85, 1} {
		for h := 0.0; h < 360; h += 20 {
			for _, c := range []float64{0, 0.1, 0.25} {
				roles = append(roles, colorful.OkLch(l, c, h).Clamped())
			}
		}
	}
	for _, bg := range []string{"#000000", "#1e1e2e", "#002b36", "#777777", "#eff1f5", "#ffffff"} {
		m.covers.bg = lipgloss.Color(bg)
		back := m.background()
		for _, role := range roles {
			k := m.barInkFor(role)
			ink, _ := colorful.MakeColor(k.ink)
			text, _ := colorful.MakeColor(k.text)
			mid, _ := colorful.MakeColor(k.mid)
			title, _ := colorful.MakeColor(k.title)
			for part, r := range map[string]float64{
				"mode":  contrast(ink, role),
				"mid":   contrast(text, mid),
				"title": contrast(title, back),
			} {
				if r < minContrast {
					t.Errorf("%s on %s: %s contrast %.2f", role.Hex(), bg, part, r)
				}
			}
		}
	}
}

func TestPaletteReply(t *testing.T) {
	for in, want := range map[string]string{
		"\x1b]4;5;rgb:cdcd/0000/cdcd\x07":    "5 #cd00cd",
		"\x1b]4;15;rgb:ffff/ffff/ffff\x1b\\": "15 #ffffff",
		"\x1b]4;0;rgb:fb/f1/c7\x07":          "0 #fbf1c7",
	} {
		n, c, ok := paletteReply(in)
		cf, _ := colorful.MakeColor(c)
		if got := strconv.Itoa(int(n)) + " " + cf.Hex(); !ok || got != want {
			t.Errorf("paletteReply(%q) = %s, %v; want %s", in, got, ok, want)
		}
	}
	for _, in := range []string{
		"\x1b]11;rgb:0000/0000/0000\x07", "\x1b]4;16;rgb:0/0/0\x07", "\x1b]4;3;nonsense\x07", "\x1b]4;x\x07",
	} {
		if _, _, ok := paletteReply(in); ok {
			t.Errorf("paletteReply(%q) should not be a palette colour", in)
		}
	}
}

// gruvboxLight is gruvbox-light's palette as a terminal reports it: its
// black is the background, so black ink vanished.
var gruvboxLight = map[int]string{
	0: "rgb:fbfb/f1f1/c7c7", 3: "rgb:d7d7/9999/2121", 5: "rgb:b1b1/6262/8686",
	7: "rgb:7c7c/6f6f/6464", 8: "rgb:9292/8383/7474", 15: "rgb:3c3c/3838/3636",
}

func reportPalette(h *harness, pal map[int]string) {
	for n, spec := range pal {
		h.send(uv.UnknownOscEvent("\x1b]4;" + strconv.Itoa(n) + ";" + spec + "\x07"))
	}
}

// With one of the 16 colours on a terminal without exact colours, the ink
// is the reported colour that reads best, not black.
func TestBarInkFromTheReportedPalette(t *testing.T) {
	h := newHarness(t, 120, 40, withColors(config.Colors{Playing: "3"}))
	reportPalette(h, gruvboxLight)
	h.keys("3j<CR>j<CR>")
	fg, _ := drawnAs(t, lineWith(t, h.m.render(), " NORMAL "), " NORMAL ")
	if fg != "97" { // bright white: gruvbox-light's darkest
		t.Errorf("the Normal block's ink is %q, want 97 (bright white)", fg)
	}
}

// On a true-colour terminal, a reported colour is measured like a hex one.
func TestReportedPaletteColoursAreExact(t *testing.T) {
	h := accentHarness(t, "5")
	h.send(tea.BackgroundColorMsg{Color: lipgloss.Color("#fbf1c7")})
	before, _ := drawnAs(t, lineWith(t, h.m.render(), "Low Tide"), "Low Tide")
	reportPalette(h, gruvboxLight)
	line := lineWith(t, h.m.render(), " NORMAL ")
	fg, bg := drawnAs(t, line, " NORMAL ")
	if bg != "45" || !strings.HasPrefix(fg, "38;2;") {
		t.Fatalf("the block should stay magenta with exact ink: fg %q bg %q", fg, bg)
	}
	title, _ := drawnAs(t, line, "Low Tide")
	if before == title || !strings.HasPrefix(title, "38;2;") {
		t.Errorf(
			"the title should be the reported magenta, readable: before %q, after %q",
			before,
			title,
		)
	}
	if r := contrast(exactOf(t, title), hx("#fbf1c7")); r < minContrast {
		t.Errorf("the title's contrast is %.1f", r)
	}
}

func hx(s string) colorful.Color { c, _ := colorful.Hex(s); return c }

func TestStartAsksForThePalette(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	q := strings.Join(h.queries, "")
	for _, n := range []string{"0", "8", "15"} {
		if !strings.Contains(q, "\x1b]4;"+n+";?\x07") {
			t.Errorf("no query for colour %s in %q", n, q)
		}
	}
}
