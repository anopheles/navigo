package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func (h *harness) cmd(line string) {
	h.t.Helper()
	h.keys(":")
	h.typeText(line)
	h.keys("<CR>")
}

func (h *harness) lastCall() string {
	calls := h.fake.Calls()
	if len(calls) == 0 {
		return ""
	}
	return calls[len(calls)-1]
}

func TestServerGoesAwayAndComesBack(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.srv.SetDown(true)
	h.cmd("sync")
	h.contains(
		"can't reach the server · browsing from the cache",
		"offline · next try in 2s",
		"Ångström Days",
	)
	h.advance(time.Second)
	h.contains("offline · next try in 1s")
	// The first retry fails: the wait doubles.
	h.advance(time.Second)
	h.contains("offline · next try in 4s")
	h.srv.SetDown(false)
	h.advance(4 * time.Second)
	h.contains("back online")
	h.lacks("offline ·")
}

func TestPlayWhileOfflineWaits(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.srv.SetDown(true)
	h.cmd("sync")
	h.keys("3j<CR><CR>")
	h.contains(
		"Queue (6)",
		"offline · the queue is ready · <Space> plays it once the server is back",
	)
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("player calls while offline: %q", calls)
	}
	// Coming back says so, and plays nothing until asked.
	h.srv.SetDown(false)
	h.cmd("reconnect")
	h.contains("back online")
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("playback started on reconnect: %q", calls)
	}
	h.keys("<Space>")
	if calls := h.fake.Calls(); len(calls) == 0 || !strings.HasPrefix(calls[0], "Play(") {
		t.Errorf("<Space> after reconnect: %q", calls)
	}
}

func TestStreamFailureWithTheServerGone(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.srv.SetDown(true)
	h.fake.FailCurrent("loading failed")
	h.pump()
	if h.lastCall() != "Stop" {
		t.Errorf("player calls = %q, want them to end with Stop", h.fake.Calls())
	}
	h.contains("offline · next try in 2s", "■ nothing playing")
	if h.m.state.Current != 0 {
		t.Errorf("current row %d, want the failed track", h.m.state.Current)
	}
}

func TestBrokenFileIsSkipped(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.fake.FailCurrent("loading failed")
	h.pump()
	h.contains("couldn't play Dusk Signal (loading failed) · skipped", "▶ Low Tide")
	h.lacks("offline")
}

func TestStartOfflineFromTheCache(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.srv.SetDown(true)
	h2 := h.restart()
	h2.contains("Ångström Days", "offline · next try in 2s")
	h2.lacks("can't log in")
	h2.srv.SetDown(false)
	h2.advance(2 * time.Second)
	h2.contains("back online")
}

func TestStartOfflineWithoutCache(t *testing.T) {
	var url string
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		url = srv.URL
		srv.SetDown(true)
	})
	h.contains("can't reach "+url+" · retrying · :login to change it", "offline · next try in 2s")
	h.srv.SetDown(false)
	h.advance(2 * time.Second)
	h.contains("Ångström Days")
}

func TestRejectedCredentialsFoundOnReconnect(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		c.Server.Password = "changed"
		srv.SetDown(true)
	})
	h.contains("offline · next try in 2s")
	h.srv.SetDown(false)
	h.advance(2 * time.Second)
	h.contains("the server rejected the saved username or password", "Password")
}

func TestBufferingStall(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.fake.Emit(player.Buffering{Buffering: true})
	h.pump()
	// A slow server: still waiting after 10 s.
	h.advance(10 * time.Second)
	h.lacks("offline")
	// Gone: stop and go offline at the next check.
	h.srv.SetDown(true)
	h.advance(10 * time.Second)
	h.contains("offline · next try in")
	if h.lastCall() != "Stop" {
		t.Errorf("player calls = %q, want Stop", h.fake.Calls())
	}
}

func TestReconnectWhileOnline(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.cmd("reconnect")
	h.contains("connected to the server")
}

// Offline, the track playing from mpv's buffer stays in the bar even when
// the queue moves to another track.
func TestBarShowsWhatPlaysWhileOffline(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.srv.SetDown(true)
	h.cmd("sync")
	h.keys("2j<CR>")
	h.contains("▶ Dusk Signal", "not in the queue", "offline · the queue is ready")
	h.lacks("▶ Streetlamp")
}
