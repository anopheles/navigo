package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
)

// wheelRows is how many rows one wheel notch scrolls a list.
const wheelRows = 3

// doubleClick is the longest gap between two clicks on a row that count as
// one double-click.
const doubleClick = 400 * time.Millisecond

// paneHit is the columns a window covers: w cells from x.
type paneHit struct{ x, w int }

// click is the last click on a list row.
type click struct {
	win window
	row int
	at  time.Time
}

// wheelVolume is how many points one wheel notch moves the volume.
const wheelVolume = 5

// barHit is where the progress bar sits on screen: row y, w cells from x.
type barHit struct{ y, x, w int }

// at reports whether the cell is on the bar.
func (b barHit) at(x, y int) bool { return b.w > 0 && y == b.y && x >= b.x && x < b.x+b.w }

// mouse acts on a mouse event: a click or drag on the progress bar seeks
// there, and the wheel over the now-playing bar changes the volume.
func (m *Model) mouse(msg tea.MouseMsg) tea.Cmd {
	if m.screen == screenLogin || m.popup != nil {
		return nil
	}
	ms := msg.Mouse()
	switch msg.(type) {
	case tea.MouseClickMsg, tea.MouseMotionMsg:
		if ms.Button != tea.MouseLeft {
			return nil
		}
		if _, ok := msg.(tea.MouseClickMsg); ok {
			if win, row, ok := m.rowAt(ms.X, ms.Y); ok {
				return m.clickRow(win, row)
			}
		}
		if !m.bar.at(ms.X, ms.Y) {
			return nil
		}
		dur := m.state.Player.Duration
		if dur <= 0 {
			return nil
		}
		pos := time.Duration(int64(dur) * int64(ms.X-m.bar.x) / int64(m.bar.w))
		return m.dispatch(app.SeekAt{Pos: pos})
	case tea.MouseWheelMsg:
		if win, ok := m.windowAt(ms.X, ms.Y); ok {
			switch ms.Button {
			case tea.MouseWheelUp:
				m.wheelList(win, -wheelRows)
			case tea.MouseWheelDown:
				m.wheelList(win, wheelRows)
			}
			return nil
		}
		if ms.Y < m.npRows[0] || ms.Y >= m.npRows[0]+m.npRows[1] {
			return nil
		}
		switch ms.Button {
		case tea.MouseWheelUp:
			return m.dispatch(app.ChangeVolume{By: wheelVolume})
		case tea.MouseWheelDown:
			return m.dispatch(app.ChangeVolume{By: -wheelVolume})
		}
	}
	return nil
}

// windowAt is the window whose rows are under the cell, if any.
func (m *Model) windowAt(x, y int) (window, bool) {
	if y < 1 || y > m.bodyHeight() {
		return 0, false
	}
	for _, win := range []window{libraryWindow, queueWindow} {
		if p := m.paneAt[win]; p.w > 2 && x > p.x && x < p.x+p.w-1 {
			return win, true
		}
	}
	return 0, false
}

// rowAt is the list row under the cell: in the window, the row's index.
func (m *Model) rowAt(x, y int) (window, int, bool) {
	win, ok := m.windowAt(x, y)
	if !ok {
		return 0, 0, false
	}
	_, off, n := m.viewOf(win)
	row := *off + y - 1
	return win, row, row < n
}

// clickRow moves the cursor to the row, in that window, which takes the
// focus. A second click on the same row at once does its <CR>.
func (m *Model) clickRow(win window, row int) tea.Cmd {
	if m.focus != win {
		m.focus = win
		m.checkVisual()
	}
	m.setCursor(row)
	now := m.clock.Now()
	last := m.click
	m.click = click{win, row, now}
	if last.win != win || last.row != row || now.Sub(last.at) > doubleClick {
		return nil
	}
	m.click = click{}
	m.seq++
	m.prefixShown = false
	return m.key(tea.KeyPressMsg{Code: tea.KeyEnter})
}

// wheelList scrolls a window's list by d rows without moving its focus; the
// cursor goes along only when it would leave the screen.
func (m *Model) wheelList(win window, d int) {
	cur, off, n := m.viewOf(win)
	h := m.bodyHeight()
	*off = min(max(*off+d, 0), max(n-h, 0))
	to := min(max(*cur, *off), max(*off+h-1, 0))
	*cur = min(to, max(n-1, 0))
	if win == libraryWindow {
		m.settle(m.top(), 1)
	}
}
