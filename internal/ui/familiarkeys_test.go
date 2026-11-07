package ui

import "testing"

// The keys people reach for outside vim: arrows, Backspace, Esc, Delete,
// PgUp/PgDn, Home/End (#142).
func TestArrowsAndBackspaceGoBackAndOpen(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<Right>")
	h.onPath("Albums › Blue Hour")
	h.keys("<Left>")
	h.lacks("Albums ›")
	h.cursorOn("Blue Hour")
	h.keys("<Right><BS>")
	h.lacks("Albums ›")
	h.keys("<Right><Esc>")
	h.lacks("Albums ›")
}

// <Esc> cancels first, and goes back only when there is nothing to cancel.
func TestEscCancelsThenGoesBack(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>Vj<Esc>")
	if h.m.visual != nil {
		t.Fatal("<Esc> left Visual mode on")
	}
	h.onPath("Albums › Blue Hour")
	h.keys(":nope<CR>")
	h.contains("not a command: nope")
	h.keys("<Esc>")
	h.lacks("not a command: nope")
	h.onPath("Albums › Blue Hour")
	h.keys("<Esc>")
	h.lacks("Albums ›")
}

func TestHomeEndPageKeys(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>") // Blue Hour: 6 tracks
	h.keys("<End>")
	h.cursorOn("Afterglow")
	h.keys("<Home>")
	h.cursorOn("Dusk Signal")
	h.keys("<PageDown>")
	h.cursorOn("Afterglow")
	h.keys("<PageUp>")
	h.cursorOn("Dusk Signal")
}

func TestDeleteRemovesFromQueue(t *testing.T) {
	h := queueOfTwoAlbums(t)
	h.keys("gg<Down><Del>")
	h.contains("1 track removed (10 in queue)")
	h.queueIs("Dusk Signal, Streetlamp, Blue Hour, Wintering, Afterglow, " +
		"Undertow, Quiet Room, Blue Static, Radio Silence, Homeward")
}
