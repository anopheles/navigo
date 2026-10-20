package ui

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// The walking skeleton's path: albums → open one → play a track.
func TestBrowseOpenPlay(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if h.logins != 1 {
		t.Fatalf("logged in %d times", h.logins)
	}
	h.contains("Ångström Days", "The Long Haul", "夜明け", "The queue is empty.", "<CR> open")
	golden.RequireEqual(t, h.screen())

	h.keys("3j")
	if got := h.cursorRow(); !strings.Contains(got, "Blue Hour") {
		t.Fatalf("cursor on %q after 3j, want Blue Hour", got)
	}
	h.keys("<CR>")
	h.contains("Albums › Blue Hour", "Dusk Signal", "<CR> play", "A add to queue", "a play next")

	h.keys("j<CR>")
	if calls := h.fake.Calls(); len(calls) < 2 || calls[0] != "Play(1 2 3)" ||
		calls[1] != "SetNeighbours(1 3)" {
		t.Fatalf("player calls = %q", calls)
	}
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.contains(
		"▶ Low Tide — Aurora Lane · Blue Hour",
		"2/6 in queue",
		"0:42 / 3:05",
		"playing · 6 tracks in the queue",
	)

	// The start of the track was reported, without counting it as played.
	var scrobbles []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "scrobble" {
			scrobbles = append(scrobbles, r.Params.Get("submission"))
		}
	}
	if len(scrobbles) != 1 || scrobbles[0] != "false" {
		t.Errorf("scrobbles = %v, want one submission=false", scrobbles)
	}
}

func TestAlbumTracksFrame(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("4j<CR>") // The Long Haul: two discs, more rows than fit
	h.keys("G")
	h.contains("2-255", "Mile 510")
	golden.RequireEqual(t, h.screen())
}

func TestPlayingFrame(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>")
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.keys("<Tab>")
	golden.RequireEqual(t, h.screen())
}

func TestNarrowShowsOneWindow(t *testing.T) {
	h := newHarness(t, 80, 40, nil)
	h.contains("Albums   <Tab> queue")
	h.lacks("Queue (0)")
	golden.RequireEqual(t, h.screen())
	h.keys("<Tab>")
	h.contains("Queue (0)   <Tab> library", "The queue is empty.")
	h.lacks("Ångström Days")
}

func TestAutoAdvanceAndTransport(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>")
	h.fake.Finish()
	h.pump()
	h.contains("▶ Low Tide", "2/6 in queue")
	if calls := h.fake.Calls(); calls[len(calls)-1] != "SetNeighbours(1 3)" {
		t.Errorf("after the track ended: %q", calls)
	}
	h.keys("<Space>")
	h.contains("⏸ Low Tide")
	h.keys("<Space>3.>")
	want := []string{"Pause", "Resume", "SeekBy(30s)", "Play(2 3 4)", "SetNeighbours(2 4)"}
	if calls := h.fake.Calls(); strings.Join(calls[len(calls)-5:], " ") != strings.Join(want, " ") {
		t.Errorf("calls = %q, want them to end %q", calls, want)
	}
}

func TestAddAndUndo(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<CR>jA")
	h.contains("Queue (1)", "1 track added to the end of the queue (1 in queue) · u undoes")
	h.keys("2a")
	h.contains("Queue (3)", "2 tracks will play next · u undoes")
	h.keys("u")
	h.contains("Queue (1)", "undone · 1 track in the queue · <C-r> redoes")
	h.keys("uu")
	h.contains("The queue is empty.", "nothing to undo")
	if calls := h.fake.Calls(); len(calls) != 0 {
		t.Errorf("adding without playing called the player: %q", calls)
	}
}

