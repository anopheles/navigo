package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/keymap"
)

func TestSearchTyping(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'s")
	h.contains("search: ", "type to search the library", "<CR> go to the results")

	// Results come live from the first letter.
	h.typeText("b")
	h.contains("Search “b”", "── Albums · ")
	h.typeText("lue")
	h.contains("Search “blue”", "── Albums · 3 ──", "── Tracks · ")
	h.lacks("── Artists")
	golden.RequireEqual(t, h.screen())

	// <CR> goes into the results, on the best match.
	h.keys("<CR>")
	if h.m.mode != keymap.Normal || h.m.focus != libraryWindow {
		t.Fatalf("mode %s, focus %d after <CR>", h.m.mode, h.m.focus)
	}
	h.cursorOn("Blue")
	h.cursorOn("Aurora Lane")
	h.keys("}")
	h.cursorOn("Blue") // the track called Blue: headings are skipped
	h.keys("{{")
	h.cursorOn("Aurora Lane")
	h.keys("<CR>")
	h.contains("Search “blue” › Blue", "Tidewater")
	h.keys("h")
	h.contains("Search “blue”", "── Tracks")
}

func TestSearchEscRestores(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j")
	h.keys("'s")
	h.typeText("zz")
	h.contains(`nothing in the library matches "zz"`)
	h.keys("<Esc>")
	h.contains(" Albums ")
	h.lacks("Search")
	h.cursorOn("Blue Hour")
	if h.m.mode != keymap.Normal {
		t.Errorf("mode %s after <Esc>", h.m.mode)
	}
}

func TestSearchLastQueryAndHistory(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'s")
	h.typeText("blue")
	h.keys("<CR>'b's")
	// The last query is back, selected: typing replaces it.
	h.contains("search: blue", "Search “blue”")
	h.typeText("quiet")
	h.contains("search: quiet", "The Quiet Ones")
	h.keys("<CR>")

	h.keys("'s<Up>")
	h.contains("search: blue")
	h.keys("<Esc>'s")
	h.send(tea.PasteMsg{Content: "angst"})
	h.contains("search: angst", "Ångström Days")
	h.keys("<Up>")
	h.contains("search: blue")
	h.keys("<Down>")
	h.contains("search: quiet")
	h.keys("<Esc>")

	// The history outlives the session.
	n := h.restart()
	n.keys("'s")
	n.contains("search: quiet")
	n.keys("<Up><Up>")
	n.contains("search: blue")
}

func TestSearchCapsAndMore(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search mile<CR>")
	h.contains("── Tracks · 510 ──")
	h.cursorOn("Mile 001")
	h.keys("G")
	h.cursorOn("… 310 more")
	h.keys("<CR>")
	h.cursorOn("Mile 201")
	h.keys("G")
	h.cursorOn("Mile 510")
	h.lacks("more")
}

func TestSearchRows(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	// A track plays within its album.
	h.keys(":search low tide<CR>")
	h.cursorOn("Low Tide")
	h.keys("<CR>")
	h.contains("▶ Low Tide", "2/6 in queue")

	// ga goes to its album, on the track.
	h.keys("ga")
	h.contains("Albums › Blue Hour")
	h.cursorOn("Low Tide")

	// a/A add a whole album, or just a track.
	h.keys(":search blue hour<CR>")
	h.cursorOn("Blue Hour")
	h.cursorOn("2021") // the album, before the track of that name
	h.keys("A")
	h.contains("6 tracks added to the end of the queue (12 in queue)")
	h.keys("}A")
	h.contains("1 track added to the end of the queue (13 in queue)")

	// Artists open; adding one says why not.
	h.keys(":search pell<CR>")
	h.cursorOn("Pell Mell")
	h.keys("A")
	h.contains("an artist can't be added")
	h.keys("<CR>")
	h.contains("Search “pell” › Pell Mell", "── Appears on ──")
	h.keys("gg<CR>")
	h.cursorOn("Night Drive")

	// From the section list, Search opens the prompt.
	h.keys("'b'bh")
	h.cursorOn("Albums")
	h.keys("j<CR>")
	if h.m.mode != keymap.Search {
		t.Errorf("mode %s, want the search prompt", h.m.mode)
	}
}

func TestSearchWhileSyncing(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.m.state.Library = nil
	h.m.state.Sync.Running = true
	h.keys("'s")
	h.typeText("blue")
	h.contains(`nothing in the library matches "blue"`, "still syncing, results may be incomplete")
}

func TestSearchAccents(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search angstrom<CR>")
	h.cursorOn("Ångström")
	if !strings.Contains(h.screen(), "Ångström Days") {
		t.Error("album not found without its accents")
	}
}
