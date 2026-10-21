package ui

import (
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/library"
)

// writes are the playlist requests the fake server got, as
// "endpoint params", with track and playlist IDs as the library names
// them: titles for tracks, names for playlists.
func (h *harness) writes() []string {
	h.t.Helper()
	lib := h.m.state.Library
	name := func(key, v string) string {
		switch key {
		case "songId", "songIdToAdd":
			if t, ok := lib.Track(library.TrackID(v)); ok {
				return t.Title
			}
		case "playlistId", "id":
			for _, p := range lib.Playlists {
				if string(p.ID) == v {
					return p.Name
				}
			}
			return "?" // deleted, or made in the recording
		}
		return v
	}
	var out []string
	for _, r := range h.srv.Requests() {
		switch r.Endpoint {
		case "createPlaylist", "updatePlaylist", "deletePlaylist":
		default:
			continue
		}
		var parts []string
		for _, key := range []string{"id", "playlistId", "name", "songId", "songIdToAdd", "songIndexToRemove"} {
			for _, v := range r.Params[key] {
				parts = append(parts, key+"="+name(key, v))
			}
		}
		out = append(out, r.Endpoint+" "+strings.Join(parts, " "))
	}
	return out
}

func (h *harness) wrote(want ...string) {
	h.t.Helper()
	if got := h.writes(); strings.Join(got, "\n") != strings.Join(want, "\n") {
		h.t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// playlistIs checks a playlist's tracks, as titles.
func (h *harness) playlistIs(name, want string) {
	h.t.Helper()
	for _, p := range h.m.state.Library.Playlists {
		if p.Name != name {
			continue
		}
		var titles []string
		for _, t := range h.m.state.Library.PlaylistTracks(p.ID) {
			titles = append(titles, t.Title)
		}
		if got := strings.Join(titles, ", "); got != want {
			h.t.Errorf("%s = %s\n   want %s", name, got, want)
		}
		return
	}
	h.t.Errorf("no playlist %s", name)
}

const mix = "Blue, Undertow, Café Nocturne, 朝霧, Blue"

func TestPlaylistsSection(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'p")
	h.contains(" Playlists ", "Empty", "0 tracks", "Morning Mix", "5 tracks")
	h.keys("j<CR>")
	h.contains("Playlists › Morning Mix", "Undertow", "朝霧")
	h.keys("j<CR>")
	h.contains("playing · 5 tracks in the queue", "▶ Undertow", "2/5 in queue")
}

func TestPlaylistRemoveAsks(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'pj<CR>jdd")
	h.contains("Remove 1 track from Morning Mix?", "y yes  n no")
	h.keys("n")
	h.contains("nothing changed")
	h.wrote()

	h.keys("ddy")
	h.wrote("updatePlaylist playlistId=Morning Mix songIndexToRemove=1")
	h.playlistIs("Morning Mix", "Blue, Café Nocturne, 朝霧, Blue")
	h.contains("1 track removed from Morning Mix · P puts them back")
	h.cursorOn("Café Nocturne")

	// No undo, but the removal is in the register.
	h.keys("u")
	h.contains("playlist changes can't be undone · P puts the last removal back")
	h.keys("P")
	h.playlistIs("Morning Mix", mix)
	h.wrote(
		"updatePlaylist playlistId=Morning Mix songIndexToRemove=1",
		"createPlaylist playlistId=Morning Mix songId=Blue songId=Undertow songId=Café Nocturne songId=朝霧 songId=Blue",
	)
	h.contains("1 track added to Morning Mix (5 in it)")

	// A selection and d{motion} ask too.
	h.keys("ggVjd")
	h.contains("Remove 2 tracks from Morning Mix?")
	h.keys("<Esc>")
	h.keys("ggd2jy")
	h.playlistIs("Morning Mix", "朝霧, Blue")
	if w := h.writes(); w[len(w)-1] != "updatePlaylist playlistId=Morning Mix songIndexToRemove=0 songIndexToRemove=1 songIndexToRemove=2" {
		t.Errorf("last request %q", w[len(w)-1])
	}
	if h.m.state.Queue != nil {
		t.Error("the queue changed")
	}
}

func TestPlaylistPasteAndMove(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'pj<CR>J")
	h.playlistIs("Morning Mix", "Undertow, Blue, Café Nocturne, 朝霧, Blue")
	h.contains("moved 1 track down in Morning Mix")
	h.cursorOn("Blue")
	h.keys("VjK<Esc>") // a selection moves together
	h.playlistIs("Morning Mix", "Blue, Café Nocturne, Undertow, 朝霧, Blue")
	h.cursorOn("Café Nocturne")
	h.keys("yyGp") // pasted after the last row: added at the end
	h.playlistIs("Morning Mix", "Blue, Café Nocturne, Undertow, 朝霧, Blue, Café Nocturne")
	h.wrote(
		"createPlaylist playlistId=Morning Mix songId=Undertow songId=Blue songId=Café Nocturne songId=朝霧 songId=Blue",
		"createPlaylist playlistId=Morning Mix songId=Blue songId=Café Nocturne songId=Undertow songId=朝霧 songId=Blue",
		"updatePlaylist playlistId=Morning Mix songIdToAdd=Café Nocturne",
	)
}

func TestPlaylistAddNewDelete(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	// :playlist-add the track under the cursor, names completing.
	h.keys("3j<CR>j:")
	h.typeText("playlist-add mo")
	h.keys("<Tab>")
	if v := h.m.line.Value(); v != "playlist-add Morning Mix" {
		t.Errorf("completed %q", v)
	}
	h.keys("<CR>")
	h.contains("1 track added to Morning Mix (6 in it)")
	h.keys(":playlist-add nope<CR>")
	h.contains(`no playlist called "nope" · :playlist-new nope makes one`)

	// :playlist-new from a selection.
	h.keys("ggVj:playlist-new Rock<CR>")
	h.contains("playlist Rock made with 2 tracks")
	h.playlistIs("Rock", "Dusk Signal, Low Tide")

	// dd in the list deletes one, after y/n.
	h.keys("'p")
	h.cursorOn("Empty")
	h.keys("G")
	h.cursorOn("Rock")
	h.keys("dd")
	h.contains("Delete the playlist Rock?")
	h.keys("y")
	h.contains("playlist Rock deleted")
	for _, p := range h.m.state.Library.Playlists {
		if p.Name == "Rock" {
			t.Error("Rock is still in the library")
		}
	}
	h.wrote(
		"updatePlaylist playlistId=Morning Mix songIdToAdd=Low Tide",
		"createPlaylist name=Rock songId=Dusk Signal songId=Low Tide",
		"deletePlaylist id=?",
	)
}

func TestPlaylistEditsOffline(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'pj<CR>dd")
	h.srv.SetDown(true)
	h.keys("y")
	h.contains("couldn't change Morning Mix", "offline")
	h.playlistIs("Morning Mix", mix)

	// Offline, edits are refused before asking.
	h.keys("<Esc>dd")
	h.lacks("Remove 1 track")
	h.contains("offline · playlists can be changed once the server is back")
}

func TestPlaylistRename(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	// In the list, the playlist under the cursor.
	h.keys("'pj:playlist-rename Evening Mix<CR>")
	h.contains("playlist Morning Mix renamed to Evening Mix", "Evening Mix")
	h.playlistIs("Evening Mix", mix)
	h.wrote("updatePlaylist playlistId=Evening Mix name=Evening Mix")

	// In a playlist, the one shown; the title follows.
	h.keys("<CR>:playlist-rename Dawn<CR>")
	h.contains("Playlists › Dawn", "playlist Evening Mix renamed to Dawn")

	h.keys(":playlist-rename empty<CR>")
	h.contains(`there's a playlist called "Empty" already`)
	h.keys(":playlist-rename<CR>")
	h.contains("playlist-rename: give the new name, such as playlist-rename Rock")
	h.keys("'a:playlist-rename X<CR>")
	h.contains("not on a playlist")

	h.srv.SetDown(true)
	h.keys("'p:playlist-rename X<CR>")
	h.contains("couldn't change Dawn")
	h.keys(":playlist-rename Y<CR>")
	h.contains("offline · playlists can be changed once the server is back")
}
