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
	h.contains("▶  Low Tide", " 2/6 ")

	// ga goes to its album, on the track.
	h.keys("ga")
	h.onPath("Albums › Blue Hour")
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
	h.keys("jj<CR>") // past Playlists
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

func TestSearchFindsPlaylists(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search morning<CR>")
	h.contains("Playlists · 1", "Morning Mix", "5 tracks")
	h.cursorOn("Morning Mix") // the best match

	// <CR> opens the playlist's tracks.
	h.keys("<CR>")
	h.contains("Search “morning” › Morning Mix", "Undertow", "朝霧")
	h.keys("h")

	// a/A add every track; * and dd leave a playlist alone.
	h.keys("A")
	h.contains("5 tracks added to the end of the queue (5 in queue)")
	h.keys("*")
	h.contains("playlists can't be starred")
	h.keys("dd")
	h.lacks("delete")
	if n := len(h.m.state.Library.Playlists); n != 2 {
		t.Errorf("%d playlists left, want 2", n)
	}
	if len(h.writes()) != 0 {
		t.Errorf("a key in Search wrote to the server: %v", h.writes())
	}

	// In Visual mode too.
	h.keys("Va")
	h.contains("Queue (10)")
}

func TestSearchFindsAPlaylistMadeLater(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":playlist-new Gym Rotation<CR>")
	h.keys(":search gym<CR>")
	h.contains("Playlists · 1", "Gym Rotation")
}

// Search results sort and filter group by group.
func TestSortAndFilterSearch(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":search blue<CR>:sort year desc<CR>")
	h.contains("Search “blue” (year ↓)", "sorted by year")
	want := []string{"2021", "2019", "2015", "Blue Highway"} // albums, then the newest track
	if got := strings.Join(h.order(want...), ","); got != strings.Join(want, ",") {
		t.Errorf("rows in order %q", got)
	}
	h.keys(":filter genre indie<CR>")
	h.contains(
		"Search “blue” (year ↓ · genre indie)",
		"── Albums · 1 ──",
		"── Tracks · 5 ──",
		"6 shown",
	)
	h.lacks("Blue Hour", "Blue Highway")

	// Artists have no genre or year: a filter leaves them out.
	h.keys(":search aurora<CR>")
	h.contains("Search “aurora” (year ↓ · genre indie)")
	h.lacks("── Artists")
	h.keys(":filter<CR>")
	h.contains("── Artists · 1 ──", "Aurora Lane")
	h.keys(":sort plays<CR>") // artists have no plays: they keep their order
	h.contains("── Artists · 1 ──")
	h.keys(":sort bogus<CR>")
	h.contains(
		`sort: "bogus" isn't a field here; try name, artist, album, year, plays, tracks, albums, length, genre`,
	)
}
