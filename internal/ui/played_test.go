package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// playedTitles is the titles of the tracks played this session.
func (h *harness) playedTitles() []string {
	var out []string
	for _, t := range h.m.state.Played {
		out = append(out, t.Title)
	}
	return out
}

// listTitles is the titles of the open Home track list's rows.
func (h *harness) listTitles() []string {
	var out []string
	for _, t := range h.m.homeTracksOf(h.m.top()) {
		out = append(out, t.Title)
	}
	return out
}

func TestPlayedThisSession(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()  // Blue Hour from its first track
	h.keys(">")    // then the second
	h.keys("<lt>") // and the first again: it moves back to the top
	h.pump()
	want := []string{"Dusk Signal", "Low Tide"}
	if got := h.playedTitles(); !slices.Equal(got, want) {
		t.Fatalf("played %q, want %q", got, want)
	}

	h.keys(":played<CR>")
	h.contains("Home › Played this session (2)")
	if got := h.listTitles(); !slices.Equal(got, want) {
		t.Errorf("rows %q, want %q", got, want)
	}
	golden.RequireEqual(t, h.screen())

	// A track starting keeps the cursor on its row as the list grows.
	h.keys("j")
	if row := h.cursorRow(); !strings.Contains(row, "Low Tide") {
		t.Fatalf("cursor on %q", row)
	}
	h.keys("'qG<CR>'h")
	h.pump()
	h.keys(":played<CR>")
	h.contains("Played this session (3)")
	h.keys("j")
	if row := h.cursorRow(); !strings.Contains(row, "Dusk Signal") {
		t.Errorf("cursor on %q, want the row under Low Tide's old place", row)
	}

	// <CR> plays the list from that row, as any track list does.
	h.keys("G<CR>")
	if q := h.m.state.Queue; len(q) != 3 || q[2].Track.Title != "Low Tide" {
		t.Errorf("queue = %v, want the list", q)
	}
}

func TestPlayedKeepsTheCursorOnItsTrack(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.keys(">")
	h.pump()
	h.keys(":played<CR>j") // on Dusk Signal, under Low Tide
	if row := h.cursorRow(); !strings.Contains(row, "Dusk Signal") {
		t.Fatalf("cursor on %q", row)
	}
	h.keys(">") // a third track starts above it
	h.pump()
	if row := h.cursorRow(); !strings.Contains(row, "Dusk Signal") {
		t.Errorf("cursor moved to %q, want it on Dusk Signal", row)
	}
}

func TestPlayedOnHome(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.homeRow("Played this session")
	h.keys("<CR>")
	h.contains(
		"Home › Played this session (0)",
		"nothing played yet this session · <CR> on a track plays it",
	)
	h.keys(":sort title<CR>")
	h.contains("Played this session (0) (title ↑)")
}
