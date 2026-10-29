package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"
)

func (h *harness) albumNames() []string {
	var out []string
	for _, a := range h.m.albumsOf(h.m.top()) {
		out = append(out, a.Name)
	}
	return out
}

func TestForgottenFavouritesOnHome(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.m.seedFn = func() uint64 { return 3 }
	h.homeRow("Forgotten favourites")
	h.keys("<CR>")
	// The fixture's two starred tracks have never played.
	got := h.listTitles()
	slices.Sort(got)
	if want := []string{"Blue Again", "Night Drive"}; !slices.Equal(got, want) {
		t.Fatalf("rows %q, want %q", got, want)
	}
	h.contains("Home › Forgotten favourites (2)")
	golden.RequireEqual(t, h.screen())

	// Chosen once: a redraw or a new snapshot keeps the rows.
	before := h.listTitles()
	h.m.seedFn = func() uint64 { return 99 }
	h.pump()
	if after := h.listTitles(); !slices.Equal(before, after) {
		t.Errorf("rows changed while open: %q → %q", before, after)
	}
}

func TestNeverPlayedOnHome(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.homeRow("Never played")
	h.keys("<CR>")
	names := h.albumNames()
	if len(names) == 0 {
		t.Fatal("no albums")
	}
	for _, n := range names {
		for _, a := range h.m.state.Library.Albums {
			if a.Name == n && a.PlayCount > 0 {
				t.Errorf("%s has plays", n)
			}
		}
	}
	h.contains("Home › Never played")
	if n := h.requests("getAlbumList2"); n != 0 {
		t.Errorf("%d album list requests for a list from the cache", n)
	}
	// An album opens as anywhere else.
	h.keys("<CR>")
	h.contains("Home › Never played › " + names[0])
}

func TestRediscoverWhenEmpty(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.emptied()
	h.homeRow("Forgotten favourites")
	h.keys("<CR>")
	h.contains("nothing forgotten · favourites unplayed for 6 months show up here")
	h.homeRow("Never played")
	h.keys("<CR>")
	h.contains("you've played something from every album")
	if strings.Contains(h.screen(), "loading") {
		t.Error("a cache list says loading")
	}
}
