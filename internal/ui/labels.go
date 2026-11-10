package ui

import (
	"fmt"
	"slices"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
)

// labelAlphabet is the jump labels in the order they are given out, the
// home row first.
const labelAlphabet = "asdfghjklqwertyuiopzxcvbnm"

// labeler is f: the text typed, and the jump labels it puts on the rows
// on screen that match it. f is a motion, so an operator waits in op
// while the text is typed, and target is the row a label chose while the
// operator runs.
type labeler struct {
	prompting bool
	labels    map[int]rune // row → its jump label
	order     []int        // the labelled rows, nearest the cursor first
	rows      int          // the rows that match, labelled or not
	op        *keymap.Result
	chosen    bool // a label chose target, for the operator now running
	target    int
}

// labelStyle draws a jump label as a key: reverse bold, so it shows
// without colours too.
func (m *Model) labelStyle() lipgloss.Style {
	return keyStyle.Reverse(true).Bold(true)
}

// labelJump is f: it opens the prompt, or, while an operator runs with f
// as its motion, goes to the row a label chose.
func (m *Model) labelJump(command.Call) error {
	if m.labeling.chosen {
		m.setCursor(m.labeling.target)
		return nil
	}
	if m.popup != nil {
		if m.popup.keyMenu == nil {
			return nil // labels go on a list, never a popup
		}
		m.closePopup() // f from the key menu jumps where it was opened
	}
	m.startLabels(nil)
	return nil
}

// startLabels opens the f prompt; op is the operator waiting for its
// motion, if any.
func (m *Model) startLabels(op *keymap.Result) {
	m.labeling = labeler{prompting: true, op: op}
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "f "
	m.fitLine()
	m.then(m.line.Focus())
}

// labelOperator takes an operator whose motion is f: it waits at the
// prompt for the label rather than running now. It reports whether it did.
func (m *Model) labelOperator(r keymap.Result) bool {
	if r.Motion == "" || m.labeling.chosen ||
		(keymap.Binding{Command: r.Motion}).Name() != "label-jump" {
		return false
	}
	if m.popup != nil {
		return true
	}
	m.startLabels(&r)
	return true
}

// labelLetter jumps when k is a jump label on screen; it reports whether
// it was one.
func (m *Model) labelLetter(k tea.KeyPressMsg) (tea.Cmd, bool) {
	r, size := utf8.DecodeRuneInString(k.Text)
	if size == 0 || size != len(k.Text) {
		return nil, false
	}
	for row, l := range m.labeling.labels {
		if l == r {
			return m.jumpTo(row), true
		}
	}
	return nil, false
}

// labelKey handles the f prompt's keys other than editing; it reports
// whether it handled the key.
func (m *Model) labelKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc":
		m.leaveLine()
		return nil, true
	case "enter":
		if len(m.labeling.order) > 0 {
			return m.jumpTo(m.labeling.order[0]), true
		}
		return nil, true
	case "", "backspace":
		return nil, false
	}
	return nil, true // no history, completion or next match here
}

// jumpTo ends the prompt on row: the cursor goes there, or the operator
// that waited runs to there.
func (m *Model) jumpTo(row int) tea.Cmd {
	op := m.labeling.op
	m.leaveLine()
	if op != nil {
		m.labeling.chosen, m.labeling.target = true, row
		defer func() { m.labeling.chosen = false }()
		return m.execCall(
			op.Command,
			command.Call{Count: op.Count, Motion: op.Motion, Double: op.Double},
		)
	}
	if m.mode == keymap.Normal {
		m.recordJump()
	}
	m.setCursor(row)
	return nil
}

// relabel gives the rows on screen that match the typed text their jump
// labels, nearest the cursor first. A letter that could continue a match
// is no label, so typing on still narrows.
func (m *Model) relabel() {
	l := &m.labeling
	l.labels, l.order, l.rows = map[int]rune{}, nil, 0
	p := m.line.Value()
	if p == "" {
		return
	}
	cur, off, n := m.view()
	var rows []int
	for i := *off; i < min(*off+m.listHeight(m.focus), n); i++ {
		if i != *cur && m.rowMatches(i, p) {
			rows = append(rows, i)
		}
	}
	l.rows = len(rows)
	slices.SortStableFunc(rows, func(a, b int) int { return dist(a, *cur) - dist(b, *cur) })
	letters := []rune(labelAlphabet)
	letters = slices.DeleteFunc(letters, func(r rune) bool {
		return slices.ContainsFunc(rows, func(i int) bool { return m.rowMatches(i, p+string(r)) })
	})
	for k, i := range rows {
		if k >= len(letters) {
			break
		}
		l.labels[i] = letters[k]
		l.order = append(l.order, i)
	}
}

func dist(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// labelStatus is the right end of the f prompt.
func (m *Model) labelStatus() string {
	switch l := m.labeling; {
	case !l.prompting || m.line.Value() == "":
		return ""
	case l.rows == 0:
		return "no match"
	case l.rows == 1:
		return "1 row"
	default:
		return fmt.Sprintf("%d rows", l.rows)
	}
}

// labelRow puts row i's jump label over its first column.
func (m *Model) labelRow(i int, row string) string {
	if !m.labeling.prompting {
		return row
	}
	r, ok := m.labeling.labels[i]
	if !ok {
		return row
	}
	return m.labelStyle().Render(string(r)) + ansi.TruncateLeft(row, 1, "")
}
