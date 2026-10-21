package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func TestFirstRunShowsSyncProgress(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	if !slices.Contains(h.progress, " Syncing library… 500 / 547 tracks") {
		t.Errorf("progress seen = %q", h.progress)
	}
	h.contains("Ångström Days")
	h.lacks("⟳ syncing", "Syncing library")
	if got := h.requests("search3"); got != 2 {
		t.Errorf("first run made %d search3 requests, want 2 pages", got)
	}
	if _, err := os.Stat(library.CachePath(h.cache, h.srv.URL, subsonictest.Username)); err != nil {
		t.Errorf("no cache file after the first sync: %v", err)
	}
}

func TestSecondStartUsesTheCache(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	before := h.requests("search3")
	h2 := h.restart()
	h2.contains("Ångström Days", "The Long Haul")
	if got := h2.requests("search3") - before; got != 0 {
		t.Errorf("second start ran %d search3 requests; the server hasn't rescanned", got)
	}
	// It still refreshes what changes without a scan.
	if h2.requests("getPlaylists") < 2 || h2.requests("getStarred2") < 2 {
		t.Error("second start didn't refresh playlists and stars")
	}
	if slices.ContainsFunc(
		h2.progress,
		func(p string) bool { return strings.Contains(p, "Syncing") },
	) {
		t.Errorf("second start showed sync progress: %q", h2.progress)
	}
}

func TestSyncCommandKeepsTheCursor(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>2j")
	before := h.requests("search3")
	h.keys(":")
	h.typeText("sync")
	h.keys("<CR>")
	if got := h.requests("search3") - before; got != 2 {
		t.Errorf(":sync ran %d search3 requests, want a full sync", got)
	}
	if got := h.cursorRow(); !strings.Contains(got, "Streetlamp") {
		t.Errorf("cursor on %q after :sync, want Streetlamp", got)
	}
	h.keys("h")
	if got := h.cursorRow(); !strings.Contains(got, "Blue Hour") {
		t.Errorf("album cursor on %q after :sync, want Blue Hour", got)
	}
}

func TestNewSnapshotKeepsCursorsByID(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j") // Blue Hour, 4th by sort name
	old := h.m.state.Library
	var blueHour library.Album
	var rest []library.Album
	for _, a := range old.Albums {
		if a.Name == "Blue Hour" {
			blueHour = a
		} else {
			rest = append(rest, a)
		}
	}
	// A sync where Blue Hour now sorts first.
	blueHour.SortName = "aaa"
	fresh := library.Build(
		old.Server,
		old.User,
		old.LastScan,
		old.Artists,
		append(rest, blueHour),
		old.Tracks,
		old.Playlists,
		old.Starred,
	)
	h.send(eventMsg{appSynced(fresh)})
	if h.m.top().cur != 0 || !strings.Contains(h.cursorRow(), "Blue Hour") {
		t.Errorf("cursor at row %d (%q), want Blue Hour at row 0", h.m.top().cur, h.cursorRow())
	}
}

// A Mix asked for during the first sync waits for it, and Home's Mix row
// says so, with the sync's progress.
func TestMixDuringTheFirstSync(t *testing.T) {
	var before, waiting string
	h := setupHarness(t, 120, 40, nil, func(h *harness) {
		h.midSync = func(h *harness) {
			if waiting != "" {
				return
			}
			if before == "" {
				before = h.screen()
				h.keys("gm")
				waiting = h.screen()
			}
		}
	})
	if !strings.Contains(before, "Mix · once the library has synced · 500 / 547 tracks") {
		t.Errorf("Home mid-sync:\n%s", before)
	}
	for _, w := range []string{
		"Mix · starts once the library has synced",
		"the library is syncing · the mix starts once it's done",
	} {
		if !strings.Contains(waiting, w) {
			t.Errorf("after gm mid-sync, screen lacks %q:\n%s", w, waiting)
		}
	}
	h.contains("mixed ")
	if len(h.m.state.Queue) == 0 {
		t.Error("the queue is empty after the sync; the waiting mix didn't run")
	}
	h.lacks("once the library has synced")
}
