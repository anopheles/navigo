package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/resume"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// savedFile is the saved queue the harness's navigo wrote, if any.
func (h *harness) savedFile() (resume.Saved, error) {
	s := h.m.deps.Config.Server
	return resume.Load(h.state, s.URL, s.User)
}

// playAlbum plays Blue Hour's first track, so the queue holds the album.
func (h *harness) playAlbum() {
	h.t.Helper()
	h.keys("3j<CR><CR>")
	if len(h.m.state.Queue) == 0 {
		h.t.Fatal("nothing queued")
	}
}

func TestQueueComesBackAtTheNextStart(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	queue := h.queueTitles()
	h.keys("jj<CR>") // play the third row
	h.advance(saveAfter)
	saved, err := h.savedFile()
	if err != nil || len(saved.Tracks) != len(h.m.state.Queue) || saved.Current != 2 {
		t.Fatalf("saved = %+v, %v", saved, err)
	}

	n := h.restart()
	n.contains("restored 6 tracks", "plays it")
	if got := n.queueTitles(); got != queue {
		t.Errorf("queue = %q, want %q", got, queue)
	}
	for _, c := range n.fake.Calls() {
		if strings.HasPrefix(c, "Play") {
			t.Errorf("starting played something: %v", n.fake.Calls())
		}
	}
	n.keys(" ")
	if sp := n.fake.Span(); sp.Current.Title == "" || sp.Prev.ID == 0 {
		t.Errorf("<Space> played %+v, want the third row with a track before it", sp)
	}
}

func TestResumeOffKeepsNothing(t *testing.T) {
	h := newHarness(
		t,
		120,
		40,
		func(c *config.Config, _ *subsonictest.Server) { c.Resume.Off = true },
	)
	h.playAlbum()
	h.advance(saveAfter)
	if _, err := h.savedFile(); err == nil {
		t.Error("saved the queue with resume off")
	}
	h.restart().lacks("restored")
}

func TestClearingTheQueueClearsTheSavedOne(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	if _, err := h.savedFile(); err != nil {
		t.Fatal(err)
	}
	h.keys(":queue-clear<CR>")
	if _, err := h.savedFile(); err == nil {
		t.Error("the saved queue outlived the queue")
	}
}

func TestLogoutDeletesTheSavedQueue(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	h.keys(":logout<CR>")
	h.advance(saveAfter)
	if _, err := h.savedFile(); err == nil {
		t.Error(":logout left the saved queue")
	}
}

func TestReloadConfigWithResumeOffDeletesIt(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte("resume: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.playAlbum()
	h.advance(saveAfter)
	if _, err := h.savedFile(); err != nil {
		t.Fatal(err)
	}
	h.keys(":reload-config<CR>")
	if _, err := h.savedFile(); err == nil {
		t.Error("the saved queue stayed after resume: false")
	}
	h.keys("A") // changes the queue, which is no longer kept
	h.advance(saveAfter)
	if _, err := h.savedFile(); err == nil {
		t.Error("the queue was saved with resume off")
	}
}

func TestFlushSavedWritesAtQuit(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	if _, err := h.savedFile(); err == nil {
		t.Fatal("written before the delay")
	}
	h.m.FlushSaved()
	if s, err := h.savedFile(); err != nil || len(s.Tracks) == 0 {
		t.Errorf("after FlushSaved: %+v, %v", s, err)
	}
}

func TestResumeProblemShows(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Resume.Problems = []config.Problem{
			{Pos: config.Pos{Line: 4, Col: 9}, Msg: "resume: give true or false"},
		}
	})
	h.contains("config.yaml has problems", "config.yaml:4:9: resume: give true or false")
}

func TestReplayGainSurvivesANewLogin(t *testing.T) {
	cfg := config.Config{}
	cfg.Player.ReplayGain = "album"
	s := newState(cfg)
	if s.ReplayGain.Mode != "album" || !s.ResumeOn {
		t.Errorf("newState = %+v, resume %v", s.ReplayGain, s.ResumeOn)
	}
}
