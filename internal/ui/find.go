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

var (
	// findStyle marks find matches: yellow, and underlined for when colours
	// are off.
	findStyle = lipgloss.NewStyle().Background(ansi.Yellow).Foreground(ansi.Black).Underline(true)
	// curFindStyle marks the match the cursor is on while / is typed, where
	// <CR> will land; bold tells it apart without colours.
	curFindStyle = lipgloss.NewStyle().
			Background(ansi.BrightYellow).
			Foreground(ansi.Black).
			Bold(true).
			Underline(true)
)

// finder is / and its last pattern. Highlight is cleared by :noh and
// <Esc>; n and N still use the pattern. While the prompt is open, the
// typed text is lit and the cursor sits on the match <CR> will go to;
// start and startOff are where the prompt opened, for <Esc>.
type finder struct {
	pattern   string
	back      bool // the last find was :find-back
	highlight bool
	prompting bool
	backward  bool // the prompt is a :find-back one
	start     int
	startOff  int
	at, total int // the typed text's matches: the cursor's is the at-th
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
	cur, off, _ := m.view()
	m.finding.start, m.finding.startOff = *cur, *off
	m.finding.at, m.finding.total = 0, 0
}

// findKey handles the find prompt's keys other than editing; it reports
// whether it handled the key.
func (m *Model) findKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc":
		m.restoreFind()
		m.leaveLine()
		return nil, true
	case "tab", "match-next", "shift+tab", "match-prev":
		p := m.line.Value()
		reverse := key == "shift+tab" || key == "match-prev"
		cur, _, _ := m.view()
		if p == "" {
			return nil, true
		}
		if at, _, ok := m.nextMatch(*cur, findDir(m.finding.backward, reverse), p); ok {
			*cur = at
			m.finding.at, m.finding.total = m.matchCount(p)
		}
		return nil, true
	case "enter":
		return m.acceptFind(), true
	}
	return nil, false
}

// acceptFind is <CR> at the prompt: the cursor stays on the match it
// shows, or with nothing typed the last pattern is found again.
func (m *Model) acceptFind() tea.Cmd {
	p, back := m.line.Value(), m.finding.backward
	cur, _, _ := m.view()
	at, start := *cur, m.finding.start
	m.restoreFind()
	m.leaveLine()
	if p == "" {
		p = m.finding.pattern // / alone finds the last pattern again
		if p == "" {
			return nil
		}
		m.finding.pattern, m.finding.back, m.finding.highlight = p, back, true
		m.recordJump()
		m.find(false, 1)
		return nil
	}
	m.finding.pattern, m.finding.back, m.finding.highlight = p, back, true
	if !m.rowMatches(at, p) { // the prompt said so: search the library
		return m.exec("search "+p, 0)
	}
	m.recordJump()
	cur, _, _ = m.view()
	*cur = at
	dir := findDir(back, false)
	m.findMsg(p, dir > 0 && at <= start || dir < 0 && at >= start, dir)
	return nil
}

// restoreFind puts the cursor and the scroll back where the prompt opened.
func (m *Model) restoreFind() {
	cur, off, _ := m.view()
	*cur, *off = m.finding.start, m.finding.startOff
	m.finding.at, m.finding.total = 0, 0
}

// liveFind runs as the pattern is typed: the cursor goes to the match <CR>
// would find from where the prompt opened, or back there.
func (m *Model) liveFind() {
	m.restoreFind()
	p := m.line.Value()
	if p == "" {
		return
	}
	if at, _, ok := m.nextMatch(m.finding.start, findDir(m.finding.backward, false), p); ok {
		cur, _, _ := m.view()
		*cur = at
		m.finding.at, m.finding.total = m.matchCount(p)
	}
}

// findStatus is the right end of the find prompt: where the cursor is
// among the matches, or what <CR> does when there are none.
func (m *Model) findStatus() string {
	switch {
	case !m.finding.prompting || m.line.Value() == "":
		return ""
	case m.finding.total == 0:
		return "no match · <CR> searches the whole library"
	}
	return fmt.Sprintf("[%d/%d]", m.finding.at, m.finding.total)
}

// find moves to the count-th next match of the pattern, the other way
// when reverse, wrapping round the ends.
func (m *Model) find(reverse bool, count int) {
	p := m.finding.pattern
	cur, _, _ := m.view()
	dir := findDir(m.finding.back, reverse)
	at, wrapped := *cur, false
	for range count {
		next, w, ok := m.nextMatch(at, dir, p)
		if !ok {
			m.offerSearch = p
			m.setMsg(fmt.Sprintf("no match for %q here · <CR> searches the whole library", p), true)
			return
		}
		at, wrapped = next, wrapped || w
	}
	*cur = at
	m.findMsg(p, wrapped, dir)
}

// findDir is 1 for a find down the list and -1 up it.
func findDir(back, reverse bool) int {
	if back != reverse {
		return -1
	}
	return 1
}

// nextMatch is the first row after from, going dir, that matches p,
// wrapping round the ends; wrapped says it went round.
func (m *Model) nextMatch(from, dir int, p string) (at int, wrapped, ok bool) {
	_, _, n := m.view()
	for step := 1; step <= n; step++ {
		i := from + dir*step
		if i < 0 || i >= n {
			wrapped = true
		}
		i = (i%n + n) % n
		if m.rowMatches(i, p) {
			return i, wrapped, true
		}
	}
	return 0, false, false
}

// matchCount is how many rows match p, and which of them the cursor is
// on (0 when it is on none).
func (m *Model) matchCount(p string) (at, total int) {
	cur, _, n := m.view()
	for i := range n {
		if m.rowMatches(i, p) {
			total++
			if i == *cur {
				at = total
			}
		}
	}
	return at, total
}

// findMsg says what a find found: the pattern, the count, and whether it
// went round an end.
func (m *Model) findMsg(p string, wrapped bool, dir int) {
	at, total := m.matchCount(p)
	msg := fmt.Sprintf("/%s [%d/%d]", p, at, total)
	switch {
	case wrapped && dir > 0:
		msg += " · went on from the top"
	case wrapped:
		msg += " · went on from the bottom"
	}
	m.setMsg(msg, false)
}

// findAgain is n and N.
func (m *Model) findAgain(c command.Call, reverse bool) error {
	if m.finding.pattern == "" {
		return fmt.Errorf("nothing to find again · / finds a row in this list")
	}
	m.finding.highlight = true
	m.find(reverse, c.N())
	return nil
}

func (m *Model) rowMatches(i int, p string) bool {
	return !m.rowSkipped(i) && matches(m.rowText(i), p)
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

// litPattern is what the focused list marks: the text being typed, else
// the last find until :noh.
func (m *Model) litPattern() string {
	switch {
	case m.finding.prompting:
		return m.line.Value()
	case m.finding.highlight:
		return m.finding.pattern
	}
	return ""
}

// highlight marks the pattern's matches in a row that has no styles of
// its own; while / is typed, the cursor row's stand out.
func (m *Model) highlight(row string, cursor bool) string {
	p := m.litPattern()
	if p == "" || strings.Contains(row, "\x1b") {
		return row
	}
	style := findStyle
	if cursor && m.finding.prompting {
		style = curFindStyle
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
		out.WriteString(row[:i] + style.Render(row[i:i+len(needle)]))
		row, hay = row[i+len(needle):], hay[i+len(needle):]
	}
}
