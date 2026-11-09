package ui

// PROTOTYPE, throw away. Three looks for a find that shows its matches
// while the / pattern is typed, switchable with <F2> at the / prompt.
// Only on when NAVIGO_FIND_PROTO is set (A, B or C picks the first one):
//
//	NAVIGO_FIND_PROTO=A make run
//
// A  incsearch: every match lit as you type, the cursor previews the next
//    match (the list scrolls to it), <Esc> puts it back; [3/12] counts.
// B  spotlight: the cursor stays put, rows without a match go faint,
//    the prompt says how many matches are above and below the window.
// C  labels: as flash.nvim, each match on screen gets a letter over the
//    character after it; typing that letter jumps there at once.

import (
	"fmt"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	protoNames      = []string{"A incsearch", "B spotlight", "C labels"}
	protoCurStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#ff8c1a")).Foreground(ansi.Black).Bold(true)
	protoLabelStyle = lipgloss.NewStyle().Background(lipgloss.Color("#ff2fa8")).Foreground(ansi.White).Bold(true)
	protoInfoStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#888888"))
	protoTagStyle   = lipgloss.NewStyle().Background(lipgloss.Color("#5f00af")).Foreground(ansi.White)
	protoAlphabet   = "asdfghjklqwertyuiopzxcvbnm"
)

type findProto struct {
	on                 bool
	variant            int
	savedCur, savedOff int
	matches            []int        // rows that match the typed pattern
	labels             map[int]rune // row → label (C)
}

func newFindProto() findProto {
	v, ok := os.LookupEnv("NAVIGO_FIND_PROTO")
	if !ok {
		return findProto{}
	}
	p := findProto{on: true}
	switch strings.ToUpper(v) {
	case "B":
		p.variant = 1
	case "C":
		p.variant = 2
	}
	return p
}

// protoLive reports whether the prototype draws for the prompt now open.
func (m *Model) protoLive() bool { return m.proto.on && m.finding.prompting }

// livePattern is what the rows highlight: the typed text while the
// prototype's prompt is open, else the last find.
func (m *Model) livePattern() (string, bool) {
	if m.protoLive() {
		return m.line.Value(), m.line.Value() != ""
	}
	return m.finding.pattern, m.finding.highlight && m.finding.pattern != ""
}

func (m *Model) protoStart() {
	if !m.proto.on {
		return
	}
	cur, off, _ := m.view()
	m.proto.savedCur, m.proto.savedOff = *cur, *off
	m.proto.matches, m.proto.labels = nil, nil
}

// protoRestore puts the cursor back where the prompt opened.
func (m *Model) protoRestore() {
	if !m.proto.on {
		return
	}
	cur, off, _ := m.view()
	*cur, *off = m.proto.savedCur, m.proto.savedOff
	m.proto.matches, m.proto.labels = nil, nil
}

// protoChanged runs after each edit of the / prompt.
func (m *Model) protoChanged() {
	if !m.proto.on {
		return
	}
	p := m.line.Value()
	cur, off, n := m.view()
	*cur, *off = m.proto.savedCur, m.proto.savedOff
	m.proto.matches, m.proto.labels = nil, nil
	if p == "" {
		return
	}
	for i := range n {
		if !m.rowSkipped(i) && matches(m.rowText(i), p) {
			m.proto.matches = append(m.proto.matches, i)
		}
	}
	switch m.proto.variant {
	case 0: // preview the next match
		if next := m.protoNext(); next >= 0 {
			*cur = next
		}
	case 2:
		m.protoLabel(p)
	}
}

// protoNext is the first match after the saved cursor, wrapping.
func (m *Model) protoNext() int {
	for _, i := range m.proto.matches {
		if i > m.proto.savedCur {
			return i
		}
	}
	if len(m.proto.matches) > 0 {
		return m.proto.matches[0]
	}
	return -1
}

