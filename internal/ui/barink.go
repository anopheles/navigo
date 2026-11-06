package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

// minContrast is the WCAG 2 contrast ratio the now-playing bar's text
// keeps against what it is drawn on: the level for body text.
const minContrast = 4.5

// barInk is how the now-playing bar draws one role's colour. The mode and
// the time are blocks of the role with ink on them; the play state and
// the position in the queue are the role's colour as text on mid (nil:
// the terminal's background); the title is in title.
type barInk struct {
	ink       color.Color
	text, mid color.Color
	title     color.Color
}

// barInkFor keeps every colour readable whatever it is. On a true-colour
// terminal the role's exact value is known (a #rrggbb colour, or one of the
// 16 once the terminal has said what they are), so the ink is black or
// white, whichever reads better on it, and where the role is text its
// lightness moves (hue kept) until it reads on what is behind it. Without
// exact values, the ink on the role and on the bright black middle is the
// one of the terminal's black, white, gray and bright white that reads
// best there, with xterm's colours as the guess until the terminal says.
func (m *Model) barInkFor(role color.Color) barInk {
	exact, ok := m.exact(role)
	if !ok {
		return barInk{
			ink:   m.paletteInk(role),
			text:  m.paletteInk(ansi.BrightBlack),
			mid:   ansi.BrightBlack,
			title: role,
		}
	}
	bg := m.background()
	mid := surface(bg)
	return barInk{
		ink:   blackOrWhite(exact),
		text:  readable(exact, mid),
		mid:   mid,
		title: readable(exact, bg),
	}
}

// barTitle is the playing track's title in the bar: in the accent, as
// everywhere, with each of its colours made readable on the background.
func (m *Model) barTitle(s string) string {
	bg := m.background()
	if !m.accentOn() || s == "" || strings.Contains(s, "\x1b") {
		return lipgloss.NewStyle().Foreground(m.barInkFor(playingColor).title).Bold(true).Render(s)
	}
	cs := lipgloss.Blend1D(lipgloss.Width(s), accentStops...)
	var b strings.Builder
	i := 0
	for _, r := range s {
		c, _ := colorful.MakeColor(cs[min(i, len(cs)-1)])
		b.WriteString(ansi.NewStyle().ForegroundColor(readable(c, bg)).Bold().Styled(string(r)))
		i += lipgloss.Width(string(r))
	}
	return b.String()
}

// exact is c's value when the bar may draw exact colours: on a
// true-colour terminal, for a #rrggbb colour or one of the 16 the terminal
// has reported.
func (m *Model) exact(c color.Color) (colorful.Color, bool) {
	if !m.deps.TrueColor {
		return colorful.Color{}, false
	}
	if n, basic := c.(ansi.BasicColor); basic {
		if c = m.palette[n]; c == nil {
			return colorful.Color{}, false
		}
	}
	return colorful.MakeColor(c)
}

// background is the terminal's background, black until it has said.
func (m *Model) background() colorful.Color {
	if m.covers.bg == nil {
		return colorful.Color{}
	}
	c, _ := colorful.MakeColor(m.covers.bg)
	return c
}

// xterm16 is xterm's default palette: the guess for the 16 colours until
// the terminal reports its own.
var xterm16 = [16]string{
	"#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
	"#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
}

// paletteColor is colour n of the terminal's 16: as reported, else xterm's.
func (m *Model) paletteColor(n ansi.BasicColor) colorful.Color {
	if m.palette[n] != nil {
		if c, ok := colorful.MakeColor(m.palette[n]); ok {
			return c
		}
	}
	c, _ := colorful.Hex(xterm16[n])
	return c
}

// paletteInk is the ink for a block of role drawn from the terminal's own
// colours: the one of black, white, gray and bright white with the most
// contrast on it, by the reported colours where there are some.
func (m *Model) paletteInk(role color.Color) color.Color {
	var back colorful.Color
	switch c := role.(type) {
	case ansi.BasicColor:
		back = m.paletteColor(c)
	default:
		back, _ = colorful.MakeColor(c)
	}
	best, most := ansi.Black, 0.0
	for _, n := range []ansi.BasicColor{ansi.Black, ansi.White, ansi.BrightBlack, ansi.BrightWhite} {
		if r := contrast(m.paletteColor(n), back); r > most {
			best, most = n, r
		}
	}
	return best
}

