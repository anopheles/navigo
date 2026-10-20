package ui

import (
	"errors"
	"slices"

	"github.com/anopheles/navigo/internal/command"
)

// view is the focused list's cursor and scroll offset, and its length.
func (m *Model) view() (cur, off *int, n int) { return m.viewOf(m.focus) }

// viewOf is view for either window.
func (m *Model) viewOf(win window) (cur, off *int, n int) {
	if win == queueWindow {
		return &m.queueCur, &m.queueOff, len(m.state.Queue)
	}
	f := m.top()
	return &f.cur, &f.offset, m.rows(f)
}

// place puts the cursor on row i of the focused list, within it and off
// headings (dir says which way to step off one).
func (m *Model) place(i, dir int) {
	cur, _, n := m.view()
	*cur = min(max(i, 0), max(n-1, 0))
	if m.focus == libraryWindow {
		m.settle(m.top(), dir)
	}
}

// visible is the number of the focused list's rows on screen from off.
func (m *Model) visible(off, n int) int { return max(min(m.bodyHeight(), n-off), 1) }

// page is <C-f> and <C-b>: a page on, the cursor at the top of it (or
// back, at the bottom).
func (m *Model) page(c command.Call, dir int) {
	if m.popup != nil {
		m.scrollPopup(dir * c.N() * m.popupRows())
		return
	}
	cur, off, _ := m.view()
	h := m.bodyHeight()
	m.place(*cur+dir*c.N()*h, dir)
	if dir > 0 {
		*off = *cur
	} else {
		*off = max(*cur-h+1, 0)
	}
}

// scrollBy is <C-e> and <C-y>: the view moves, the cursor only if it
// would leave the screen.
func (m *Model) scrollBy(c command.Call, dir int) {
	if m.popup != nil {
		m.scrollPopup(dir * c.N())
		return
	}
	cur, off, n := m.view()
	h := m.bodyHeight()
	*off = min(max(*off+dir*c.N(), 0), max(n-h, 0))
	switch {
	case *cur < *off:
		m.place(*off, 1)
	case *cur > *off+h-1:
		m.place(*off+h-1, -1)
	}
}

// screenRow is H, M and L: the top, middle or bottom row on screen; a
// count counts from the top (H) or the bottom (L).
func (m *Model) screenRow(c command.Call, where int) {
	if m.popup != nil {
		return
	}
	_, off, n := m.view()
	v := m.visible(*off, n)
	switch where {
	case -1:
		m.place(*off+c.N()-1, 1)
	case 0:
		m.place(*off+(v-1)/2, 1)
	case 1:
		m.place(*off+v-c.N(), -1)
	}
}

// scrollTo is zz, zt and zb: the cursor row goes to the middle, top or
// bottom of the screen.
func (m *Model) scrollTo(where int) {
	if m.popup != nil {
		return
	}
	cur, off, _ := m.view()
	h := m.bodyHeight()
	switch where {
	case -1:
		*off = *cur
	case 0:
		*off = max(*cur-h/2, 0)
	case 1:
		*off = max(*cur-h+1, 0)
	}
}

// percent is [count]%: the row count percent of the way down.
func (m *Model) percent(c command.Call) error {
	if c.Count == 0 || c.Count > 100 {
		return errors.New("give a percentage before %, such as 50%")
	}
	if m.popup != nil {
		return nil
	}
	_, _, n := m.view()
	m.place((c.Count*n+99)/100-1, 1)
	return nil
}

// goPlaying is gp: the queue window, on the entry that plays.
func (m *Model) goPlaying(command.Call) error {
	_, row, ok := m.state.NowPlaying()
	if !ok || row < 0 {
		return errors.New("nothing in the queue plays")
	}
	m.focus = queueWindow
	m.queueCur = row
	return nil
}

// location is a place the jump list can go back to.
type location struct {
	focus    window
	lib      []frame
	queueCur int
}

const jumpListSize = 100

func (m *Model) here() location {
	return location{m.focus, slices.Clone(m.lib), m.queueCur}
}

// recordJump puts where the cursor is on the jump list, before a jump;
// anything ahead of it is dropped, as in vim.
func (m *Model) recordJump() {
	m.jumps = append(m.jumps[:m.jumpAt], m.here())
	if over := len(m.jumps) - jumpListSize; over > 0 {
		m.jumps = m.jumps[over:]
	}
	m.jumpAt = len(m.jumps)
}

// jumpBack is <C-o>: to the place before the last jump.
func (m *Model) jumpBack(c command.Call) error {
	for range c.N() {
		if m.jumpAt == len(m.jumps) {
			m.jumps = append(m.jumps, m.here()) // so <C-i> comes back here
		}
		if m.jumpAt == 0 {
			return errors.New("at the oldest place in the jump list")
		}
		m.jumpAt--
		m.goLocation(m.jumps[m.jumpAt])
	}
	return nil
}

// jumpForward is <C-i> and :forward: back to where <C-o> came from.
func (m *Model) jumpForward(c command.Call) error {
	for range c.N() {
		if m.jumpAt+1 >= len(m.jumps) {
			return errors.New("at the newest place in the jump list")
		}
		m.jumpAt++
		m.goLocation(m.jumps[m.jumpAt])
	}
	return nil
}

func (m *Model) goLocation(l location) {
	m.stash()
	m.focus, m.lib, m.queueCur = l.focus, slices.Clone(l.lib), l.queueCur
	m.clamp()
}
