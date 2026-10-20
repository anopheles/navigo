package ui

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

func TestMPVMissingAtStartup(t *testing.T) {
	h := setupHarness(t, 120, 40, nil, func(h *harness) {
		h.fake.CheckErr = fmt.Errorf("%w at mpv", player.ErrMPVMissing)
	})
	h.toAlbums()
	h.contains("mpv not found · install mpv 0.38 or newer to play") // the indicator
	// Browsing works; play keys fill the queue and repeat the hint.
	h.keys("3j<CR><CR>")
	h.contains("Queue (6)", "mpv not found · install mpv 0.38 or newer to play · "+installHint())
	h.keys("<Space>")
	h.contains(installHint())
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("player calls %q with mpv missing", calls)
	}
}

func TestMPVTooOldAtStartup(t *testing.T) {
	h := setupHarness(t, 120, 40, nil, func(h *harness) {
		h.fake.CheckErr = fmt.Errorf("%w, found mpv v0.37.0", player.ErrMPVTooOld)
	})
	h.contains("mpv too old · install mpv 0.38 or newer to play")
}

func TestMPVDiesAndResumes(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>") // Low Tide
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.fake.Die(errors.New("exit status 1"))
	h.pump()
	h.contains(
		"mpv stopped · <Space> restarts it",
		"the player stopped (exit status 1) · <Space> starts it again where it stopped",
	)
	h.keys("<Space>")
	if !slices.Contains(h.fake.Calls(), "Play(1 2 3 from 42s)") {
		t.Errorf("no resume at 42s in %q", h.fake.Calls())
	}
	h.contains("▶ Low Tide", "0:42 / 3:05")
	h.lacks("mpv stopped")
}

func TestMPVCrashCap(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	for i := range 3 {
		h.fake.Die(errors.New("crash"))
		h.pump()
		h.advance(10 * time.Second)
		if i < 2 {
			h.keys("<Space>")
		}
	}
	h.contains(
		"mpv keeps crashing · :reconnect",
		"mpv crashed 3 times in a minute",
		"details in /logs/navigo.log",
	)
	before := len(h.fake.Calls())
	h.keys("<Space>")
	if len(h.fake.Calls()) != before {
		t.Errorf("restarted while capped: %q", h.fake.Calls()[before:])
	}
	// After a minute it may start again.
	h.advance(time.Minute)
	h.lacks("mpv keeps crashing")
	h.keys("<Space>")
	h.contains("▶ Dusk Signal")
}
