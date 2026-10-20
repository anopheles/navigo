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
