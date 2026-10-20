package ui

import (
	"image/color"
	"strconv"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/config"
)

// The four roles a user can recolour; the rest of the look is attributes.
// Each role has one meaning: playing, a key, an error, the Visual
// selection. The defaults are the terminal's own palette.
var (
	playingColor   color.Color = ansi.Magenta
	keyColor       color.Color = ansi.Cyan
	errorColor     color.Color = ansi.Red
	selectionColor color.Color = ansi.Blue
)

// applyColors puts the config's colours on the roles, and the defaults on
// those it leaves out. Hex colours are down-sampled by the renderer to
// what the terminal shows; NO_COLOR removes all of them.
func applyColors(c config.Colors) {
	pick := func(v string, def color.Color) color.Color {
		switch {
		case v == "":
			return def
		case v[0] == '#':
			return lipgloss.Color(v)
		}
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return ansi.BasicColor(n)
	}
	playingColor = pick(c.Playing, ansi.Magenta)
	keyColor = pick(c.Key, ansi.Cyan)
	errorColor = pick(c.Error, ansi.Red)
	selectionColor = pick(c.Selection, ansi.Blue)

	// The selection keeps its underline, so a colour close to the text's
	// can't hide it, and it shows without colour.
	visualStyle = lipgloss.NewStyle().Background(selectionColor).Underline(true)
	keyStyle = lipgloss.NewStyle().Foreground(keyColor).Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(playingColor).Bold(true)
	errStyle = lipgloss.NewStyle().Foreground(errorColor).Bold(true)
	focusedInput = lipgloss.NewStyle().Foreground(keyColor)
}
