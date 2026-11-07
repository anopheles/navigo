package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// Every queue key says the same on an empty queue, an empty list says it
// is empty, and bad arguments are refused with what would work (#153).
func TestEmptyQueueAndListSayTheSame(t *testing.T) {
	for _, k := range []string{">", "<lt>", "<Space>", "S"} {
		h := newHarness(t, 120, 40, nil)
		h.keys(k)
		h.contains("the queue is empty")
	}
	h := newHarness(t, 120, 40, nil)
	h.keys("'p<CR>")                            // Empty: a playlist with no tracks
	for _, k := range []string{"A", "a", "V"} { // P pastes here
		h.m.clearMsg()
		h.keys(k)
		h.contains("this list is empty")
	}
}

func TestBadArgumentsAreRefused(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":repeat foo<CR>")
	h.contains("repeat takes no arguments")
	h.keys(":shuffle 3<CR>")
	h.contains("shuffle takes no arguments")
	h.keys(":seek 150%<CR>")
	h.contains(`seek: "150%" is past the end · give 0% to 100%`)
	h.keys("3j<CR><CR>")
	h.fake.Tick(time.Second)
	h.pump()
	h.keys(":seek 99:00<CR>")
	h.contains("is past the end of this track (2:22)")
	if h.m.state.Current != 0 {
		t.Error(":seek past the end skipped a track")
	}
}

// With player.mpv_path wrong, the message says so instead of telling to
// install mpv.
func TestMPVPathMissingSaysSo(t *testing.T) {
	h := setupHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Player.MPVPath = "/nonexistent/mpv"
	}, func(h *harness) {
		h.fake.CheckErr = fmt.Errorf("%w at /nonexistent/mpv", player.ErrMPVMissing)
	})
	h.contains("mpv not found at /nonexistent/mpv · check player.mpv_path")
	h.lacks("install mpv")
}

func TestNamePromptIsNotSearch(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>gl/") // the playlist popup's filter
	h.contains("PROMPT")
	h.lacks("SEARCH")
}
