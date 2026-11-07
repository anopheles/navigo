package ui

import (
	"image/color"
	"strings"
	"testing"

	"github.com/lucasb-eyer/go-colorful"

	"github.com/anopheles/navigo/internal/config"
)

// The cursor row's tint shows against the background for every theme, on
// dark and light terminals, and the focused cursor's ▎ reads (#147).
func TestCursorTintsShowForEveryTheme(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	defer applyColors(config.Colors{}, false)
	backgrounds := []color.Color{
		color.RGBA{0, 0, 0, 255},
		color.RGBA{30, 30, 46, 255},    // catppuccin-mocha's base
		color.RGBA{251, 241, 199, 255}, // gruvbox-light's
		color.RGBA{255, 255, 255, 255},
	}
	for _, name := range append(config.ThemeNames(), "") {
		applyColors(config.Config{Theme: config.Theme{Name: name}}.RoleColors(), true)
		for _, bg := range backgrounds {
			h.m.covers.bg = bg
			b, _ := colorful.MakeColor(bg)
			at := func(c color.Color) colorful.Color { x, _ := colorful.MakeColor(c); return x }
			if c := contrast(at(h.m.tint()), b); c < tintContrast {
				t.Errorf(
					"%q on %v: the cursor tint has contrast %.2f, want %.2f",
					name,
					bg,
					c,
					tintContrast,
				)
			}
			if c := contrast(at(h.m.restTint()), b); c < restTintContrast {
				t.Errorf(
					"%q on %v: the resting tint has contrast %.2f, want %.2f",
					name,
					bg,
					c,
					restTintContrast,
				)
			}
			if c := contrast(at(h.m.cursorInk()), b); c < minContrast {
				t.Errorf(
					"%q on %v: the cursor mark has contrast %.2f, want %.2f",
					name,
					bg,
					c,
					minContrast,
				)
			}
		}
	}
}

// The focused cursor row has a ▎ mark, and the playing row keeps its
// accent under the cursor.
func TestFocusedCursorIsMarkedAndKeepsThePlayingColour(t *testing.T) {
	h := accentHarness(t, "#c25bd8")
	h.keys("<Tab>gp") // the queue, its cursor on the playing Low Tide
	row := lineWith(t, h.m.render(), "Low Tide")
	if strings.Count(row, "38;2;") < 5 {
		t.Errorf("the playing row lost its fade under the cursor: %q", row)
	}
	h.keys("<Tab>") // the library, its cursor on Low Tide
	lib, _, _ := strings.Cut(lineWith(t, h.m.render(), "2 Low Tide"), "││")
	if !strings.Contains(lib, "▎") {
		t.Errorf("the focused cursor row has no ▎: %q", lib)
	}
}
