package ui

import (
	"strings"
	"testing"
	"time"
)

// Off a track, ga, gr and * act on what plays, as gm and i do, and a
// starred track shows ★ in the now-playing bar (#156).
func TestKeysFallBackToWhatPlays(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>'h") // Low Tide plays; Home has no track under the cursor
	h.keys("ga")
	h.onPath("Albums › Blue Hour")
	h.keys("'hgr")
	h.contains("Aurora Lane")
	h.keys("'h*")
	h.contains("starred Low Tide")
	bar := lineWith(t, h.screen(), "0:00 /")
	if !strings.Contains(bar, "★ Low Tide") {
		t.Errorf("the bar lacks the star: %q", bar)
	}
}

// Every Home list gives its count, and the Mix row says what it does.
func TestHomeLabelsSayWhatTheyAre(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.contains("Mix · plays tracks picked for you, in place of the queue")
	h.homeRow("Recently added")
	h.keys("<CR>")
	h.contains("Home › Recently added (")
}

// Never played leaves out what the server says played, though the cache's
// counts lag behind.
func TestNeverPlayedTrustsTheServer(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.homeRow("Most played")
	h.keys("<CR>")
	most := h.albumNames()
	h.keys("h")
	for i := range h.m.state.Library.Albums { // the cache knows of no plays
		a := &h.m.state.Library.Albums[i]
		a.PlayCount = 0
	}
	for i := range h.m.state.Library.Tracks {
		h.m.state.Library.Tracks[i].PlayCount = 0
		h.m.state.Library.Tracks[i].Played = time.Time{}
	}
	h.homeRow("Never played")
	h.keys("<CR>")
	for _, n := range h.albumNames() {
		for _, p := range most {
			if n == p {
				t.Errorf("%s is in Most played and Never played", n)
			}
		}
	}
}
