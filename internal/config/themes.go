package config

import (
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// themes are the named palettes `theme:` can pick, each a colour for the
// four roles in the terminal-palette-independent #rrggbb form. They assume
// the terminal itself uses that theme: the selection is a background that
// reads against the theme's own text.
var themes = map[string]Colors{
	"catppuccin-latte": {
		Playing:   "#8839ef",
		Key:       "#04a5e5",
		Error:     "#d20f39",
		Selection: "#bcc0cc",
	},
	"catppuccin-frappe": {
		Playing:   "#ca9ee6",
		Key:       "#99d1db",
		Error:     "#e78284",
		Selection: "#51576d",
	},
	"catppuccin-macchiato": {
		Playing:   "#c6a0f6",
		Key:       "#91d7e3",
		Error:     "#ed8796",
		Selection: "#494d64",
	},
	"catppuccin-mocha": {
		Playing:   "#cba6f7",
		Key:       "#89dceb",
		Error:     "#f38ba8",
		Selection: "#45475a",
	},
	"gruvbox-dark": {
		Playing:   "#d3869b",
		Key:       "#8ec07c",
		Error:     "#fb4934",
		Selection: "#504945",
	},
	"gruvbox-light": {
		Playing:   "#8f3f71",
		Key:       "#427b58",
		Error:     "#9d0006",
		Selection: "#d5c4a1",
	},
	"nord": {
		Playing:   "#b48ead",
		Key:       "#88c0d0",
		Error:     "#bf616a",
		Selection: "#434c5e",
	},
	"dracula": {
		Playing:   "#ff79c6",
		Key:       "#8be9fd",
		Error:     "#ff5555",
		Selection: "#44475a",
	},
	"solarized-dark": {
		Playing:   "#d33682",
		Key:       "#2aa198",
		Error:     "#dc322f",
		Selection: "#073642",
	},
	"solarized-light": {
		Playing:   "#d33682",
		Key:       "#2aa198",
		Error:     "#dc322f",
		Selection: "#eee8d5",
	},
	"tokyo-night": {
		Playing:   "#bb9af7",
		Key:       "#7dcfff",
		Error:     "#f7768e",
		Selection: "#283457",
	},
	"tokyo-night-storm": {
		Playing:   "#bb9af7",
		Key:       "#7dcfff",
		Error:     "#f7768e",
		Selection: "#2e3c64",
	},
	"tokyo-night-day": {
		Playing:   "#9854f1",
		Key:       "#007197",
		Error:     "#f52a65",
		Selection: "#b7c1e3",
	},
	"one-dark": {
		Playing:   "#c678dd",
		Key:       "#56b6c2",
		Error:     "#e06c75",
		Selection: "#3e4451",
	},
	"one-light": {
		Playing:   "#a626a4",
		Key:       "#0184bc",
		Error:     "#e45649",
		Selection: "#e5e5e6",
	},
	"rose-pine": {
		Playing:   "#c4a7e7",
		Key:       "#9ccfd8",
		Error:     "#eb6f92",
		Selection: "#403d52",
	},
	"rose-pine-moon": {
		Playing:   "#c4a7e7",
		Key:       "#9ccfd8",
		Error:     "#eb6f92",
		Selection: "#44415a",
	},
	"rose-pine-dawn": {
		Playing:   "#907aa9",
		Key:       "#56949f",
		Error:     "#b4637a",
		Selection: "#dfdad9",
	},
	"everforest-dark": {
		Playing:   "#d699b6",
		Key:       "#83c092",
		Error:     "#e67e80",
		Selection: "#475258",
	},
	"kanagawa": {
		Playing:   "#957fb8",
		Key:       "#7aa89f",
		Error:     "#e82424",
		Selection: "#223249",
	},
	"monokai": {
		Playing:   "#ae81ff",
		Key:       "#66d9ef",
		Error:     "#f92672",
		Selection: "#49483e",
	},
}

// ThemeNames are the names `theme:` accepts, sorted.
func ThemeNames() []string {
	names := make([]string, 0, len(themes))
	for n := range themes {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// Theme is the theme: setting.
type Theme struct {
	Name     string
	Problems []Problem
}

// UnmarshalYAML reads a theme name; an unknown one is a Problem and leaves
// no theme.
func (t *Theme) UnmarshalYAML(n *yaml.Node) error {
	name := strings.ToLower(strings.TrimSpace(n.Value))
	if _, ok := themes[name]; n.Kind != yaml.ScalarNode || !ok {
		t.Problems = append(t.Problems, Problem{
			pos(n),
			fmt.Sprintf(
				"theme: %q is not a theme; the themes are %s",
				n.Value,
				strings.Join(ThemeNames(), ", "),
			),
		})
		return nil
	}
	t.Name = name
	return nil
}

// RoleColors are the colours for the roles: the theme's, with any role in
// the colors: block replacing the theme's colour for it.
func (c Config) RoleColors() Colors {
	out := themes[c.Theme.Name] // none: all empty
	for _, o := range []struct {
		dst *string
		v   string
	}{
		{&out.Playing, c.Colors.Playing},
		{&out.Key, c.Colors.Key},
		{&out.Error, c.Colors.Error},
		{&out.Selection, c.Colors.Selection},
	} {
		if o.v != "" {
			*o.dst = o.v
		}
	}
	return out
}
