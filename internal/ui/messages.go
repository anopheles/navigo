package ui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

// Messages: information clears itself after infoFor, errors stay until a
// newer message or <Esc>, and the newest always wins. The last
// historySize messages are kept for :messages.
const (
	infoFor     = 8 * time.Second
	historySize = 100
)

type message struct {
	id   int
	text string
	err  bool
	at   time.Time
}

type expireMsg struct{ id int }

// timeSource is time for the UI, so tests can run timers without waiting.
type timeSource interface {
	Now() time.Time
	// After returns a command that delivers fn's message after d.
	After(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
func (realClock) After(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	return tea.Tick(d, fn)
}

// setMsg shows a message in the status line and records it.
func (m *Model) setMsg(text string, isErr bool) {
	if text == "" {
		m.clearMsg()
		return
	}
	m.msgSeq++
	msg := message{id: m.msgSeq, text: text, err: isErr, at: m.clock.Now()}
	m.history = append(m.history, msg)
	if over := len(m.history) - historySize; over > 0 {
		m.history = m.history[over:]
	}
	m.shown = msg
	if !isErr {
		id := msg.id
		m.timers = append(m.timers, m.clock.After(infoFor, func(time.Time) tea.Msg { return expireMsg{id} }))
	}
}

// clearMsg empties the status line; the history keeps the message.
func (m *Model) clearMsg() { m.shown = message{} }

func (m *Model) expire(id int) {
	if m.shown.id == id && !m.shown.err {
		m.clearMsg()
	}
}

// showMessages opens the :messages popup, newest last.
func (m *Model) showMessages() {
	lines := make([]string, 0, len(m.history))
	for _, msg := range m.history {
		text := msg.text
		if msg.err {
			text = errStyle.Render(text)
		}
		lines = append(lines, dimStyle.Render(msg.at.Format("15:04:05"))+"  "+text)
	}
	if len(lines) == 0 {
		lines = []string{dimStyle.Render("no messages yet")}
	}
	m.openPopup("Messages", lines)
	m.popup.off = max(len(lines)-m.popupRows(), 0)
}

// popup is a box over both windows. Motions scroll it; q and <Esc> close
// it. A popup with pick is a choice: motions move its cursor, sel, and
// <CR> picks the row.
type popup struct {
	title string
	lines []string
	off   int
	sel   int
	pick  func(int) error
	// yes makes it a Confirmation: y runs it, n or <Esc> doesn't. These
	// keys are fixed, not bindings.
	yes func()
}

func (m *Model) openPopup(title string, lines []string) {
	m.popup = &popup{title: title, lines: lines}
	m.engine.Reset()
}

// openChoice asks the user to pick one of lines.
func (m *Model) openChoice(title string, lines []string, pick func(int) error) {
	m.openPopup(title, lines)
	m.popup.pick = pick
}

// confirm asks a y/n question before a change on the server.
func (m *Model) confirm(question string, yes func()) {
	m.openPopup(question, []string{hint("y", "yes") + "   " + hint("n", "no")})
	m.popup.yes = yes
}

// confirmKey handles a key while a Confirmation is open.
func (m *Model) confirmKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.popup
	switch k.String() {
	case "y", "Y":
		m.closePopup()
		m.pendingCmd = nil
		p.yes()
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return cmd
	case "n", "N", "esc", "q", "ctrl+c":
		m.closePopup()
		m.setMsg("nothing changed", false)
	}
	return nil
}

// choose picks the choice under the popup's cursor and closes it.
func (m *Model) choose() error {
	p := m.popup
	if p == nil || p.pick == nil {
		return nil
	}
	m.closePopup()
	return p.pick(p.sel)
}

func (m *Model) closePopup() { m.popup = nil }

// popupRows is how many lines of a popup fit between the title row and
// the hint line: it starts on row 2 and adds a title and two borders.
func (m *Model) popupRows() int { return max(m.h-9, 3) }

func (m *Model) scrollPopup(d int) {
	p := m.popup
	if p.pick != nil {
		p.sel = min(max(p.sel+d, 0), len(p.lines)-1)
		p.off = scroll(p.sel, p.off, m.popupRows(), len(p.lines))
		return
	}
	p.off = min(max(p.off+d, 0), max(len(p.lines)-m.popupRows(), 0))
}

// renderPopup draws the popup box, to lay over the screen.
func (m *Model) renderPopup() string {
	p := m.popup
	w := min(m.w-4, 100)
	rows := m.popupRows()
	end := min(p.off+rows, len(p.lines))
	body := make([]string, 0, rows+2)
	for i, l := range p.lines[p.off:end] {
		l = fit(l, w-4)
		if p.pick != nil && p.off+i == p.sel {
			l = cursorStyle.Render(l)
		}
		body = append(body, l)
	}
	more := ""
	if len(p.lines) > rows {
		more = fmt.Sprintf(" %d–%d of %d", p.off+1, end, len(p.lines))
	}
	title := keyStyle.Render(p.title) + dimStyle.Render(more)
	return popupStyle.Width(w).Render(title + "\n" + strings.Join(body, "\n"))
}
