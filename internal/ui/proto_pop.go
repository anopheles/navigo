// PROTOTYPE: throwaway code for "what else makes it pop?" Never merge.
//
//	F5  accent    fixed magenta → the cover's dominant colour → the cover's two
//	F6  rows      as now → quiet hierarchy → coloured columns
//	F7  progress  as now → half-cell thin bar → eighth-block thick bar
//
// The accent follows the playing album, so play something; the row styles
// show on lists with tracks and albums; the bar needs a playing track.

package ui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/lucasb-eyer/go-colorful"

	"github.com/anopheles/navigo/internal/library"
)

type protoPop struct {
	accent, rows, bar int
	dress             bool // the row being drawn may carry its own styles
	last              string
}

var gold = lipgloss.Color("#e5c07b")

func (m *Model) popKey(k tea.KeyPressMsg) bool {
	p := &m.pop
	note := ""
	switch k.String() {
	case "f5":
		p.accent = (p.accent + 1) % 3
		note = [...]string{"accent: fixed", "accent: the cover's colour", "accent: the cover's two colours"}[p.accent]
	case "f6":
		p.rows = (p.rows + 1) % 3
		note = [...]string{"rows: as before", "rows: quiet hierarchy", "rows: coloured columns"}[p.rows]
	case "f7":
		p.bar = (p.bar + 1) % 3
		note = [...]string{"bar: as before", "bar: half-cell", "bar: eighths"}[p.bar]
	default:
		return false
	}
	p.last = ""
	m.setMsg("pop: "+note, false)
	return true
}

// popAccent keeps the global accent in step with the variant and the
// playing album's cover.
func (m *Model) popAccent() {
	p := &m.pop
	id, ok := m.playingCover()
	var art interface{ Palette() []color.Color }
	if e := m.covers.entries[id]; ok && e != nil && e.state == coverReady {
		art = e.art
	}
	key := fmt.Sprintf("%d|%s|%v", p.accent, id, art != nil)
	if key == p.last {
		return
	}
	p.last = key
	applyColors(m.deps.Config.RoleColors(), m.deps.TrueColor) // the fixed one
	if p.accent == 0 || art == nil {
		return
	}
	pal := art.Palette()
	if len(pal) == 0 {
		return
	}
	playingColor = pal[0]
	accentStyle = lipgloss.NewStyle().Foreground(pal[0]).Bold(true)
	accentStops = accentFrom(pal[0])
	if p.accent == 2 && len(pal) == 2 {
		accentStops = []color.Color{pal[0], mixColors(pal[0], pal[1]), pal[1]}
	}
}

func mixColors(a, b color.Color) color.Color {
	ca, _ := colorful.MakeColor(a)
	cb, _ := colorful.MakeColor(b)
	return ca.BlendHcl(cb, 0.5)
}

// ---------- rows ----------

func (m *Model) styleCol(s string, col int) string {
	switch m.pop.rows {
	case 1:
		switch col {
		case colTitle:
			return s
		case colArtist:
			return dimStyle.Render(s)
		}
		return dimStyle.Render(s) // numbers, years, durations
	case 2:
		switch {
		case col == colTitle:
			return lipgloss.NewStyle().Bold(true).Render(s)
		case col == colArtist && m.accentOn():
			return lipgloss.NewStyle().Foreground(accentStops[1]).Render(s)
		case col == colMeta && m.accentOn():
			return lipgloss.NewStyle().Foreground(accentStops[2]).Render(s)
		}
		return dimStyle.Render(s)
	}
	return s
}

const (
	colTitle = iota
	colArtist
	colMeta
)

// dressed reports whether a row being drawn carries its own styles.
func (m *Model) dressed() bool { return m.pop.rows > 0 && m.pop.dress }

func (m *Model) starMark(s string) string {
	if strings.HasPrefix(s, "★ ") && m.dressed() {
		return lipgloss.NewStyle().Foreground(gold).Render("★") + " " + s[len("★ "):]
	}
	return s
}

func (m *Model) trackLineDressed(mark, num string, t library.Track, w int) string {
	title := m.starred(string(t.ID), t.Title)
	numW := max(len(num)+1, 3)
	rest := w - lipgloss.Width(mark) - numW - 6
	tw := rest * 5 / 9
	star := strings.HasPrefix(title, "★ ")
	ts := m.styleCol(fit(strings.TrimPrefix(title, "★ "), tw-boolInt(star)*2), colTitle)
	if star {
		ts = lipgloss.NewStyle().Foreground(gold).Render("★") + " " + ts
	}
	return m.styleCol(mark, colMeta) + m.styleCol(fmt.Sprintf("%*s ", numW-1, num), colMeta) + ts +
		m.styleCol(fit("  "+t.Artist, rest-tw), colArtist) +
		m.styleCol(fmt.Sprintf("%6s", clock(t.Duration)), colMeta)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (m *Model) albumLineDressed(a library.Album, w int) string {
	year := ""
	if a.Year > 0 {
		year = fmt.Sprint(a.Year)
	}
	rest := w - 1 - 6
	nw := rest * 3 / 5
	name := m.starred(string(a.ID), a.Name)
	star := strings.HasPrefix(name, "★ ")
	ns := m.styleCol(fit(strings.TrimPrefix(name, "★ "), nw-boolInt(star)*2), colTitle)
	if star {
		ns = lipgloss.NewStyle().Foreground(gold).Render("★") + " " + ns
	}
	return " " + ns + m.styleCol(fit("  "+a.Artist, rest-nw), colArtist) +
		m.styleCol(fmt.Sprintf("%6s", year), colMeta)
}

// ---------- progress ----------

// progress is the bar for fill of w cells: the variant's, or the current.
func (m *Model) progress(fill, w int) string {
	pl := m.state.Player
	if m.pop.bar == 0 || pl.Duration <= 0 || w < 4 {
		return progressBar(fill, w)
	}
	f := math.Max(0, math.Min(1, float64(pl.Position)/float64(pl.Duration)))
	paint := func(n int) []string { // n gradient-coloured copies of a style
		out := make([]string, max(n, 1))
		cs := []color.Color{playingColor}
		if m.accentOn() {
			cs = lipgloss.Blend1D(max(n, 2), lipgloss.Darken(accentStops[0], 0.5), accentStops[0], accentStops[1])
		}
		for i := range out {
			out[i] = ansi.NewStyle().ForegroundColor(cs[min(i, len(cs)-1)]).Bold().String()
		}
		return out
	}
	var sb strings.Builder
	switch m.pop.bar {
	case 1: // thin, half a cell at a time
		half := int(f * float64(w) * 2)
		full := half / 2
		st := paint(full)
		for i := 0; i < full; i++ {
			sb.WriteString(st[min(i, len(st)-1)] + "━\x1b[m")
		}
		used := full
		if half%2 == 1 {
			sb.WriteString(accentStyle.Render("╸"))
			used++
		}
		sb.WriteString(dimStyle.Render(strings.Repeat("─", w-used)))
	case 2: // thick, an eighth of a cell at a time
		eighths := int(f * float64(w) * 8)
		full := eighths / 8
		st := paint(full)
		for i := 0; i < full; i++ {
			sb.WriteString(st[min(i, len(st)-1)] + "█\x1b[m")
		}
		used := full
		if r := eighths % 8; r > 0 {
			sb.WriteString(accentStyle.Render(string([]rune("▏▎▍▌▋▊▉")[r-1])))
			used++
		}
		sb.WriteString(dimStyle.Render(strings.Repeat("░", w-used)))
	}
	return sb.String()
}
