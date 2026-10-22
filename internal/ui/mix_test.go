package ui

import "testing"

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
