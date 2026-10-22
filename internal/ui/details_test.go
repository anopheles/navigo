package ui

import (
	"strings"
	"testing"
	"time"
)

func TestVolumeKeys(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("-")
	h.contains("volume 95%", "vol 95%")
	if c := h.lastCall(); c != "SetVolume(95)" {
		t.Errorf("player call %q", c)
	}
	h.keys("3=")
	h.contains("volume 100%")
	h.lacks("vol 100%") // full volume isn't flagged
	h.keys(":volume 40<CR>")
	if c := h.lastCall(); c != "SetVolume(40)" {
		t.Errorf("player call %q", c)
	}
	h.keys("+")
	h.contains("vol 45%")
}

func TestShuffleAndRepeatKeys(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.m.seedFn = func() uint64 { return 3 }
	h.keys("3j<CR>jj<CR>") // Blue Hour from Streetlamp
	h.keys("S")
	h.contains("queue shuffled · u undoes")
	if got := h.queueTitles(); !strings.HasPrefix(got, "Streetlamp, ") ||
		got == "Streetlamp, Blue Hour, Wintering, Afterglow, Dusk Signal, Low Tide" {
		t.Errorf("shuffled queue %s", got)
	}
	h.keys("u")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow")

	h.keys("R")
	h.contains("repeat all")
	h.keys("R")
	h.contains("repeat one")
	if c := h.lastCall(); c != "SetNeighbours(2 0)" {
		t.Errorf("under repeat one: %s", c)
	}
	// The track ends and plays again.
	h.fake.Finish()
	h.pump()
	h.contains("▶  Streetlamp", "3/6 in queue")
	h.keys("R")
	h.contains("repeat off")
	h.lacks("repeat one", "repeat all")
}

func TestInfoPopup(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j")
	h.keys("i")
	h.contains("Blue Hour", "Aurora Lane", "2021", "6 · ", "Starred    ★ yes")
	h.keys("q<CR>j:info<CR>")
	h.contains(
		"Low Tide",
		"Artists    Aurora Lane",
		"Track      2 on disc 1",
		"Length     3:05",
		"Starred    no",
	)
	h.keys("q")

	// A track with two artists lists both; an artist shows its counts.
	h.keys(":search blue highway<CR>}i")
	h.contains("Artists    Pell Mell, Zoë Brandt")
	h.keys("q:search pell<CR>i")
	h.contains("Albums     0", "Appears on 1")
	h.keys("q")

	// With nothing under the cursor, it shows what plays.
	h.keys("'b'bgg4j<CR><CR>'pi")
	h.contains("Mile 001")
}

func TestStarring(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j*")
	h.contains("starred Low Tide", "★ Low Tide")
	h.keys("*")
	h.contains("unstarred Low Tide")
	h.lacks("★ Low Tide")
	// A selection of albums; an artist.
	h.keys("hggVj*")
	h.contains("starred 2 items")
	h.keys(":search quiet<CR>")
	h.cursorOn("Quiet Room")
	h.keys("gg*")
	h.contains("unstarred The Quiet Ones") // starred in the fixtures

	var got []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "star" || r.Endpoint == "unstar" {
			got = append(got, r.Endpoint+" "+r.Params.Encode())
		}
	}
	lib := h.m.state.Library
	lowTide := string(lib.AlbumTracks(lib.Albums[3].ID)[1].ID)
	want := []string{
		"star id=" + lowTide,
		"unstar id=" + lowTide,
		"star albumId=" + string(lib.Albums[0].ID) + "&albumId=" + string(lib.Albums[1].ID),
		"unstar artistId=" + string(lib.Artists[3].ID),
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	// Offline, starring is refused.
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.advance(time.Second)
	h.keys("*")
	h.contains("offline · stars can be changed once the server is back")
}

func TestRating(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j:rate 4<CR>")
	h.contains("rated Low Tide ★★★★")
	h.keys("i")
	h.contains("Rating     ★★★★☆")
	h.keys("q:rate 0<CR>")
	h.contains("rating cleared for Low Tide")
	h.keys("i")
	h.contains("Rating     not rated")
	h.keys("q")

	// A selection; refusals.
	h.keys("ggVj:rate 5<CR>")
	h.contains("rated 2 tracks ★★★★★")
	h.keys(":rate 6<CR>")
	h.contains("rate: give 1 to 5 stars, or 0 to clear: rate 4")
	h.keys("h:rate 3<CR>")
	h.contains("only tracks can be rated")

	var got []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "setRating" {
			got = append(got, r.Params.Encode())
		}
	}
	lib := h.m.state.Library
	ts := lib.AlbumTracks(lib.Albums[3].ID)
	want := []string{
		"id=" + string(ts[1].ID) + "&rating=4",
		"id=" + string(ts[1].ID) + "&rating=0",
		"id=" + string(ts[0].ID) + "&rating=5",
		"id=" + string(ts[1].ID) + "&rating=5",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if t0, _ := lib.Track(ts[0].ID); t0.Rating != 5 {
		t.Errorf("the library has rating %d", t0.Rating)
	}

	// Offline, rating is refused.
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.advance(time.Second)
	h.keys("<CR>:rate 2<CR>")
	h.contains("offline · ratings can be changed once the server is back")
}
