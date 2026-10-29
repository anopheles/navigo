package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

// order is the screen's lines that contain each of names, in screen order.
func (h *harness) order(names ...string) []string {
	h.t.Helper()
	var got []string
	for _, line := range strings.Split(h.screen(), "\n") {
		for _, n := range names {
			if strings.Contains(line, n) {
				got = append(got, n)
			}
		}
	}
	return got
}

func (h *harness) cursorOn(want string) {
	h.t.Helper()
	if got := h.cursorRow(); !strings.Contains(got, want) {
		h.t.Errorf("cursor on %q, want %q", strings.TrimSpace(got), want)
	}
}

func TestArtistsList(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a")
	h.contains(" Artists ", "Aurora Lane")
	// By sort name: The Quiet Ones under Q; Various Artists is listed.
	want := []string{
		"Aurora Lane",
		"Long Haul Orchestra",
		"Pell Mell",
		"The Quiet Ones",
		"Various Artists",
		"Zoë Brandt",
		"森の音",
	}
	if got := orderIn(h.column(1), want...); strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("artists in order %q, want %q", got, want)
	}
	golden.RequireEqual(t, h.screen())

	// :artists from elsewhere, and h back out to the sections.
	h.keys("'b:artists<CR>")
	h.contains(" Artists ")
	h.keys("h")
	h.contains(" Library ")
	h.cursorOn("Artists")
}

// Aurora Lane has albums of their own and appears on the compilation.
func TestArtistPageBothGroups(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a<CR>")
	h.onPath("Artists › Aurora Lane")
	h.contains("── Appears on ──", "Night Drive Vol. 1")
	if got := h.order(
		"Blue Hour",
		"Appears on",
		"Night Drive",
	); strings.Join(
		got,
		",",
	) != "Blue Hour,Appears on,Night Drive" {
		t.Errorf("page order %q", got)
	}
	h.cursorOn("Blue")
	golden.RequireEqual(t, h.screen())

	h.keys("jj") // over the heading
	h.cursorOn("Night Drive Vol. 1")
	h.keys("k")
	h.cursorOn("Blue Hour")
	h.keys("{")
	h.cursorOn("Blue")
	h.keys("}")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("{")
	h.cursorOn("Blue")
	h.keys("G")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("<CR>")
	h.onPath("Artists › Aurora Lane › Night Drive Vol. 1")
	h.cursorOn("Neon Coast")
}

// Pell Mell only appears on the compilation: the page starts with the
// heading, and the album opens on their first track.
func TestArtistPageAppearsOnOnly(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a2j")
	h.cursorOn("Pell Mell")
	h.keys("<CR>")
	h.contains("── Appears on ──")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("gg")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("<CR>")
	h.cursorOn("Night Drive")
	if !strings.Contains(h.cursorRow(), "3 ") {
		t.Errorf("cursor on %q, want track 3", h.cursorRow())
	}
}

func TestArtistPageVariousArtists(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a4j")
	h.cursorOn("Various Artists")
	h.keys("<CR>")
	h.onPath("Artists › Various Artists")
	h.contains("Night Drive Vol. 1")
	h.lacks("Appears on")
	h.keys("<CR>")
	h.cursorOn("Neon Coast")
}

func TestGoToArtistAndAlbum(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a2j<CR><CR>j") // Pell Mell › Night Drive Vol. 1 › Blue Highway
	h.cursorOn("Blue Highway")

	// Two artists: gr asks which.
	h.keys("gr")
	h.contains("Go to which artist?", "<CR> choose")
	h.keys("j<CR>")
	h.onPath("Artists › Zoë Brandt")
	h.contains("── Appears on ──")
	h.cursorOn("Night Drive Vol. 1")

	// gr on an album row goes to its album artist; ga to the album.
	h.keys("gr")
	h.onPath("Artists › Various Artists")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("ga")
	h.onPath("Albums › Night Drive Vol. 1")
	h.cursorOn("Neon Coast")

	// From the queue, ga lands on the track; gr on a one-artist track
	// goes straight there.
	h.keys("jjjA'q")
	h.keys("ga")
	h.onPath("Albums › Night Drive Vol. 1")
	h.cursorOn("Blue Highway")
	h.keys("kgr")
	h.lacks("Go to which artist?")
	h.onPath("Artists › Pell Mell")
	h.cursorOn("Night Drive Vol. 1")

	// h from there is the Artists list, cursor on the artist.
	h.keys("h")
	h.cursorOn("Pell Mell")

	// <Esc> closes the chooser without going anywhere.
	h.keys("'b")
	h.onPath("Albums › Night Drive Vol. 1")
	h.keys("jgr<Esc>")
	h.lacks("Go to which artist?")
	h.onPath("Albums › Night Drive Vol. 1")
}

// Each section keeps its path and cursor while another is shown.
func TestSectionsRemember(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a<CR>j'b3j'a")
	h.onPath("Artists › Aurora Lane")
	h.cursorOn("Blue Hour")
	h.keys("'b")
	h.cursorOn("Blue Hour") // the album list's own cursor
	h.keys("'a")
	h.keys("'a") // again, in Artists: the section's list
	h.contains(" Artists ")
	h.lacks("Artists ›")
}

// An artist page sorts and filters each group on its own, under its
// heading.
func TestSortAndFilterArtistPage(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'a<CR>") // Aurora Lane: Blue, Blue Hour; appears on Night Drive
	h.keys(":sort year desc<CR>")
	h.onPath("Artists › Aurora Lane (year ↓)")
	h.contains("sorted by year")
	if got := strings.Join(
		orderIn(h.column(2), "Blue Hour", "Blue  ", "Appears on", "Night Drive"),
		",",
	); got != "Blue Hour,Blue  ,Appears on,Night Drive" {
		t.Errorf("page order %q", got)
	}
	h.cursorOn("Blue  ") // the cursor stays on its album
	h.keys(":sort<CR>")
	h.contains("back in the usual order")

	// Appears on filtered empty: no heading.
	h.keys(":filter genre dream<CR>")
	h.onPath("Artists › Aurora Lane (genre dream)")
	h.contains("2 shown", "Blue Hour")
	h.lacks("Appears on", "Night Drive")

	// Their own albums filtered empty: the heading comes first.
	h.keys(":filter genre synth<CR>")
	h.contains("1 shown", "── Appears on ──", "Night Drive Vol. 1")
	h.lacks("Blue Hour")
	h.cursorOn("Night Drive Vol. 1")
	h.keys("<CR>")
	h.onPath("Artists › Aurora Lane (genre synth) › Night Drive Vol. 1")
	h.keys("h")
	h.onPath("Artists › Aurora Lane (genre synth)")

	// Nothing left.
	h.keys(":filter year 1990<CR>")
	h.contains("0 shown")
	h.lacks("Appears on", "Blue Hour", "Night Drive")
	h.keys(":filter<CR>")
	h.contains("filter cleared", "Blue Hour", "── Appears on ──", "Night Drive Vol. 1")
}
