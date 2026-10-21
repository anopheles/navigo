package ui

import (
	"fmt"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestClickOnTheProgressBarSeeks(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.screen() // the render records where the bar is
	b := h.m.bar
	if b.w == 0 {
		t.Fatal("no progress bar while playing")
	}
	dur := h.m.state.Player.Duration
	h.send(tea.MouseClickMsg{X: b.x + b.w/2, Y: b.y, Button: tea.MouseLeft})
	want := fmt.Sprintf("SeekTo(%v)", dur*time.Duration(b.w/2)/time.Duration(b.w))
	if c := h.lastCall(); c != want {
		t.Errorf("player call %q, want %q", c, want)
	}
	h.send(tea.MouseMotionMsg{X: b.x, Y: b.y, Button: tea.MouseLeft}) // a drag follows
	if c := h.lastCall(); c != "SeekTo(0s)" {
		t.Errorf("after a drag to the start: %q", c)
	}
	h.send(tea.MouseClickMsg{X: b.x + 3, Y: b.y - 1, Button: tea.MouseLeft}) // a row above
	if c := h.lastCall(); c != "SeekTo(0s)" {
		t.Errorf("a click off the bar seeked: %q", c)
	}
}

func TestWheelOverTheBarChangesVolume(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.screen()
	y := h.m.npRows[0]
	h.send(tea.MouseWheelMsg{X: 40, Y: y, Button: tea.MouseWheelDown})
	if c := h.lastCall(); c != "SetVolume(95)" {
		t.Errorf("wheel down: %q", c)
	}
	h.send(tea.MouseWheelMsg{X: 40, Y: y + 1, Button: tea.MouseWheelUp})
	h.send(tea.MouseWheelMsg{X: 40, Y: y + 1, Button: tea.MouseWheelUp})
	if c := h.lastCall(); c != "SetVolume(100)" {
		t.Errorf("wheel up: %q", c)
	}
	h.send(tea.MouseWheelMsg{X: 40, Y: 3, Button: tea.MouseWheelDown}) // over the list
	if c := h.lastCall(); c != "SetVolume(100)" {
		t.Errorf("wheel over the library changed the volume: %q", c)
	}
}

func click1(
	x, y int,
) tea.MouseClickMsg {
	return tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft}
}

func TestClickMovesTheCursorAndDoubleClickOpens(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.toAlbums()
	h.screen()
	depth := len(h.m.lib)
	h.send(click1(5, 3)) // the third row
	if f := h.m.top(); f.cur != 2 || len(h.m.lib) != depth {
		t.Fatalf("after one click: cur %d, depth %d", f.cur, len(h.m.lib))
	}
	h.advance(time.Second)
	h.send(click1(5, 2))
	h.advance(100 * time.Millisecond)
	h.send(click1(5, 2))
	if len(h.m.lib) != depth+1 {
		t.Errorf("a double-click didn't open the album: depth %d", len(h.m.lib))
	}
}

func TestSlowSecondClickIsNotADoubleClick(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.toAlbums()
	h.screen()
	depth := len(h.m.lib)
	h.send(click1(5, 2))
	h.advance(2 * time.Second)
	h.send(click1(5, 2))
	if len(h.m.lib) != depth {
		t.Errorf("two slow clicks opened the album")
	}
}

func TestClickInTheQueueTakesTheFocus(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>") // plays an album, filling the queue
	h.screen()
	if h.m.focus != libraryWindow {
		t.Fatal("test expects the library focused")
	}
	h.send(click1(h.m.paneAt[queueWindow].x+4, 3))
	if h.m.focus != queueWindow || h.m.queueCur != 2 {
		t.Errorf("focus %v, queue cursor %d", h.m.focus, h.m.queueCur)
	}
	h.advance(time.Second)
	h.send(click1(h.m.paneAt[queueWindow].x+4, 4))
	h.advance(50 * time.Millisecond)
	h.send(click1(h.m.paneAt[queueWindow].x+4, 4))
	if _, row, _ := h.m.state.NowPlaying(); row != 3 {
		t.Errorf("double-click played queue row %d, want 3", row)
	}
}

func TestWheelScrollsTheListUnderIt(t *testing.T) {
	h := newHarness(t, 120, 10, nil)
	h.toAlbums()
	h.screen()
	f := h.m.top()
	if h.m.rows(f) <= h.m.bodyHeight()+wheelRows {
		t.Skip("list too short to scroll")
	}
	h.send(tea.MouseWheelMsg{X: 5, Y: 2, Button: tea.MouseWheelDown})
	if f.offset != wheelRows || f.cur != wheelRows {
		t.Errorf("after the wheel: offset %d, cursor %d", f.offset, f.cur)
	}
	h.send(tea.MouseWheelMsg{X: 5, Y: 2, Button: tea.MouseWheelUp})
	if f.offset != 0 {
		t.Errorf("wheel up: offset %d", f.offset)
	}
	h.send(tea.MouseWheelMsg{X: h.m.paneAt[queueWindow].x + 4, Y: 2, Button: tea.MouseWheelDown})
	if h.m.focus != libraryWindow {
		t.Error("the wheel moved the focus")
	}
}
