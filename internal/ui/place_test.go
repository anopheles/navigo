package ui

import "testing"

// The queue takes the cursor to a new playing track when it gets the
// focus, and leaves it where the user put it otherwise; long lists count
// their rows on the bottom edge; a second 'h goes to Home's top (#154).
func TestQueueCursorFollowsWhatPlays(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>jj<CR>") // Streetlamp plays
	h.keys("<Tab>")
	h.cursorOn("Streetlamp")
	h.keys("gg<Tab><Tab>") // the same track: the cursor stays put
	h.cursorOn("Dusk Signal")
	h.keys("<Tab>>'q") // another track plays
	h.cursorOn("Blue Hour")
}

func TestLongListCountsItsRows(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("4j<CR>") // The Long Haul: 510 tracks
	h.contains("╰─ 1/510 ─")
	h.keys("G")
	h.contains("╰─ 510/510 ─")
}

func TestSecondHomeMarkGoesToTheTop(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.keys("j<CR><CR>") // Recently added › an album
	h.keys("'b'h")
	if len(h.m.lib) < 3 {
		t.Fatalf("'h didn't return to the place in Home: %d frames", len(h.m.lib))
	}
	h.keys("'h")
	if len(h.m.lib) != 2 {
		t.Errorf("a second 'h left %d frames, want Home's list", len(h.m.lib))
	}
}