func TestMotions(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("G")
	if !strings.Contains(h.cursorRow(), "夜明け") {
		t.Errorf("G → %q", h.cursorRow())
	}
	h.keys("gg")
	if !strings.Contains(h.cursorRow(), "Ångström") {
		t.Errorf("gg → %q", h.cursorRow())
	}
	h.keys("5G")
	if !strings.Contains(h.cursorRow(), "The Long Haul") {
		t.Errorf("5G → %q", h.cursorRow())
	}
	h.keys("g")
	h.contains("waiting for the next key")
	h.keys("<Esc>k")
	if !strings.Contains(h.cursorRow(), "Blue Hour") {
		t.Errorf("<Esc> didn't cancel g; k → %q", h.cursorRow())
	}
	h.keys("'q")
	if h.m.focus != queueWindow {
		t.Error("'q didn't focus the queue")
	}
	h.keys("'b")
	if h.m.focus != libraryWindow {
		t.Error("'b didn't go back to albums")
	}
	h.keys("<CR>h")
	h.contains(" Albums ")
	h.lacks("Albums ›")
}

func TestCommandLine(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":")
	h.contains("<CR> run  <Tab> complete")
	h.typeText("nope")
	h.keys("<CR>")
	h.contains("not a command: nope")
	h.keys(":")
	h.typeText("seek x")
	h.keys("<CR>")
	h.contains(`seek: "x" is not a number of seconds`)
	h.keys(":")
	h.typeText("q")
	h.keys("<CR>")
	if !h.quit {
		t.Error(":q didn't quit")
	}
}

func TestCtrlCQuits(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("<C-c>")
	if !h.quit {
		t.Error("<C-c> didn't quit")
	}
}

func TestHintsOff(t *testing.T) {
	off := false
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) { c.Hints = &off })
	h.lacks("<Tab> other window")
	if lines := strings.Count(h.screen(), "\n") + 1; lines != 40 {
		t.Errorf("screen has %d lines, want 40", lines)
	}
}

func TestFirstRunLogin(t *testing.T) {
	var url string
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		url = srv.URL
		c.Server = config.Server{}
	})
	h.contains("Log in to your Navidrome or Subsonic server", "Server URL", "Username", "Password")
	golden.RequireEqual(t, h.screen())

	h.typeText(url)
	h.keys("<Tab>")
	h.typeText(subsonictest.Username)
	h.keys("<CR>")
	h.typeText("wrong")
	h.keys("<CR>")
	h.contains("wrong username or password")
	h.lacks("wrong\n") // the password is masked

	for range len("wrong") {
		h.keys("<BS>")
	}
	h.typeText(subsonictest.Password)
	h.keys("<CR>")
	h.contains("logged in as admin", "Recently added")
	if p := h.kr[creds.Service+"/"+creds.Key(url, subsonictest.Username)]; p != subsonictest.Password {
		t.Errorf("keyring has %q", p)
	}
	saved, err := config.Load(h.path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Server.URL != url || saved.Server.User != subsonictest.Username ||
		saved.Server.Password != "" {
		t.Errorf("saved server = %+v, want URL and user but no password", saved.Server)
	}
	if info, err := os.Stat(h.path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("config mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestRejectedSavedPasswordOpensForm(t *testing.T) {
	h := newHarness(
		t,
		120,
		40,
		func(c *config.Config, _ *subsonictest.Server) { c.Server.Password = "stale" },
	)
	h.contains("the server rejected the saved username or password", "Password")
	if h.m.login.fields[fieldURL].Value() == "" ||
		h.m.login.fields[fieldUser].Value() != subsonictest.Username {
		t.Error("form not filled in with the saved URL and user")
	}
	if h.m.login.focus != fieldPassword {
		t.Error("focus should be on the password")
	}
}

func TestKeyringFailureOffersConfig(t *testing.T) {
	var url string
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		url = srv.URL
		c.Server = config.Server{URL: srv.URL, User: subsonictest.Username}
	})
	h.m.deps.Keyring = brokenKeyring{}
	h.typeText(subsonictest.Password)
	h.keys("<CR>")
	h.contains("couldn't save the password in the keyring", "instead?")
	h.keys("y")
	saved, _ := config.Load(h.path)
	if saved.Server.Password != subsonictest.Password || saved.Server.URL != url {
		t.Errorf("saved server = %+v, want the password in config", saved.Server)
	}
	h.contains("Recently added")
}

type brokenKeyring struct{}

func (brokenKeyring) Get(string, string) (string, error) { return "", os.ErrNotExist }
func (brokenKeyring) Set(string, string, string) error   { return os.ErrPermission }
func (brokenKeyring) Delete(string, string) error        { return os.ErrPermission }
