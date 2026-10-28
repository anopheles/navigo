package ui

import (
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/library"
)

// openStarredTracks opens Home's Starred tracks list.
func (h *harness) openStarredTracks() {
	h.t.Helper()
	h.homeRow("Starred tracks")
	h.keys("<CR>")
}

// trackID is the ID of the fixture library's track titled title.
func (h *harness) trackID(title string) library.TrackID {
	h.t.Helper()
	for _, t := range h.m.state.Library.Tracks {
		if t.Title == title {
			return t.ID
		}
	}
	h.t.Fatalf("no track %q in the fixture library", title)
	return ""
}

func TestStarredTracksOnHome(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.openStarredTracks()
	// The fixture starred Night Drive after Blue Again: newest first.
	h.contains("Home › Starred tracks (2)")
	want := []string{"Night Drive", "Blue Again"}
	if got := h.order(want...); !slices.Equal(got, want) {
		t.Errorf("order %q, want the most recently starred first", got)
	}
	golden.RequireEqual(t, h.screen())

	// <CR> plays the list from that row, as any track list does.
	h.keys("<CR>")
	if q := h.m.state.Queue; len(q) != 2 || q[0].Track.Title != "Night Drive" {
		t.Errorf("queue = %v, want the list", q)
	}
}

func TestStarredTracksSortAndFilter(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.openStarredTracks()
	h.command("sort starred")
	h.contains("Starred tracks (2) (starred ↑)")
	want := []string{"Blue Again", "Night Drive"}
	if got := h.order(want...); !slices.Equal(got, want) {
		t.Errorf(":sort starred gives %q, want the oldest first", got)
	}
	h.command("sort")
	want = []string{"Night Drive", "Blue Again"}
	if got := h.order(want...); !slices.Equal(got, want) {
		t.Errorf(":sort alone gives %q, want the newest first again", got)
	}
	h.command("sort nope")
	h.contains("try starred, title, artist")
	h.command("filter genre no-such-genre")
	h.contains("no row matches the filter")
}

func TestStarredTracksKeepAnUnstarredRow(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.openStarredTracks()
	h.keys("*") // unstars Night Drive, under the cursor
	h.contains("unstarred Night Drive")
	if h.m.state.Library.IsStarred(string(h.trackID("Night Drive"))) {
		t.Fatal("Night Drive is still starred")
	}
	if row := h.cursorRow(); !strings.Contains(row, "Night Drive") || strings.Contains(row, "★") {
		t.Errorf("cursor row %q, want Night Drive still there without its ★", row)
	}
	if n := h.m.rows(h.m.top()); n != 2 {
		t.Errorf("%d rows after unstarring, want 2 until the list opens again", n)
	}

	// A track starred elsewhere comes first the next time the list opens,
	// and the unstarred one is gone.
	h.m.state.Library = h.m.state.Library.WithExtras(
		h.m.state.Library.Playlists,
		append(
			[]library.Star{
				{
					ID: string(h.trackID("Undertow")),
					At: h.m.state.Library.StarredAt(string(h.trackID("Blue Again"))).Add(1),
				},
			},
			h.m.state.Library.Starred...,
		),
	)
	h.openStarredTracks()
	h.contains("Starred tracks (2)")
	want := []string{"Undertow", "Blue Again"}
	if got := h.order(want...); !slices.Equal(got, want) {
		t.Errorf("reopened list %q, want Undertow then Blue Again", got)
	}
}

func TestStarredTracksSkipMissingTracks(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	lib := h.m.state.Library
	h.send(eventMsg{appSynced(library.Build(
		lib.Server, lib.User, lib.LastScan, lib.Artists, lib.Albums, lib.Tracks, lib.Playlists,
		append([]library.Star{{ID: "no-such-track"}}, lib.Starred...),
	))})
	h.openStarredTracks()
	h.contains("Starred tracks (2)")
}

func TestStarredTracksOffline(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.openStarredTracks()
	h.contains("Starred tracks (2)", "Night Drive", "Blue Again")
}

func TestEmptyStarredTracksNamesTheKey(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.emptied()
	h.openStarredTracks()
	h.contains("Starred tracks (0)", "nothing starred yet · * stars the track under the cursor")
}
