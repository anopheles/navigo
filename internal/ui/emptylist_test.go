package ui

import (
	"testing"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/library"
)

// emptied replaces the library with one that has no artists, albums,
// tracks or playlists.
func (h *harness) emptied() {
	h.t.Helper()
	old := h.m.state.Library
	h.send(
		eventMsg{
			appSynced(library.Build(old.Server, old.User, old.LastScan, nil, nil, nil, nil, nil)),
		},
	)
}

func TestEmptyListsSayWhatToDo(t *testing.T) {
	for _, tc := range []struct{ keys, want string }{
		{"'a", "no artists in the library · add music to the server, then :sync"},
		{"'b", "no albums in the library · add music to the server, then :sync"},
		{"'p", "no playlists yet · in a track list, :playlist-new <name> makes one"},
	} {
		h := newHarness(t, 120, 40, nil)
		h.emptied()
		h.keys(tc.keys)
		h.contains(tc.want)
	}
}

func TestEmptyPlaylistSaysHowToFillIt(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	old := h.m.state.Library
	p := old.Playlists[0]
	p.Tracks = nil
	h.send(eventMsg{appSynced(library.Build(
		old.Server, old.User, old.LastScan, old.Artists, old.Albums, old.Tracks,
		[]library.Playlist{p}, old.Starred,
	))})
	h.keys("'p<CR>")
	h.contains("empty playlist · :playlist-add <name> adds tracks")
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("empty playlist · tracks can be added once the server is back")
}

func TestEmptyPlaylistsOffline(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.emptied()
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.keys("'p")
	h.contains("no playlists in the cache · they can be made once the server is back")
}

func TestEmptyStarredNamesTheKey(t *testing.T) {
	h := homeHarness(t, 120, 40, nil)
	h.homeRow("Starred albums")
	h.keys("<CR>")
	h.send(eventMsg{app.AlbumListLoaded{Kind: "starred"}})
	h.contains("nothing starred yet · * stars an album")

	// Only Starred albums names the key.
	h.homeRow("Most played")
	h.keys("<CR>")
	h.send(eventMsg{app.AlbumListLoaded{Kind: "frequent"}})
	h.contains("nothing played yet · play an album and it shows up here")
	h.lacks("· * stars")
}

func TestEmptyFilterSaysHowToClearIt(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys(":filter genre no-such-genre<CR>")
	h.contains("no row matches the filter · :filter alone shows all")
}

func TestEmptySearchHints(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'szzqx<CR>")
	h.contains(`nothing in the library matches "zzqx"`, "try fewer letters")
	h.srv.SetDown(true)
	h.keys(":sync<CR>")
	h.contains("offline · this searches the cache")
}
