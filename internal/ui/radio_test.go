package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// radioRows counts the queue's radio rows.
func (h *harness) radioRows() int {
	n := 0
	for _, q := range h.m.state.Queue {
		if q.Radio {
			n++
		}
	}
	return n
}

func TestRadioKeepsTheQueueGoing(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.m.seedFn = func() uint64 { return 3 }
	h.keys(":radio<CR>")
	h.contains("radio on · it keeps the queue going from what plays")
	h.keys("3j<CR>VjA'q<CR>") // two of Blue Hour's tracks, the first playing
	h.pump()
	if n := h.radioRows(); n == 0 {
		t.Fatalf("the radio added nothing: queue %s", h.queueTitles())
	}
	h.contains("~ ", "radio")
	if n := h.requests("getArtistInfo2"); n == 0 {
		t.Error("no similar-artist lookup")
	}
	golden.RequireEqual(t, h.screen())

	// g- on a radio row drops its artist's upcoming radio tracks.
	h.keys("G")
	artist := h.m.state.Queue[h.m.queueCur].Track.Artist
	h.keys("g-")
	if !h.logged("no more " + artist + " on the radio") {
		t.Errorf("no message for g- on %s", artist)
	}
	for i, q := range h.m.state.Queue {
		if q.Radio && q.Track.Artist == artist && i > h.m.state.Current {
			t.Errorf("row %d by %s is still queued", i, artist)
		}
	}

	// gR switches it off; steering then says so.
	h.keys("gR")
	h.contains("radio off")
	h.keys("g+")
	h.contains("the radio is off · :radio turns it on")
}

func TestRadioFromTheConfig(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Player.Radio = true
	})
	if !h.m.state.Radio {
		t.Fatal("player.radio: true left the radio off")
	}
	if strings.Contains(h.screen(), "radio on") {
		t.Error("the config's radio announced itself")
	}
}
