package config

import (
	"slices"
	"strings"
	"testing"
)

func TestEveryThemeHasFourReadableHexColours(t *testing.T) {
	for _, name := range ThemeNames() {
		th := themes[name]
		for role, v := range map[string]string{"playing": th.Playing, "key": th.Key, "error": th.Error, "selection": th.Selection} {
			if got, err := ParseColor(v); err != nil || got != v || !strings.HasPrefix(v, "#") {
				t.Errorf("%s %s = %q (%q, %v): want lower-case #rrggbb", name, role, v, got, err)
			}
		}
		// A role needs its own colour to carry its own meaning.
		if th.Playing == th.Key || th.Playing == th.Error || th.Key == th.Error {
			t.Errorf("%s gives two roles the same colour: %+v", name, th)
		}
	}
}

func TestTheNamedThemes(t *testing.T) {
	names := ThemeNames()
	for _, want := range []string{
		"catppuccin-latte", "catppuccin-frappe", "catppuccin-macchiato", "catppuccin-mocha",
		"gruvbox-dark", "gruvbox-light", "nord", "dracula", "solarized-dark", "solarized-light",
		"tokyo-night", "one-dark", "rose-pine", "kanagawa", "monokai",
	} {
		if !slices.Contains(names, want) {
			t.Errorf("no theme %q", want)
		}
	}
	if !slices.IsSorted(names) {
		t.Errorf("names are not sorted: %v", names)
	}
}

func TestThemeAppliesAndColorsOverride(t *testing.T) {
	c, err := Parse([]byte("theme: Catppuccin-Mocha\n"))
	if err != nil || len(c.Problems()) != 0 {
		t.Fatalf("%v %v", err, c.Problems())
	}
	if g := c.RoleColors(); g.Playing != "#cba6f7" || g.Key != "#89dceb" || g.Error != "#f38ba8" ||
		g.Selection != "#45475a" {
		t.Errorf("RoleColors = %+v", g)
	}
	c, err = Parse([]byte("theme: nord\ncolors:\n  playing: green\n  selection: '#112233'\n"))
	if err != nil {
		t.Fatal(err)
	}
	got := c.RoleColors()
	if got.Playing != "2" || got.Selection != "#112233" || got.Key != "#88c0d0" ||
		got.Error != "#bf616a" {
		t.Errorf("RoleColors = %+v", got)
	}
	// With no theme the roles the user left out stay empty: the default.
	c, _ = Parse([]byte("colors:\n  key: red\n"))
	if got := c.RoleColors(); got.Key != "1" || got.Playing != "" {
		t.Errorf("RoleColors = %+v", got)
	}
}

func TestUnknownTheme(t *testing.T) {
	c, err := Parse([]byte("theme: catppuccin\ncolors:\n  key: red\n"))
	if err != nil {
		t.Fatal(err)
	}
	ps := c.Problems()
	if len(ps) != 1 || ps[0].Line != 1 || ps[0].Col != 8 ||
		!strings.HasPrefix(
			ps[0].Msg,
			`theme: "catppuccin" is not a theme; the themes are catppuccin-frappe, `,
		) {
		t.Fatalf("problems = %+v", ps)
	}
	if c.Theme.Name != "" || c.RoleColors().Key != "1" {
		t.Errorf("a bad theme changed something: %+v", c.RoleColors())
	}
}
