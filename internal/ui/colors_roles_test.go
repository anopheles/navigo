package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func withColors(c config.Colors) func(*config.Config, *subsonictest.Server) {
	return func(cfg *config.Config, _ *subsonictest.Server) { cfg.Colors = c }
}

// lineWith is the rendered line that holds text.
func lineWith(t *testing.T, screen, text string) string {
	t.Helper()
	for _, l := range strings.Split(screen, "\n") {
		if strings.Contains(ansi.Strip(l), text) {
			return l
		}
	}
	t.Fatalf("no line with %q in\n%s", text, screen)
	return ""
}

func TestRolesTakeTheConfiguredColours(t *testing.T) {
	h := newHarness(t, 120, 40, withColors(config.Colors{
		Playing: "#ff9e64", Key: "2", Error: "#f7768e", Selection: "8",
	}))
	h.keys("3j<CR>j<CR>") // Low Tide plays
	screen := h.m.render()
	if l := lineWith(t, screen, "▶"); !strings.Contains(l, "38;2;255;158;100") {
		t.Errorf("playing row has no orange: %q", l)
	}
	if l := lineWith(t, screen, "<CR>"); !strings.Contains(l, "32") { // green, a number
		t.Errorf("hint line keys aren't green: %q", l)
	}
	h.keys(
		"Vj",
	) // Low Tide selected, the cursor on the next row
	if l := lineWith(t, h.m.render(), "Low Tide"); !has(l, 100) { // bright black background
		t.Errorf("selection isn't gray: %q", l)
	}
}

func TestRolesLeftOutKeepTheTerminalPalette(t *testing.T) {
	h := newHarness(t, 120, 40, withColors(config.Colors{Playing: "#ff9e64"}))
	h.keys("3j<CR>j<CR>")
	if l := lineWith(t, h.m.render(), "<CR>"); !has(l, 36) { // cyan
		t.Errorf("keys lost their default: %q", l)
	}
}

func TestSelectionStaysUnderlined(t *testing.T) {
	h := newHarness(t, 120, 40, withColors(config.Colors{Selection: "0"}))
	h.keys("Vj") // the first row is selected, the cursor is on the second
	if l := lineWith(t, h.m.render(), "Ångström"); !has(l, 4) {
		t.Errorf("the selection isn't underlined: %q", l)
	}
}

func TestNoColourStillStripsRecolouredRoles(t *testing.T) {
	h := newHarness(t, 120, 40, withColors(config.Colors{
		Playing: "#ff9e64", Key: "#7dcfff", Error: "1", Selection: "#3b4261",
	}))
	h.keys("3j<CR>j<CR>V")
	var b strings.Builder
	w := colorprofile.NewWriter(&b, nil)
	w.Profile = colorprofile.ASCII
	if _, err := w.WriteString(h.m.render()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "38;") || strings.Contains(b.String(), "48;") {
		t.Error("a colour survived NO_COLOR")
	}
}

