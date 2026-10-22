package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// gm mixes around the row under the cursor, else what plays, else the
// whole library; :mix library and Home's Mix row always mix the library.
func TestMixAroundTheRow(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)

	// Home's lists can't be seeds, and nothing plays: the whole library.
	h.keys("j5gm")
	h.contains("mixed 5 tracks")
	h.lacks("tracks from")

	// An artist row is the seed.
	h.keys("'a")
	artist := h.m.artistsOf(h.m.top())[0]
	h.keys("5gm")
	h.contains(" of 5 tracks from " + artist.Name + " · u brings the old queue back")
	if got := h.m.state.Queue[0].Track; got.ArtistID != artist.ID && !got.Credits(artist.ID) {
		t.Errorf("a Mix around %s opens with %s by %s", artist.Name, got.Title, got.Artist)
	}

	// :mix library ignores the row.
	h.keys(":mix library<CR>")
	h.contains("of 50 tracks · u brings")
	h.lacks("tracks from")

	// A queue row is a track seed, and plays first.
	h.keys("<Tab>j")
	want := h.m.state.Queue[1].Track
	h.keys("gm")
	h.contains("from " + want.Title)
	if got := h.m.state.Queue[0].Track.ID; got != want.ID {
		t.Errorf("a Mix around a track opens with %s, want %s", got, want.ID)
	}

	// On a row that can't be a seed, what plays is the seed.
	h.keys("<Tab>'h")
	playing := h.m.state.Queue[h.m.state.Current].Track
	h.keys("jgm")
	h.contains("from " + playing.Title)

	// Home's Mix row always mixes the whole library.
	h.keys("gg<CR>")
	h.contains("of 50 tracks · u brings")
	h.lacks("tracks from")
}

func TestMixLibraryCompletes(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys(":mix li<Tab>")
	if got := h.m.line.Value(); got != "mix library" {
		t.Errorf("completed to %q", got)
	}
	h.keys("<Esc>:mix everything<CR>")
	h.contains("mix: give library, or nothing to mix around the row")
}

// Online, a Mix around a seed asks the server for its artists' similar
// artists; offline it doesn't, and is made from the cache at once.
func TestMixAroundAsksForSimilarArtists(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.keys("'a")
	artist := h.m.artistsOf(h.m.top())[0]
	h.keys("gm")
	h.contains("tracks from " + artist.Name)
	var asked []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "getArtistInfo2" {
			asked = append(asked, r.Params.Encode())
		}
	}
	if want := "count=20&id=" + string(artist.ID); len(asked) != 1 || asked[0] != want {
		t.Errorf("similar-artist requests = %q, want [%s]", asked, want)
	}

	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("offline")
	h.keys("u") // the old queue back, so the next Mix's message is fresh
	h.keys("gm")
	h.contains("tracks from " + artist.Name)
	if n := h.requests("getArtistInfo2"); n != 1 {
		t.Errorf("offline, %d similar-artist requests in all, want still 1", n)
	}
}

// The hint line offers gm only where it has a seed; the key menu names
// what gm would mix from; the empty queue offers it for the whole library.
func TestMixIsDiscoverable(t *testing.T) {
	h := homeHarness(t, 160, 40, nil)
	h.contains("Or let navigo pick:", "gm mix from the whole library")

	h.keys("j") // a Home list: not a seed
	if hl := ansi.Strip(h.m.hintLine()); strings.Contains(hl, "gm mix") {
		t.Errorf("hint line on a Home list offers gm: %q", hl)
	}
	h.keys("?")
	h.contains("mix from the whole library")
	h.keys("q'a")
	artist := h.m.artistsOf(h.m.top())[0]
	if hl := ansi.Strip(h.m.hintLine()); !strings.Contains(hl, "gm mix") {
		t.Errorf("hint line on an artist lacks gm: %q", hl)
	}
	h.keys("?")
	h.contains("mix from " + artist.Name)
	h.keys("q")

	// With something playing, a row that is no seed mixes from it.
	h.keys("gm'hj?")
	playing := h.m.state.Queue[h.m.state.Current].Track
	h.contains("mix from " + playing.Title)
	h.keys("q")

	h.keys("'?/mix<CR>")
	h.contains("Mix around the row, else what plays, else the whole library")
}