// protoLabel gives the matches on screen a letter each, nearest the
// cursor first, skipping letters that could continue a match.
func (m *Model) protoLabel(p string) {
	_, off, _ := m.view()
	h := m.listHeight(m.focus)
	taken := map[rune]bool{}
	for _, i := range m.proto.matches {
		hay := strings.ToLower(m.rowText(i))
		needle := strings.ToLower(p)
		for at := 0; ; {
			j := strings.Index(hay[at:], needle)
			if j < 0 {
				break
			}
			at += j + len(needle)
			if r, _ := utf8.DecodeRuneInString(hay[at:]); r != utf8.RuneError {
				taken[unicode.ToLower(r)] = true
			}
		}
	}
	var free []rune
	for _, r := range protoAlphabet {
		if !taken[r] {
			free = append(free, r)
		}
	}
	var onScreen []int
	for _, i := range m.proto.matches {
		if i >= *off && i < *off+h {
			onScreen = append(onScreen, i)
		}
	}
	// nearest the cursor first
	c := m.proto.savedCur
	for a := 1; a < len(onScreen); a++ {
		for b := a; b > 0 && dist(onScreen[b], c) < dist(onScreen[b-1], c); b-- {
			onScreen[b], onScreen[b-1] = onScreen[b-1], onScreen[b]
		}
	}
	m.proto.labels = map[int]rune{}
	for k, i := range onScreen {
		if k >= len(free) {
			break
		}
		m.proto.labels[i] = free[k]
	}
}

func dist(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// protoKey takes <F2> and, in C, a label; it reports whether it did.
func (m *Model) protoKey(k tea.KeyPressMsg) (tea.Cmd, bool) {
	if !m.protoLive() {
		return nil, false
	}
	s := k.String()
	if s == "f2" {
		m.proto.variant = (m.proto.variant + 1) % len(protoNames)
		m.protoChanged()
		return nil, true
	}
	if m.proto.variant == 2 && utf8.RuneCountInString(s) == 1 {
		r, _ := utf8.DecodeRuneInString(s)
		for row, l := range m.proto.labels {
			if l == r {
				p := m.line.Value()
				m.protoRestore()
				m.leaveLine()
				m.finding.pattern, m.finding.back, m.finding.highlight = p, false, true
				m.recordJump()
				cur, _, _ := m.view()
				*cur = row
				m.setMsg("/"+p, false)
				return nil, true
			}
		}
	}
	return nil, false
}

// protoRow dresses a row of the focused list while the prompt is open:
// B fades rows without a match.
func (m *Model) protoRow(i int, row string) string {
	if !m.protoLive() || m.proto.variant != 1 || m.line.Value() == "" {
		return row
	}
	if matches(ansi.Strip(row), m.line.Value()) {
		return row
	}
	return dimStyle.Render(ansi.Strip(row))
}

// protoMatchStyle is the style of a match in row i: A marks the match the
// cursor previews more strongly.
func (m *Model) protoMatchStyle(i int) lipgloss.Style {
	if m.protoLive() && m.proto.variant == 0 {
		cur, _, _ := m.view()
		if i == *cur {
			return protoCurStyle
		}
	}
	return findStyle
}

// protoLabelFor is row i's label, if C gave it one.
func (m *Model) protoLabelFor(i int) (rune, bool) {
	if !m.protoLive() || m.proto.variant != 2 {
		return 0, false
	}
	r, ok := m.proto.labels[i]
	return r, ok
}

// protoStatus is the right end of the prompt line.
func (m *Model) protoStatus() string {
	if !m.protoLive() {
		return ""
	}
	tag := protoTagStyle.Render(" proto " + protoNames[m.proto.variant] + " · <F2> ")
	if m.line.Value() == "" {
		return tag
	}
	n := len(m.proto.matches)
	info := ""
	switch {
	case n == 0:
		info = "no match"
	case m.proto.variant == 0:
		cur, _, _ := m.view()
		k := 0
		for j, i := range m.proto.matches {
			if i == *cur {
				k = j + 1
			}
		}
		info = fmt.Sprintf("[%d/%d]", k, n)
	case m.proto.variant == 1:
		_, off, _ := m.view()
		h := m.listHeight(m.focus)
		above, below := 0, 0
		for _, i := range m.proto.matches {
			switch {
			case i < *off:
				above++
			case i >= *off+h:
				below++
			}
		}
		info = fmt.Sprintf("%d matches · ↑%d ↓%d off screen", n, above, below)
	default:
		info = fmt.Sprintf("%d matches · %d labelled · type a pink letter to jump", n, len(m.proto.labels))
	}
	return protoInfoStyle.Render(info) + "  " + tag
}
