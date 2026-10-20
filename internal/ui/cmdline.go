package ui

import (
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
)

// maxMatches is how many matches the ":" line lists at most.
const maxMatches = 8

// completion is what <Tab> cycles through: head is the text before the
// part being completed, items the candidates and idx the one shown.
type completion struct {
	head  string
	items []string
	idx   int
}

// cmdKey handles the keys of the ":" line that aren't plain editing. It
// reports whether it handled k.
func (m *Model) cmdKey(key string) (tea.Cmd, bool) {
	s := key
	if s != "tab" && s != "shift+tab" {
		m.compl = nil
	}
	switch s {
	case "esc", "ctrl+c":
		m.leaveLine()
		m.endVisual()
		return nil, true
	case "enter":
		text := m.line.Value()
		m.leaveLine()
		if strings.TrimSpace(text) == "" {
			m.endVisual()
			return nil, true
		}
		cmd := m.runTyped(text) // on the selection, if any
		m.endVisual()
		return tea.Batch(m.remember(&m.cmdHist, text), cmd), true
	case "backspace":
		if m.line.Value() == "" {
			m.leaveLine()
			return nil, true
		}
	case "up", "down":
		m.line.SetValue(m.cmdHist.step(s == "up", m.line.Value()))
		m.line.CursorEnd()
		return nil, true
	case "tab", "shift+tab":
		m.complete(s == "tab")
		return nil, true
	}
	return nil, false
}

// runTyped runs a typed command line. If a key runs the same command here, it
// adds a tip naming it, after whatever the command said.
func (m *Model) runTyped(line string) tea.Cmd {
	key := m.keyFor(line)
	seq := m.msgSeq
	cmd := m.exec(line, 0)
	switch {
	case key == "" || m.shown.err && m.msgSeq != seq:
	case m.msgSeq == seq || m.shown.text == "":
		m.setMsg("tip: next time press "+key, false)
	default:
		m.appendMsg(" · tip: next time press " + key)
	}
	return cmd
}

// keyFor is the key that runs a command line where the cursor is, or "".
func (m *Model) keyFor(line string) string {
	call, err := command.Parse(line)
	if err != nil {
		return ""
	}
	c, ok := m.registry.Get(call.Name)
	if !ok {
		return ""
	}
	full := strings.Join(append([]string{c.Name}, call.Args...), " ")
	if ks := m.engine.Table().KeysFor(keymap.Normal, m.scopes(), full); len(ks) > 0 {
		return ks[0]
	}
	return ""
}

// matches are the commands the ":" line's text could mean, while a name
// is being typed.
func (m *Model) matches() []*cmd {
	v := m.line.Value()
	if m.mode != keymap.Cmdline || strings.TrimSpace(v) == "" || strings.Contains(strings.TrimLeft(v, " :"), " ") {
		return nil
	}
	narrow := m.scopes()
	narrow = narrow[:len(narrow)-1] // without global: what applies here
	return m.registry.Match(strings.TrimLeft(v, " :"), func(c *cmd) bool {
		return len(m.engine.Table().Keys(keymap.Normal, narrow, c.Name)) > 0
	})
}

// complete fills in the next (or previous) command name, or argument
// when the name is typed, from the matches or the command's Complete.
func (m *Model) complete(forward bool) {
	if m.compl == nil {
		m.compl = m.candidates()
		if m.compl == nil {
			return
		}
	} else if n := len(m.compl.items); forward {
		m.compl.idx = (m.compl.idx + 1) % n
	} else {
		m.compl.idx = (m.compl.idx + n - 1) % n
	}
	m.line.SetValue(m.compl.head + m.compl.items[m.compl.idx])
	m.line.CursorEnd()
}

func (m *Model) candidates() *completion {
	v := strings.TrimLeft(m.line.Value(), " :")
	name, arg, hasArg := strings.Cut(v, " ")
	if !hasArg {
		var names []string
		for _, c := range m.matches() {
			names = append(names, c.Name)
		}
		if len(names) == 0 {
			return nil
		}
		return &completion{items: names}
	}
	c, ok := m.registry.Get(name)
	if !ok || c.Complete == nil {
		return nil
	}
	arg = strings.TrimLeft(arg, " ")
	var items []string
	for _, s := range c.Complete(m) {
		if strings.HasPrefix(strings.ToLower(s), strings.ToLower(arg)) {
			items = append(items, s)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return &completion{head: name + " ", items: items}
}

// matchList draws the matches above the status line: name, description
// and key, with the completion shown highlighted.
func (m *Model) matchList() []string {
	ms := m.matches()
	if len(ms) == 0 {
		return nil
	}
	ms = ms[:min(len(ms), maxMatches, max(m.h-3, 0))]
	nameW := 0
	for _, c := range ms {
		nameW = max(nameW, len(c.Name))
	}
	var out []string
	for _, c := range ms {
		key := ""
		if ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), c.Name); len(ks) > 0 {
			key = ks[0]
		}
		right := keyStyle.Render(key) + " "
		row := " " + fit(c.Name, nameW) + "  " + dimStyle.Render(c.Desc)
		row = fit(row, m.w-lipgloss.Width(right)) + right
		if m.compl != nil && m.compl.head == "" && m.compl.items[m.compl.idx] == c.Name {
			row = cursorStyle.Render(fit(" "+fit(c.Name, nameW)+"  "+c.Desc, m.w-lipgloss.Width(key)-1) + key + " ")
		}
		out = append(out, (row))
	}
	return out
}

// appendMsg adds text to the message shown, as one message.
func (m *Model) appendMsg(text string) {
	m.shown.text += text
	if i := slices.IndexFunc(m.history, func(h message) bool { return h.id == m.shown.id }); i >= 0 {
		m.history[i].text = m.shown.text
	}
}