func TestReloadConfigRecolours(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(s string) {
		if err := os.WriteFile(h.path, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	h.keys("3j<CR>j<CR>")
	write("colors:\n  playing: '#ff9e64'\n")
	h.keys(":reload-config<CR>")
	if l := lineWith(t, h.m.render(), "▶"); !strings.Contains(l, "38;2;255;158;100") {
		t.Errorf("not recoloured: %q", l)
	}
	write("# no colours\n")
	h.keys(":reload-config<CR>")
	if l := lineWith(t, h.m.render(), "▶"); !has(l, 35) {
		t.Errorf("the default didn't come back: %q", l)
	}
}

func TestColourProblemsShow(t *testing.T) {
	cfg, err := config.Parse([]byte("colors:\n  playing: greenish\n"))
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(
		t,
		120,
		40,
		func(c *config.Config, _ *subsonictest.Server) { c.Colors = cfg.Colors },
	)
	h.contains("config.yaml has problems", "config.yaml:2:12: colors.playing")
}

func TestThemeColoursTheRoles(t *testing.T) {
	cfg, err := config.Parse([]byte("theme: catppuccin-mocha\ncolors:\n  key: red\n"))
	if err != nil {
		t.Fatal(err)
	}
	h := newHarness(
		t,
		120,
		40,
		func(c *config.Config, _ *subsonictest.Server) { c.Theme, c.Colors = cfg.Theme, cfg.Colors },
	)
	h.keys("3j<CR>j<CR>")
	screen := h.m.render()
	if l := lineWith(t, screen, "▶"); !strings.Contains(l, "38;2;203;166;247") { // mauve
		t.Errorf("playing isn't the theme's mauve: %q", l)
	}
	if l := lineWith(t, screen, "<CR>"); !has(l, 31) { // colors: overrides the theme
		t.Errorf("keys aren't red: %q", l)
	}
}

func TestReloadConfigChangesTheTheme(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte("theme: dracula\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.keys("3j<CR>j<CR>:reload-config<CR>")
	if l := lineWith(t, h.m.render(), "▶"); !strings.Contains(l, "38;2;255;121;198") { // pink
		t.Errorf("not dracula: %q", l)
	}
}

// distinctFills counts the colours the progress bar's ━ cells are drawn in.
func distinctFills(t *testing.T, screen string) int {
	t.Helper()
	colours := map[string]bool{}
	for _, m := range regexp.MustCompile(`\x1b\[([0-9;]*)m━`).FindAllStringSubmatch(screen, -1) {
		colours[m[1]] = true
	}
	return len(colours)
}

func TestProgressFillFadesOnlyForHexPlaying(t *testing.T) {
	play := func(h *harness) {
		h.keys("3j<CR>j<CR>")
		h.fake.Tick(42 * time.Second)
		h.pump()
	}
	h := newHarness(t, 120, 40, withColors(config.Colors{Playing: "#ff9e64"}))
	play(h)
	if n := distinctFills(t, h.m.render()); n < 3 {
		t.Errorf("a hex playing colour left the fill in %d colours, want a fade", n)
	}
	h = newHarness(t, 120, 40, nil)
	play(h)
	screen := h.m.render()
	if n := distinctFills(t, screen); n != 1 {
		t.Errorf("the terminal's palette drew the fill in %d colours, want 1", n)
	}
	if bar := ansi.Strip(lineWith(t, screen, "━")); !strings.Contains(bar, "━") ||
		!strings.Contains(bar, "─") {
		t.Errorf("the progress bar is not filled up to a rest:\n%s", bar)
	}
}

func TestTrueColorTerminalsGetAFadingPlayingColourByDefault(t *testing.T) {
	for _, c := range []struct {
		name      string
		trueColor bool
		colors    config.Colors
		fades     bool
	}{
		{"true colour, nothing set", true, config.Colors{}, true},
		{"16 colours, nothing set", false, config.Colors{}, false},
		{"true colour, playing set to an ANSI colour", true, config.Colors{Playing: "5"}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, 120, 40, func(cfg *config.Config, _ *subsonictest.Server) {
				cfg.Colors = c.colors
			})
			h.m.deps.TrueColor = c.trueColor
			applyColors(h.m.deps.Config.RoleColors(), c.trueColor)
			h.keys("3j<CR>j<CR>")
			h.fake.Tick(42 * time.Second)
			h.pump()
			if got := distinctFills(t, h.m.render()) > 1; got != c.fades {
				t.Errorf("fade = %v, want %v", got, c.fades)
			}
		})
	}
}

// accentHarness starts navigo on a true-colour terminal with a hex playing
// colour, so the accent fades are drawn, and plays Low Tide.
func accentHarness(t *testing.T, playing string) *harness {
	t.Helper()
	h := setupHarness(t, 120, 40, withColors(config.Colors{Playing: playing}), func(hs *harness) {
		hs.m.deps.TrueColor = true
	})
	h.toAlbums()
	// Low Tide plays; the album opens again from Search, which shows the
	// queue beside it, with the cursor on Low Tide.
	h.keys("3j<CR>j<CR>:search blue hour<CR><CR>j")
	return h
}

func TestAccentFadesTheFocusedTitleAndFrame(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	top := strings.SplitN(h.m.render(), "\n", 2)[0]
	title := strings.SplitN(top, "Queue", 2)[0] // the focused library title
	if n := len(distinct(sgrs(title), "38;2;")); n < 4 {
		t.Errorf("the focused title has %d colours, want a fade: %q", n, title)
	}
	if !strings.Contains(top, "38;2;") ||
		strings.Contains(strings.SplitN(top, "╭", 2)[1][:30], ";7m") {
		t.Errorf("the focused title is still reversed: %q", top)
	}
	h.keys("<Tab>")
	top = strings.SplitN(h.m.render(), "\n", 2)[0]
	lib, queue, _ := strings.Cut(top, "╮") // the library's frame is faint now
	if !strings.Contains(lib, "\x1b[2m") || strings.Contains(lib, "38;2;") ||
		!strings.Contains(queue, "38;2;") {
		t.Errorf("the frames did not follow the focus: %q", top)
	}
}

func TestAccentTintsTheCursorAndFadesThePlayingRow(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	h.keys("<Tab>") // the queue is focused, its cursor on Dusk Signal
	screen := h.m.render()
	cursor := lineWith(t, screen, "Dusk Signal")
	if has(cursor, 7) || !strings.Contains(cursor, "48;2;") {
		t.Errorf("the cursor row should be a tint, not reverse: %q", cursor)
	}
	playing := lineWith(t, screen, "▶")
	if n := len(distinct(sgrs(playing), "38;2;")); n < 5 {
		t.Errorf("the playing row has %d colours, want a fade: %q", n, playing)
	}
}

func TestAccentTintsTheOtherWindowsCursorFainter(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	h.keys("<Tab>") // the library keeps its cursor, out of focus
	it, ok := h.m.rowItem(h.m.top(), h.m.top().cur)
	if !ok {
		t.Fatal("the library's cursor isn't on an item")
	}
	lib, _, _ := strings.Cut(lineWith(t, h.m.render(), it.name), "││")
	r, g, b, _ := h.m.restTint().RGBA()
	rest := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
	if !strings.Contains(lib, rest) || has(lib, 4) || strings.Contains(lib, "▎") {
		t.Errorf("the other window's cursor should be the faint tint %s: %q", rest, lib)
	}
	if fmt.Sprint(h.m.restTint()) == fmt.Sprint(h.m.tint()) {
		t.Error("the resting tint is as strong as the focused one")
	}
}

func TestAccentOnLightAndDarkTerminals(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	h.keys("<Tab>")
	dark := h.m.tint()
	h.send(tea.BackgroundColorMsg{Color: color.RGBA{250, 250, 250, 255}})
	light := h.m.tint()
	dr, _, _, _ := dark.RGBA()
	lr, _, _, _ := light.RGBA()
	if lr <= dr {
		t.Errorf("the tint should follow the background: dark %v, light %v", dark, light)
	}
}

func TestNoAccentFadesWithoutExactColours(t *testing.T) {
	plain := setupHarness(t, 120, 40, withColors(config.Colors{Playing: "#c25bd8"}), nil)
	plain.toAlbums()
	plain.keys("3j<CR>j<CR>")
	for name, h := range map[string]*harness{
		"playing is the terminal's magenta": accentHarness(t, "5"),
		"not a true-colour terminal":        plain,
	} {
		h.keys("<Tab>")
		screen := h.m.render()
		if q := lineWith(t, screen, "Dusk Signal"); !has(q, 7) {
			t.Errorf("%s: the cursor row lost its reverse: %q", name, q)
		}
		top := strings.SplitN(screen, "\n", 2)[0]
		if len(distinct(sgrs(top), "38;2;")) > 1 && name != "not a true-colour terminal" {
			t.Errorf("%s: the title fades: %q", name, top)
		}
	}
}

func TestAccentFrom(t *testing.T) {
	if accentFrom(ansi.Magenta) != nil {
		t.Error("the terminal's magenta has no exact value to fade from")
	}
	stops := accentFrom(lipgloss.Color("#c25bd8"))
	if len(stops) != 3 {
		t.Fatalf("stops = %v", stops)
	}
	r0, g0, b0, _ := stops[0].RGBA()
	r2, g2, b2, _ := stops[2].RGBA()
	if r0 == r2 && g0 == g2 && b0 == b2 || b2 <= r2 {
		t.Errorf(
			"the last stop should be bluer than the playing colour: %v → %v",
			stops[0],
			stops[2],
		)
	}
}

// distinct is the SGRs that start with prefix, without repeats.
func distinct(all []string, prefix string) []string {
	var out []string
	for _, s := range all {
		if strings.HasPrefix(s, prefix) && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

func TestAccentTintsTheSelectionInATurnedHue(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	h.keys("Vj") // the first row is selected, the cursor is on the second
	it, ok := h.m.rowItem(h.m.top(), h.m.top().cur-1)
	if !ok {
		t.Fatal("the selected row isn't an item")
	}
	sel := lineWith(t, h.m.render(), it.name)
	r, g, b, _ := h.m.selectionTint().RGBA()
	want := fmt.Sprintf("48;2;%d;%d;%d", r>>8, g>>8, b>>8)
	if !strings.Contains(sel, want) || has(sel, 4) || has(sel, 44) {
		t.Errorf("the selection should be the tint %s, without underline: %q", want, sel)
	}
	if fmt.Sprint(h.m.selectionTint()) == fmt.Sprint(h.m.tint()) {
		t.Error("the selection looks like the cursor")
	}
	h.send(tea.BackgroundColorMsg{Color: color.RGBA{250, 250, 250, 255}})
	lr, _, _, _ := h.m.selectionTint().RGBA()
	if lr <= r {
		t.Errorf("the selection tint should follow the background: dark %d, light %d", r>>8, lr>>8)
	}
}

func TestASetSelectionColourIsNotTinted(t *testing.T) {
	colors := config.Colors{Playing: "#c25bd8", Selection: "#45475a"}
	h := setupHarness(t, 120, 40, withColors(colors), func(hs *harness) {
		hs.m.deps.TrueColor = true
	})
	h.toAlbums()
	h.keys("3j<CR>Vj")
	it, _ := h.m.rowItem(h.m.top(), h.m.top().cur-1)
	l := lineWith(t, h.m.render(), it.name)
	if !strings.Contains(l, "48;2;69;71;90") || !has(l, 4) {
		t.Errorf("the configured selection should stay, underlined: %q", l)
	}
}
