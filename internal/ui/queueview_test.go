package ui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

const twoAlbums = "Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
	"Undertow, Quiet Room, Blue Static, Radio Silence, Homeward"

// Blue Hour plays (Dream Pop) from Dusk Signal; The Quiet Ones' Blue
// (Indie) follows.
func queueOfTwoAlbums(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>hk<CR>VGA<Tab>")
	h.queueIs(twoAlbums)
	return h
}

func TestSortQueue(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys(":sort title<CR>")
	h.contains("queue sorted by title · u undoes")
	// What plays stays first.
	h.queueIs("Dusk Signal, Afterglow, Blue Hour, Blue Static, Homeward, Low Tide, " +
		"Quiet Room, Radio Silence, Streetlamp, Undertow, Wintering")
	h.contains("▶  1 Dusk Signal")
	h.keys("u")
	h.queueIs(twoAlbums)
	h.keys(":sort<CR>")
	h.contains("the queue has no usual order · u undoes the sort")
	h.keys(":sort bogus<CR>")
	h.contains(
		`sort: "bogus" isn't a field here; try title, artist, album, year, plays, length, genre`,
	)
}

func TestFilterQueue(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys(":filter genre indie<CR>")
	h.contains("Queue (6 of 11) · genre indie", "6 shown")
	h.lacks("Low Tide", "Wintering")
	h.contains("▶  1 Dusk Signal") // playing, though it doesn't match
	h.cursorOn("Dusk Signal")

	h.keys("jjdd") // Quiet Room
	h.contains("1 track removed (10 in queue)")
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Undertow, Blue Static, Radio Silence, Homeward")
	h.cursorOn("Blue Static")
	h.keys("Vjd")
	h.queueIs(
		"Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, Undertow, Homeward",
	)
	h.cursorOn("Homeward")
	h.keys("P") // back before the row shown
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Undertow, Blue Static, Radio Silence, Homeward")
	h.keys("J")
	h.contains("filtered view · :filter alone to move tracks")

	h.keys("ggj<CR>") // plays the row shown
	h.contains("▶  7 Undertow")

	h.keys("G<Tab>gp") // the row that plays
	h.cursorOn("Undertow")
	h.keys(":filter year 1900<CR>")
	h.contains("Queue (1 of 10)", "▶  7 Undertow") // only what plays
	h.keys(":filter<CR>")
	h.contains("filter cleared", "Queue (10)", "Low Tide")
	h.cursorOn("Undertow") // the cursor stays on its entry
}

// Rows added below the view leave the rows on screen as they were, so the
// bottom edge has to say they are there.
func TestQueueEdgeCountsRowsOffScreen(t *testing.T) {
	h := homeHarness(t, 120, 16, nil)
	h.keys(":mix-add-end library 3<CR>")
	h.lacks("↑", "more ─")

	h.keys(":mix-add-end library 11<CR><Tab>G")
	h.contains("╰─ ↑ 4 ─")
	h.lacks("more ─")

	h.keys(":mix-add-end library 10<CR>")
	h.contains("╰─ ↑ 4 · ↓ 10 more ─")

	h.keys("gg")
	h.contains("╰─ ↓ 14 more ─")
}

func TestQueueFoot(t *testing.T) {
	for _, c := range []struct {
		off, h, n int
		want      string
	}{
		{0, 10, 5, ""},
		{0, 10, 10, ""},
		{0, 10, 11, " ↓ 1 more "},
		{1, 10, 11, " ↑ 1 "},
		{4, 10, 24, " ↑ 4 · ↓ 10 more "},
	} {
		if got := queueFoot(c.off, c.h, c.n); got != c.want {
			t.Errorf("queueFoot(%d, %d, %d) = %q, want %q", c.off, c.h, c.n, got, c.want)
		}
	}
}

// A window too narrow for the count keeps its width.
func TestQueueEdgeFitsANarrowWindow(t *testing.T) {
	m := &Model{}
	for _, w := range []int{2, 3, 6, 10, 40} {
		lines := m.paneFooted("", " ↑ 4 · ↓ 10 more ", nil, w, false)
		if got := lipgloss.Width(lines[len(lines)-1]); got != w {
			t.Errorf("width %d: the edge is %d wide", w, got)
		}
	}
}

// In the queue a group is a run of one album's tracks: } goes to the next
// album, and d} takes the rest of this one, not the whole queue (#140).
func TestQueueGroupsAreAlbums(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys("gg}")
	h.cursorOn("Undertow")
	h.keys("}")
	h.cursorOn("Homeward") // past the last group: the end
	h.keys("{")
	h.cursorOn("Undertow")
	h.keys("{")
	h.cursorOn("Dusk Signal")
	h.keys("2}")
	h.cursorOn("Homeward")

	h.keys("ggjd}")
	h.queueIs("Dusk Signal, Undertow, Quiet Room, Blue Static, Radio Silence, Homeward")
	h.keys("u")
	h.queueIs(twoAlbums)
	h.keys("gg7jd{") // from Quiet Room back to Undertow; Quiet Room stays
	h.queueIs("Dusk Signal, Low Tide, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Quiet Room, Blue Static, Radio Silence, Homeward")
	h.keys("ggd}d}") // the last group: to the end
	h.queueIs("")
}

// On an album of several discs, } goes to the next disc.
func TestAlbumGroupsAreDiscs(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("4j<CR>") // The Long Haul: two discs
	h.keys("}")
	if row := h.cursorRow(); !strings.Contains(row, "2-01") {
		t.Errorf("} on a two-disc album went to %q, want disc 2's first track", row)
	}
	h.keys("{")
	if row := h.cursorRow(); !strings.Contains(row, "1-01") {
		t.Errorf("{ went to %q, want disc 1's first track", row)
	}
}
