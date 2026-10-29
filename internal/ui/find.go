package ui

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
)

// findStyle marks find matches: yellow, and underlined for when colours
// are off.
var findStyle = lipgloss.NewStyle().Background(ansi.Yellow).Foreground(ansi.Black).Underline(true)

// finder is / and its last pattern. Highlight is cleared by :noh and
// <Esc>; n and N still use the pattern.
type finder struct {
	pattern   string
	back      bool // the last find was :find-back
	highlight bool
	prompting bool
	backward  bool // the prompt is a :find-back one
}

// startFind opens the / prompt (or ? for :find-back).
func (m *Model) startFind(back bool) {
	m.finding.prompting, m.finding.backward = true, back
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "/"
	if back {
		m.line.Prompt = "?"
	}
	m.fitLine()
	m.then(m.line.Focus())
}

// findKey handles the find prompt's keys other than editing; it reports
// whether it handled the key.
func (m *Model) findKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc":
		m.leaveLine()
		return nil, true
	case "enter":
		p, back := m.line.Value(), m.finding.backward
		m.leaveLine()
		if p == "" {
			p = m.finding.pattern // / alone finds the last pattern again
		}
		if p == "" {
			return nil, true
		}
		m.finding.pattern, m.finding.back, m.finding.highlight = p, back, true
		m.recordJump()
		m.find(false, 1)
		return nil, true
	}
	return nil, false
}

// find moves to the count-th next match of the pattern, the other way
// when reverse, wrapping round the ends.
func (m *Model) find(reverse bool, count int) {
	p := m.finding.pattern
	cur, _, n := m.view()
	dir := 1
	if m.finding.back != reverse {
		dir = -1
	}
	at, wrapped := *cur, false
	for range count {
		next := -1
		for step := 1; step <= n; step++ {
			i := at + dir*step
			if i < 0 || i >= n {
				wrapped = true
			}
			i = (i%n + n) % n
			if !m.rowSkipped(i) && matches(m.rowText(i), p) {
				next = i
				break
			}
		}
		if next < 0 {
			m.offerSearch = p
			m.setMsg(fmt.Sprintf("no match for %q here · <CR> searches the whole library", p), true)
			return
		}
		at = next
	}
	*cur = at
	switch {
	case wrapped && dir > 0:
		m.setMsg("/"+p+" · went on from the top", false)
	case wrapped:
		m.setMsg("/"+p+" · went on from the bottom", false)
	default:
		m.setMsg("/"+p, false)
	}
}

// findAgain is n and N.
func (m *Model) findAgain(c command.Call, reverse bool) error {
	if m.finding.pattern == "" {
		return fmt.Errorf("no find yet · / finds")
	}
	m.finding.highlight = true
	m.find(reverse, c.N())
	return nil
}

func (m *Model) rowSkipped(i int) bool {
	return m.focus == libraryWindow && m.skipped(m.top(), i)
}

// rowText is what row i of the focused list shows, without styles.
func (m *Model) rowText(i int) string {
	if m.focus == queueWindow {
		e, _ := m.queueEntry(i)
		return e.Track.Title + " " + e.Track.Artist
	}
	return ansi.Strip(m.libraryRow(m.top(), i, 300))
}

// matches is smart-case: a pattern with a capital letter matches case.
func matches(text, p string) bool {
	if strings.IndexFunc(p, unicode.IsUpper) >= 0 {
		return strings.Contains(text, p)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(p))
}

// highlight marks the pattern's matches in a row that has no styles of
// its own.
func (m *Model) highlight(row string) string {
	p := m.finding.pattern
	if !m.finding.highlight || p == "" || strings.Contains(row, "\x1b") {
		return row
	}
	hay, needle := row, p
	if strings.IndexFunc(p, unicode.IsUpper) < 0 {
		hay, needle = strings.ToLower(row), strings.ToLower(p)
	}
	if len(hay) != len(row) { // lower-casing changed the byte offsets
		return row
	}
	var out strings.Builder
	for {
		i := strings.Index(hay, needle)
		if i < 0 {
			out.WriteString(row)
			return out.String()
		}
		out.WriteString(row[:i] + findStyle.Render(row[i:i+len(needle)]))
		row, hay = row[i+len(needle):], hay[i+len(needle):]
	}
}
