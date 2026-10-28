package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"
)

// row is the screen's line holding text, library window only.
func (h *harness) libraryRowWith(text string) string {
	h.t.Helper()
	for _, l := range strings.Split(h.screen(), "\n") {
		lib, _, _ := strings.Cut(l, "││")
		if strings.Contains(lib, text) {
			return lib
		}
	}
	h.t.Fatalf("no library line with %q:\n%s", text, h.screen())
	return ""
}

// marksHarness opens Blue Hour with its second track queued, its third
// starred and its fourth rated.
func marksHarness(t *testing.T) *harness {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>")
	h.keys("jA")           // Low Tide to the queue
	h.keys("j*")           // Streetlamp starred
	h.keys("j:rate 4<CR>") // Blue Hour rated
	h.keys("gg")
	return h
}

func TestLibraryMarksQueuedStarredAndRated(t *testing.T) {
	h := marksHarness(t)
	if r := h.libraryRowWith("Low Tide"); !strings.Contains(r, "•") {
		t.Errorf("a queued track has no mark: %q", r)
	}
	for _, other := range []string{"Dusk Signal", "Streetlamp", "Wintering"} {
		if r := h.libraryRowWith(other); strings.Contains(r, "•") {
			t.Errorf("%s is not queued but is marked: %q", other, r)
		}
	}
	if r := h.libraryRowWith("Streetlamp"); !strings.Contains(r, "★ Streetlamp") {
		t.Errorf("a starred track lost its ★: %q", r)
	}
	if r := h.libraryRowWith("4 Blue Hour"); !strings.Contains(r, "★4") {
		t.Errorf("a rated track doesn't show its rating: %q", r)
	}
	if r := h.libraryRowWith("Dusk Signal"); strings.Contains(r, "★") {
		t.Errorf("an unrated track shows a rating: %q", r)
	}
	golden.RequireEqual(t, h.screen())
}

// The queue is the queue: no marks, no ratings.
func TestQueueWindowHasNoLibraryMarks(t *testing.T) {
	h := marksHarness(t)
	h.keys("jjA") // the rated track too
	for _, l := range strings.Split(h.screen(), "\n") {
		_, queue, ok := strings.Cut(l, "││")
		if ok && (strings.Contains(queue, "•") || strings.Contains(queue, "★4")) {
			t.Errorf("the queue window carries a library mark: %q", queue)
		}
	}
}

func TestAlbumsListMarksQueuedAlbums(t *testing.T) {
	h := marksHarness(t)
	h.keys("h") // Albums: Blue Hour is the fourth row
	if r := h.libraryRowWith("Blue Hour"); !strings.Contains(r, "•") {
		t.Errorf("an album with a queued track has no mark: %q", r)
	}
	if r := h.libraryRowWith("The Long Haul"); strings.Contains(r, "•") {
		t.Errorf("an album with nothing queued is marked: %q", r)
	}
	golden.RequireEqual(t, h.screen())
}

// The marks are glyphs, so they stay with colours off.
func TestLibraryMarksSurviveNoColor(t *testing.T) {
	h := marksHarness(t)
	var b strings.Builder
	w := colorprofile.NewWriter(&b, nil)
	w.Profile = colorprofile.ASCII
	if _, err := w.WriteString(h.m.render()); err != nil {
		t.Fatal(err)
	}
	plain := ansi.Strip(b.String())
	if !strings.Contains(plain, "•") || !strings.Contains(plain, "★4") {
		t.Errorf("a mark needs colour:\n%s", plain)
	}
}

// Under 40 cells a row keeps its title and marks and drops the rest.
func TestNarrowRowsKeepTheQueueMark(t *testing.T) {
	h := marksHarness(t)
	h.send(tea.WindowSizeMsg{Width: 36, Height: 20})
	if r := h.libraryRowWith("Low Tide"); !strings.Contains(r, "•") {
		t.Errorf("a narrow row lost its queue mark: %q", r)
	}
	if r := h.libraryRowWith("Blue Hour"); strings.Contains(r, "★4") {
		t.Errorf("a narrow row shows a rating: %q", r)
	}
}
