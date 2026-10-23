package ui

// PROTOTYPE (throwaway, never merge): five looks for the Visual
// selection, to replace the hard-to-read blue background. F2 cycles them
// live; NAVIGO_SELECTION=A..E picks the first one. The current variant's
// name shows at the start of the hint line.

import (
	"image/color"
	"math"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"
)

var protoSelNames = []string{
	"A current: blue bg + underline",
	"B tint: selection colour faded into the background",
	"C gutter: coloured bar in the margin + bold text",
	"D contrast: solid bg with forced black/white text",
	"E gray: neutral gray bg, text untouched",
	"F tint from playing: cover colour faded in",
	"G tint from playing, turned 75°: cover colour's neighbour hue",
	"H tint from playing, opposite: cover colour's complement",
}

var protoSel = func() int {
	if v := os.Getenv("NAVIGO_SELECTION"); len(v) == 1 && v[0] >= 'A' && v[0] < 'A'+byte(len(protoSelNames)) {
		return int(v[0] - 'A')
	}
	return 0
}()

func protoSelNext() { protoSel = (protoSel + 1) % len(protoSelNames) }

func protoSelLabel() string {
	return lipgloss.NewStyle().Reverse(true).Render(" F2 " + protoSelNames[protoSel] + " ")
}

func (m *Model) protoSelStyle(line string) string {
	switch protoSel {
	case 1:
		return lipgloss.NewStyle().Background(m.protoTint(3)).Render(line)
	case 2:
		bar := lipgloss.NewStyle().Foreground(selectionColor).Bold(true).Render("┃")
		body := strings.TrimPrefix(line, " ")
		return bar + lipgloss.NewStyle().Bold(true).Render(body)
	case 3:
		fg := color.Color(ansi.BrightWhite)
		if c, ok := colorful.MakeColor(selectionColor); ok {
			if _, basic := selectionColor.(ansi.BasicColor); !basic {
				if l, _, _ := c.Lab(); l > 0.6 {
					fg = ansi.Black
				}
			}
		}
		return lipgloss.NewStyle().Background(selectionColor).Foreground(fg).Bold(true).Render(line)
	case 5, 6, 7:
		return lipgloss.NewStyle().Background(m.protoPlayingTint([]float64{0, 75, 180}[protoSel-5])).Render(line)
	case 4:
		bg := color.Color(ansi.BrightBlack)
		if m.deps.TrueColor {
			bg = m.protoGray()
		}
		return lipgloss.NewStyle().Background(bg).Render(line)
	}
	return visualStyle.Render(line)
}

// protoTint is 1/part of the selection colour over the terminal's
// background; the terminal's bright black where the colour has no exact value.
func (m *Model) protoTint(part uint32) color.Color {
	if _, basic := selectionColor.(ansi.BasicColor); basic || !m.deps.TrueColor {
		if selectionColor == color.Color(ansi.Blue) && m.deps.TrueColor {
			return m.protoMix(color.RGBA{0x3b, 0x6e, 0xd8, 255}, part)
		}
		return ansi.BrightBlack
	}
	return m.protoMix(selectionColor, part)
}

func (m *Model) protoMix(c color.Color, part uint32) color.Color {
	var br, bg, bb uint32
	if m.covers.bg != nil {
		br, bg, bb, _ = m.covers.bg.RGBA()
	}
	ar, ag, ab, _ := c.RGBA()
	mix := func(back, acc uint32) uint8 { return uint8((back>>8)*(part-1)/part + (acc>>8)/part) }
	return color.RGBA{mix(br, ar), mix(bg, ag), mix(bb, ab), 255}
}

// protoGray is a gray a step off the terminal's background: lighter on a
// dark one, darker on a light one.
func (m *Model) protoGray() color.Color {
	var r, g, b uint32
	if m.covers.bg != nil {
		r, g, b, _ = m.covers.bg.RGBA()
	}
	if (r+g+b)/3>>8 > 128 {
		return m.protoMix(color.Black, 6)
	}
	return m.protoMix(color.White, 6)
}

// protoPlayingTint is a third of the playing colour, its hue turned by
// turn degrees, over the terminal's background; B's tint where there is no
// exact playing colour.
func (m *Model) protoPlayingTint(turn float64) color.Color {
	if !m.accentOn() {
		return m.protoTint(3)
	}
	base, ok := colorful.MakeColor(accentStops[0])
	if !ok {
		return m.protoTint(3)
	}
	h, s, l := base.Hsl()
	return m.protoMix(colorful.Hsl(math.Mod(h-turn+360, 360), s, l), 3)
}
