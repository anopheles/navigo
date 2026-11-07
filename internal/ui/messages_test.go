package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/app"
)

func TestInfoMessagesExpire(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<CR>jA")
	msg := "1 track added to the end of the queue (1 in queue) · u undoes"
	h.contains(msg)
	h.advance(7 * time.Second)
	h.contains(msg)
	h.advance(2 * time.Second)
	h.lacks(msg)
}

func TestErrorsStayUntilReplaced(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":")
	h.typeText("nope")
	h.keys("<CR>")
	h.advance(time.Hour)
	h.contains("not a command: nope")
	// The newest message wins, even information over an error.
	h.keys("<CR>jA")
	h.contains("1 track added")
	h.lacks("not a command")
	// <Esc> clears an error.
	h.keys(":")
	h.typeText("nope")
	h.keys("<CR><Esc>")
	h.lacks("not a command")
}

func TestMessagesPopup(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	for i := range 120 {
		h.m.setMsg(fmt.Sprintf("message %d", i), i%2 == 0)
	}
	h.keys(":")
	h.typeText("messages")
	h.keys("<CR>")
	// The newest are shown first; only the last 100 are kept.
	h.contains("Messages", "message 119", "of 100", "j/k scroll", "q close")
	h.lacks("message 19 ")
	h.keys("gg")
	h.contains("message 20", "Messages 1–")
	if h.m.popup.off != 0 {
		t.Errorf("gg left the popup at line %d", h.m.popup.off)
	}
	h.keys("q")
	h.lacks("Messages")
	// Keys go back to the lists once it's closed.
	h.keys("j")
	if !strings.Contains(h.cursorRow(), "Blue") {
		t.Errorf("cursor on %q after closing the popup", h.cursorRow())
	}
}

func TestUnexpectedErrorsPointToTheLog(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.send(
		eventMsg{app.Failed{What: "couldn't save the library cache", Err: errors.New("disk full")}},
	)
	h.contains("couldn't save the library cache: disk full · details in /logs/navigo.log")
}

func TestSyncingIndicator(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.lacks("⟳ syncing")
	h.m.state.Sync.Running = true
	h.contains("■  nothing playing", "⟳ syncing")
}

// Narrow, the least important items take their short forms, then go; a
// first item too long even short is cut, never dropped (#148).
func TestFitIndicators(t *testing.T) {
	items := []indicator{
		{"mpv not found · install mpv 0.38 or newer", "mpv not found", true},
		{long: "offline · next try in 8s", short: "offline"},
		{long: "⟳ syncing", short: "⟳"},
		{long: "repeat all", short: "rep all"},
	}
	for width, want := range map[int]string{
		100: "mpv not found · install mpv 0.38 or newer · offline · next try in 8s · ⟳ syncing · repeat all",
		60:  "mpv not found · offline · next try in 8s · ⟳ · repeat all",
		40:  "mpv not found · offline · ⟳ · repeat all",
		25:  "mpv not found · offline",
		15:  "mpv not found",
		8:   "mpv not…",
		0:   "",
	} {
		if got := fitIndicators(items, width); got != want {
			t.Errorf("width %d: %q, want %q", width, got, want)
		}
	}
}