// luminance is c's relative luminance, as WCAG 2 defines it.
func luminance(c colorful.Color) float64 {
	r, g, b := c.Clamped().LinearRgb()
	return 0.2126*r + 0.7152*g + 0.0722*b
}

// contrast is the WCAG 2 contrast ratio of a and b, from 1 to 21.
func contrast(a, b colorful.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

var (
	black = colorful.Color{}
	white = colorful.Color{R: 1, G: 1, B: 1}
)

// blackOrWhite is the one of the two that reads better on c.
func blackOrWhite(c colorful.Color) colorful.Color {
	if contrast(black, c) > contrast(white, c) {
		return black
	}
	return white
}

// surface is the band the bar's middle blocks sit on: the background
// moved 0.12 in OKLCH lightness towards the middle, so it shows on light
// and dark terminals alike.
func surface(bg colorful.Color) colorful.Color {
	l, c, h := bg.OkLch()
	if l < 0.6 {
		l += 0.12
	} else {
		l -= 0.12
	}
	return colorful.OkLch(l, c, h).Clamped()
}

// drawn is c as a terminal draws it: 8 bits a channel, so a contrast
// measured on it holds on screen.
func drawn(c colorful.Color) colorful.Color {
	r, g, b := c.Clamped().RGB255()
	return colorful.Color{R: float64(r) / 255, G: float64(g) / 255, B: float64(b) / 255}
}

// readable is c with its OKLCH lightness moved, away from bg, until it has
// minContrast against bg. The hue stays, and as much of the chroma as sRGB
// holds at that lightness. c comes back as drawn when it reads already.
func readable(c, bg colorful.Color) colorful.Color {
	if c = drawn(c); contrast(c, bg) >= minContrast {
		return c
	}
	lighter := contrast(white, bg) >= contrast(black, bg)
	l, ch, h := c.OkLch()
	for range 100 {
		if lighter {
			l = math.Min(1, l+0.01)
		} else {
			l = math.Max(0, l-0.01)
		}
		next := colorful.OkLch(l, ch, h)
		for k := 0; !next.IsValid() && k < 80; k++ {
			ch *= 0.94
			next = colorful.OkLch(l, ch, h)
		}
		if next = drawn(next); contrast(next, bg) >= minContrast {
			return next
		}
	}
	if lighter {
		return white
	}
	return black
}

// requestPalette asks the terminal for its 16 colours (OSC 4), so the bar
// can measure them; a terminal that doesn't answer leaves the guess.
func requestPalette() tea.Cmd {
	var b strings.Builder
	for n := range 16 {
		fmt.Fprintf(&b, "\x1b]4;%d;?\x07", n)
	}
	return tea.Raw(b.String())
}

// paletteReply reads a terminal's answer to requestPalette:
// ESC ] 4 ; n ; rgb:rrrr/gggg/bbbb, ended by BEL or ST.
func paletteReply(s string) (ansi.BasicColor, color.Color, bool) {
	s = strings.TrimPrefix(s, "\x1b]")
	s = strings.TrimPrefix(s, "\x9d")
	s = strings.TrimSuffix(strings.TrimSuffix(strings.TrimSuffix(s, "\x07"), "\x1b\\"), "\x9c")
	rest, ok := strings.CutPrefix(s, "4;")
	if !ok {
		return 0, nil, false
	}
	num, spec, ok := strings.Cut(rest, ";")
	n, err := strconv.Atoi(num)
	if !ok || err != nil || n < 0 || n > 15 {
		return 0, nil, false
	}
	c := ansi.XParseColor(spec)
	if c == nil {
		return 0, nil, false
	}
	return ansi.BasicColor(n), c, true
}
