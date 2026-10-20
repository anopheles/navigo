package ui

import (
	"image/color"
	"math"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"

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

// accentStops fade from the playing colour towards blue, for the focused
// window's frame and title and the playing row. They are nil while playing
// is one of the terminal's own colours, which have no exact value to blend.
var accentStops []color.Color

// defaultPlaying is the playing colour on a true-colour terminal when the
// config sets none, or until the cover has given one: a mid-tone magenta that reads on light and dark
// backgrounds, and that the progress bar can fade up to. Elsewhere the
// terminal's own magenta stays.
const defaultPlaying = "#c25bd8"

// applyColors puts the config's colours on the roles, and the defaults on
// those it leaves out. Hex colours are down-sampled by the renderer to
// what the terminal shows; NO_COLOR removes all of them.
func applyColors(c config.Colors, trueColor bool) {
	if c.Playing == "" || c.Playing == config.CoverColor { // the cover's colour comes later
		c.Playing = ""
		if trueColor {
			c.Playing = defaultPlaying
		}
	}
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
	accentStops = accentFrom(playingColor)
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

// accentFrom is c and two hues turned towards blue; nil for the terminal's
// own colours.
func accentFrom(c color.Color) []color.Color {
	if _, basic := c.(ansi.BasicColor); basic {
		return nil
	}
	base, ok := colorful.MakeColor(c)
	if !ok {
		return nil
	}
	h, s, l := base.Hsl()
	turn := func(d float64) color.Color { return colorful.Hsl(math.Mod(h-d+360, 360), s, l) }
	return []color.Color{c, turn(35), turn(75)}
}

// accentOn reports whether the accent fades are drawn: the playing colour
// is an exact one and the terminal shows 24-bit colour with colours on.
func (m *Model) accentOn() bool { return len(accentStops) > 0 && m.deps.TrueColor }

// accent renders s in the accent: faded across the text when the fades are
// on, else in the playing colour.
func (m *Model) accent(s string) string {
	if !m.accentOn() || s == "" || strings.Contains(s, "\x1b") {
		return accentStyle.Render(s)
	}
	cs := lipgloss.Blend1D(lipgloss.Width(s), accentStops...)
	var b strings.Builder
	i := 0
	for _, r := range s {
		b.WriteString(
			ansi.NewStyle().ForegroundColor(cs[min(i, len(cs)-1)]).Bold().Styled(string(r)),
		)
		i += lipgloss.Width(string(r))
	}
	return b.String()
}

// tint is the cursor row's background: a fifth of the accent over the
// terminal's own background, which is black until the terminal has said.
func (m *Model) tint() color.Color {
	var br, bg, bb uint32
	if m.covers.bg != nil {
		br, bg, bb, _ = m.covers.bg.RGBA()
	}
	ar, ag, ab, _ := accentStops[0].RGBA()
	mix := func(back, acc uint32) uint8 { return uint8((back>>8)*4/5 + (acc>>8)/5) }
	return color.RGBA{mix(br, ar), mix(bg, ag), mix(bb, ab), 255}
}
