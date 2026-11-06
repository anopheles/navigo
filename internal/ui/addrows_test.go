package ui

import "testing"

const blueHour = "Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow"

// A, a and P act on an album or a playlist row as on its tracks, without
// opening it first (#141).
func TestAddAlbumRow(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j")
	h.cursorOn("Blue Hour")
	h.contains("A add to queue")
	h.keys("A")
	h.queueIs(blueHour)
	h.contains("6 tracks added")

	h.keys("u")
	h.queueIs("")
	h.keys("2A") // a count adds it again, as on a track
	if n := len(h.m.state.Queue); n != 12 {
		t.Errorf("2A on an album queued %d tracks, want 12", n)
	}
}

func TestAddNextAndPlayAlbumRow(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3jP")
	h.queueIs(blueHour)
	h.contains("▶  1 Dusk Signal")

	h.keys("'pj") // Morning Mix
	h.cursorOn("Morning Mix")
	h.keys("a")
	if q := h.m.state.Queue; len(q) != 11 || q[1].Track.Title != "Blue" ||
		q[2].Track.Title != "Undertow" {
		t.Errorf(
			"a on a playlist: queue = %s, want its 5 tracks after Dusk Signal",
			h.queueTitles(),
		)
	}
}

// An artist can't be added, and says so instead of doing nothing.
func TestAddArtistRowSaysWhy(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a")
	h.keys("A")
	h.contains("an artist can't be added · open it and add their albums")
	h.queueIs("")
}
