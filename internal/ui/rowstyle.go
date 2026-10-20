package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/library"
)

// Rows have a hierarchy: the title is plain, and the artist, the numbers
// and the duration are faint, so a list reads by its titles. Starred names
// carry a gold ★ on a true-colour terminal, and the queue's tracks that
// have played are faint all through. A row the cursor, a selection or the
// playing mark has styled as a whole, or that a find highlights, stays
// plain, as m.dress says while it is drawn: the styles of its parts would
// end the style of the whole.

var starStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#e5c07b"))

// highlighting reports whether find matches are marked in the focused window.
func (m *Model) highlighting() bool {
	return m.finding.highlight && m.finding.pattern != ""
}

// name is a starred name's text with the ★ in gold; fit it to w cells.
func (m *Model) name(id, name string, w int) string {
	s := m.starred(id, name)
	star := strings.HasPrefix(s, "★ ")
	if !star {
		return fit(s, w)
	}
	text := fit(strings.TrimPrefix(s, "★ "), max(w-2, 0))
	if m.deps.TrueColor {
		return starStyle.Render("★") + " " + text
	}
	return "★ " + text
}

func (m *Model) trackLineDressed(mark, num string, t library.Track, w int) string {
	numW := max(len(num)+1, 3)
	rest := w - lipgloss.Width(mark) - numW - 6
	tw := rest * 5 / 9
	return dimStyle.Render(mark+fmt.Sprintf("%*s ", numW-1, num)) +
		m.name(string(t.ID), t.Title, tw) +
		dimStyle.Render(fit("  "+t.Artist, rest-tw)+fmt.Sprintf("%6s", clock(t.Duration)))
}

func (m *Model) albumLineDressed(a library.Album, w int) string {
	year := ""
	if a.Year > 0 {
		year = fmt.Sprint(a.Year)
	}
	rest := w - 1 - 6
	nw := rest * 3 / 5
	return " " + m.name(string(a.ID), a.Name, nw) +
		dimStyle.Render(fit("  "+a.Artist, rest-nw)+fmt.Sprintf("%6s", year))
}

func (m *Model) artistLineDressed(a library.Artist, w int) string {
	n := artistCount(a.AlbumCount, m.state.Library.AppearsOn(a.ID))
	return " " + m.name(string(a.ID), a.Name, w-2-len(n)-2) + dimStyle.Render("  "+n) + " "
}
