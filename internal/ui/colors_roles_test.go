package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	h.keys("V")
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
