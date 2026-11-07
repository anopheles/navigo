package ui

import (
	"slices"
	"testing"
	"time"
)

// X keeps the place: <Space> plays on from where it stopped (#151).
func TestStopKeepsThePlace(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>") // Low Tide
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.keys("X")
	h.contains("stopped at 0:42 · <Space> plays on from there")
	h.keys("<Space>")
	if !slices.Contains(h.fake.Calls(), "Play(1 2 3 from 42s)") {
		t.Errorf("no resume at 42s in %q", h.fake.Calls())
	}
}

// <CR> on the row that plays pauses it rather than start it over.
func TestEnterOnThePlayingRowPauses(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>")
	h.fake.Tick(12 * time.Second)
	h.pump()
	h.keys("<Tab>gp<CR>")
	if c := h.lastCall(); c != "Pause" {
		t.Errorf("<CR> on the playing row: last call %q, want Pause", c)
	}
}

// u says what it undid and takes a count; right after a change it can't
// undo, it says so first.
func TestUndoSaysWhatAndCounts(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3jAjAjA") // three albums, three edits
	n := len(h.m.state.Queue)
	h.keys("2u")
	if len(h.m.state.Queue) >= n || h.m.state.CanUndo() == false {
		t.Errorf("2u: %d tracks, want one album left", len(h.m.state.Queue))
	}
	h.contains("undone: ")
	h.keys("*") // a star: not something u undoes
	h.keys("u")
	h.contains("u undoes changes to the queue only · u again undoes the last one")
	before := len(h.m.state.Queue)
	h.keys("u")
	if len(h.m.state.Queue) == before {
		t.Error("the second u didn't undo")
	}
}

func TestCountsOnNextTrack(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR><CR>2>")
	h.contains("▶  3 Streetlamp")
}
