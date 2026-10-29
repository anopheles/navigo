package ui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/golden"
)

// header is the first line of a column: its name.
func (h *harness) header(depth int) string {
	h.t.Helper()
	return strings.TrimSpace(strings.SplitN(h.column(depth), "\n", 2)[0])
}

// Artists | Albums | Tracks: the columns right of the focus follow its
// cursor, and the queue is a <Tab> away.
func TestArtistColumnsFollowTheCursor(t *testing.T) {
	for _, w := range []int{80, 120} {
		h := newHarness(t, w, 30, nil)
		h.keys("'a")
		h.screen()
		if !h.m.inColumns() || len(h.m.colAt) != 3 {
			t.Fatalf("%d cols: %d columns, want 3", w, len(h.m.colAt))
		}
		h.contains("Artists   <Tab> queue")
		h.lacks("Queue (0)", "Artists ›")
		if h.header(2) != "Aurora Lane" || h.header(3) != "Blue" {
			t.Errorf("%d cols: previews %q, %q", w, h.header(2), h.header(3))
		}
		h.keys("j") // Long Haul Orchestra: its album and its tracks follow
		if h.header(2) != "Long Haul Orchestra" || h.header(3) != "The Long Haul" {
			t.Errorf("%d cols: after j, previews %q, %q", w, h.header(2), h.header(3))
		}
		if len(h.m.lib) != 2 {
			t.Errorf("%d cols: moving opened a list: depth %d", w, len(h.m.lib))
		}
		t.Run(map[int]string{80: "80", 120: "120"}[w], func(t *testing.T) {
			golden.RequireEqual(t, h.screen())
		})
		h.keys("<Tab>")
		h.contains("Queue (0)   <Tab> library", "The queue is empty.")
		h.lacks("Long Haul Orchestra")
	}
}

// l and h move the focus between columns; a column left forgets its
// cursor, and a preview shows none.
func TestColumnFocusMoves(t *testing.T) {
	h := newHarness(t, 120, 30, nil)
	h.keys("'al") // Aurora Lane's albums
	h.onPath("Artists › Aurora Lane")
	h.cursorOn("Blue")
	h.keys("jl") // Blue Hour's tracks
	h.onPath("Artists › Aurora Lane › Blue Hour")
	h.keys("jjh") // back to the albums, on Blue Hour
	h.cursorOn("Blue Hour")
	if h.header(3) != "Blue Hour" {
		t.Errorf("the tracks column is %q, want Blue Hour's preview", h.header(3))
	}
	h.keys("l") // the tracks open at the top again
	h.cursorOn("Dusk Signal")

	// Below 60 cells, one list at a time as before.
	n := newHarness(t, 59, 30, nil)
	n.keys("'al")
	if n.m.inColumns() {
		t.Error("columns at 59 cells")
	}
	n.contains("Artists › Aurora Lane")
}

// A track row under 40 cells keeps its number and length and drops the
// artist; an artist's albums show their year, not the artist.
func TestNarrowColumnRows(t *testing.T) {
	h := newHarness(t, 80, 30, nil)
	h.keys("'a")
	tracks := h.column(3)
	if !strings.Contains(tracks, "1 Tidewater") || !strings.Contains(tracks, "3:05") ||
		strings.Contains(tracks, "Aurora Lane  ") {
		t.Errorf("tracks column:\n%s", tracks)
	}
	albums := h.column(2)
	if !strings.Contains(albums, "Blue Hour") || !strings.Contains(albums, "2021") {
		t.Errorf("albums column:\n%s", albums)
	}
}

// A sort or filter belongs to its column: the next artist's albums open
// sorted too, and their preview shows them so.
func TestColumnSortStays(t *testing.T) {
	h := newHarness(t, 120, 30, nil)
	h.keys("'al:sort year desc<CR>")
	if got := h.header(2); got != "Aurora Lane (year ↓)" {
		t.Errorf("header %q", got)
	}
	h.contains("Artists (year ↓)")
	h.keys("hjjj") // The Quiet Ones: Static Bloom 2018, Blue 2015
	if got := orderIn(
		h.column(2),
		"Static Bloom",
		"Blue",
	); strings.Join(
		got,
		",",
	) != "Static Bloom,Blue" {
		t.Errorf("preview in order %q", got)
	}
	h.keys("l")
	h.onPath("Artists › The Quiet Ones (year ↓)")
	h.cursorOn("Static Bloom")
	h.keys(":sort<CR>hl")
	h.onPath("Artists › The Quiet Ones")
}

// Playlists | Tracks: an empty playlist's preview says so in a word.
func TestPlaylistColumns(t *testing.T) {
	h := newHarness(t, 80, 24, nil)
	h.keys("'p")
	if h.header(1) != "Playlists" || h.header(2) != "Empty" {
		t.Fatalf("columns %q, %q", h.header(1), h.header(2))
	}
	if !strings.Contains(h.column(2), "empty") {
		t.Errorf("an empty preview:\n%s", h.column(2))
	}
	h.keys("l")
	h.contains("empty playlist")
	h.keys("hj")
	if h.header(2) != "Morning Mix" || !strings.Contains(h.column(2), "Undertow") {
		t.Errorf("Morning Mix's preview:\n%s", h.column(2))
	}
	golden.RequireEqual(t, h.screen())
}

// A click focuses a column and moves its cursor; in a preview it opens it.
// The wheel scrolls the column under it.
func TestClickAColumn(t *testing.T) {
	h := newHarness(t, 120, 30, nil)
	h.keys("'a")
	h.screen()
	albums := h.m.colAt[1].x + 2
	h.send(click1(albums, 3)) // Blue Hour, the second row of the preview
	h.onPath("Artists › Aurora Lane")
	h.cursorOn("Blue Hour")
	h.screen()
	h.send(click1(h.m.colAt[0].x+2, 3)) // Long Haul Orchestra: the albums close
	if len(h.m.lib) != 2 {
		t.Errorf("depth %d after a click on the artists", len(h.m.lib))
	}
	h.cursorOn("Long Haul Orchestra")
	h.advance(100 * time.Millisecond)
	h.send(click1(h.m.colAt[0].x+2, 3)) // twice: <CR> opens the artist
	h.onPath("Artists › Long Haul Orchestra")

	h.screen()
	tracks := h.m.colAt[2].x + 2
	h.send(tea.MouseWheelMsg{X: tracks, Y: 5, Button: tea.MouseWheelDown})
	h.onPath("Artists › Long Haul Orchestra › The Long Haul")
	if f := h.m.top(); f.offset != wheelRows {
		t.Errorf("the wheel scrolled the tracks to %d, want %d", f.offset, wheelRows)
	}
}
