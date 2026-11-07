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

// selectionTinted is whether the Visual selection is a tint of the accent:
// no theme and no colors: entry sets the selection role.
var selectionTinted bool

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
	selectionTinted = c.Selection == ""

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
func (m *Model) accent(s string) string { return m.accentOver(s, nil) }

// accentOver is accent on the background bg, or the terminal's when nil.
func (m *Model) accentOver(s string, bg color.Color) string {
	if !m.accentOn() || s == "" || strings.Contains(s, "\x1b") {
		st := accentStyle
		if bg != nil {
			st = st.Background(bg)
		}
		return st.Render(s)
	}
	cs := lipgloss.Blend1D(lipgloss.Width(s), accentStops...)
	var b strings.Builder
	i := 0
	for _, r := range s {
		st := ansi.NewStyle().ForegroundColor(cs[min(i, len(cs)-1)]).Bold()
		if bg != nil {
			st = st.BackgroundColor(bg)
		}
		b.WriteString(st.Styled(string(r)))
		i += lipgloss.Width(string(r))
	}
	return b.String()
}

// Contrast the cursor row's background keeps against the terminal's: the
// focused window's shows at a glance, the other window's still shows.
const (
	tintContrast     = 1.6
	restTintContrast = 1.25
)

// tint is the cursor row's background in the focused window: the accent
// over the terminal's own background, which is black until the terminal
// has said, as little as reaches tintContrast and at least a fifth.
func (m *Model) tint() color.Color { return m.tintTo(5, tintContrast) }

// restTint is the cursor row's background in the other window: the same
// cursor at rest, at least a tenth of the accent.
func (m *Model) restTint() color.Color { return m.tintTo(10, restTintContrast) }

// tintTo is 1/part of the accent over the background, or more of it until
// the mix has want contrast against the background.
func (m *Model) tintTo(part uint32, want float64) color.Color {
	bg := colorful.Color{}
	if m.covers.bg != nil {
		bg, _ = colorful.MakeColor(m.covers.bg)
	}
	acc, _ := colorful.MakeColor(accentStops[0])
	mix := drawn(bg.BlendRgb(acc, 1/float64(part)))
	for f := 1 / float64(part); f <= 1 && contrast(mix, bg) < want; f += 0.02 {
		mix = drawn(bg.BlendRgb(acc, f))
	}
	r, g, b := mix.RGB255()
	return color.RGBA{r, g, b, 255}
}

// cursorInk is the colour of the ▎ that marks the cursor row: the accent,
// moved until it reads on the background, so the cursor shows by its mark
// and not by its background alone.
func (m *Model) cursorInk() color.Color {
	bg := colorful.Color{}
	if m.covers.bg != nil {
		bg, _ = colorful.MakeColor(m.covers.bg)
	}
	acc, _ := colorful.MakeColor(accentStops[0])
	r, g, b := readable(acc, bg).RGB255()
	return color.RGBA{r, g, b, 255}
}

// selectionTint is the Visual selection's background when it follows the
// accent: a third of the accent's last hue, turned 75° from the cursor's,
// so a selection and the cursor in it stay apart.
func (m *Model) selectionTint() color.Color { return m.mixOver(accentStops[len(accentStops)-1], 3) }

// mixOver is 1/part of c over the terminal's background, which is black
// until the terminal has said.
func (m *Model) mixOver(c color.Color, part uint32) color.Color {
	var br, bg, bb uint32
	if m.covers.bg != nil {
		br, bg, bb, _ = m.covers.bg.RGBA()
	}
	ar, ag, ab, _ := c.RGBA()
	mix := func(back, acc uint32) uint8 { return uint8((back>>8)*(part-1)/part + (acc>>8)/part) }
	return color.RGBA{mix(br, ar), mix(bg, ag), mix(bb, ab), 255}
}
