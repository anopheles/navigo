package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
)

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
		if ms.Button != tea.MouseLeft || !m.bar.at(ms.X, ms.Y) {
			return nil
		}
		dur := m.state.Player.Duration
		if dur <= 0 {
			return nil
		}
		pos := time.Duration(int64(dur) * int64(ms.X-m.bar.x) / int64(m.bar.w))
		return m.dispatch(app.SeekAt{Pos: pos})
	case tea.MouseWheelMsg:
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
