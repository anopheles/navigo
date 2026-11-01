package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/keymap"
)

// pickRows are the playlist popup's rows, without styling.
func (h *harness) pickRows() []string {
	h.t.Helper()
	if h.m.popup == nil || h.m.pick == nil {
		h.t.Fatal("the playlist popup isn't open")
	}
	var out []string
	for _, l := range h.m.popup.lines {
		out = append(out, strings.Join(strings.Fields(ansi.Strip(l)), " "))
	}
	return out
}

// pickCursor is the row under the popup's cursor.
func (h *harness) pickCursor() string {
	h.t.Helper()
	return h.pickRows()[h.m.popup.sel]
}

func TestPlaylistPickAddsAndRemembers(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'b<CR>") // the first album's tracks, on the first
	title := h.m.tracksOf(h.m.top())[0].Title
	h.keys("gl")
	h.contains("add "+title+" to…", "new playlist…", "<CR> add", "/ filter")
	if got := strings.Join(
		h.pickRows(),
		" | ",
	); got != "new playlist… | Empty 0 tracks | Morning Mix 5 tracks" {
		t.Errorf("rows %q", got)
	}
	if c := h.pickCursor(); c != "Empty 0 tracks" {
		t.Errorf("cursor on %q, want the first playlist", c)
	}
	golden.RequireEqual(t, h.screen())

	h.keys("j<CR>")
	h.contains("1 track added to Morning Mix (6 in it)")
	if h.m.popup != nil {
		t.Error("the popup is still open")
	}

	// The playlist used last comes first, under the cursor.
	h.keys("jgl")
	if c := h.pickCursor(); c != "Morning Mix 6 tracks" {
		t.Errorf("cursor on %q, want the last used", c)
	}
	h.keys("<CR>")
	h.contains("1 track added to Morning Mix (7 in it)")

	// :playlist-add alone opens it too, and :playlist-add <name> counts as
	// using a playlist.
	h.keys(":playlist-add Empty<CR>")
	h.keys(":playlist-add<CR>")
	if c := h.pickCursor(); !strings.HasPrefix(c, "Empty") {
		t.Errorf("cursor on %q, want Empty, used last", c)
	}
	h.keys("q")
	if h.m.popup != nil {
		t.Error("q didn't close it")
	}
}

func TestPlaylistPickFilterAndNew(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'b<CR>gl/")
	h.typeText("mor")
	h.contains("filter playlists: mor", "<CR> keep the filter")
	if got := strings.Join(
		h.pickRows(),
		" | ",
	); got != `new playlist "mor" | Morning Mix 5 tracks` {
		t.Errorf("filtered rows %q", got)
	}
	golden.RequireEqual(t, h.screen())
	// <Esc> puts the filter back as it was.
	h.keys("<Esc>")
	if n := len(h.pickRows()); n != 3 {
		t.Errorf("%d rows after <Esc>, want all 3", n)
	}

	// A filter that matches nothing leaves the new row, which takes the
	// filter as the name.
	h.keys("/")
	h.typeText("Late Night")
	h.keys("<CR>")
	if got := strings.Join(h.pickRows(), " | "); got != `new playlist "Late Night"` {
		t.Errorf("rows %q", got)
	}
	h.keys("<CR>")
	h.contains("new playlist: Late Night", "<CR> make it")
	h.keys("<CR>")
	h.contains("playlist Late Night made with 1 track")
	h.playlistIs("Late Night", h.m.tracksOf(h.m.top())[0].Title)

	// The new playlist is now the one used last.
	h.keys("gl")
	if c := h.pickCursor(); c != "Late Night 1 track" {
		t.Errorf("cursor on %q, want the new playlist", c)
	}

	// A name another playlist has is refused; <Esc> changes nothing.
	h.keys("k<CR>")
	h.typeText("empty")
	h.keys("<CR>")
	h.contains(`there's a playlist called "Empty" already`)
	h.keys("glk<CR><Esc>")
	h.contains("nothing changed")
	if n := len(h.m.state.Library.Playlists); n != 3 {
		t.Errorf("%d playlists, want 3", n)
	}
}

func TestPlaylistPickKeepsTheSelectionOnCancel(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'b<CR>Vjgl")
	h.contains("add 2 tracks to…")
	h.keys("<Esc>")
	if h.m.popup != nil || h.m.visual == nil {
		t.Fatalf(
			"popup %v, visual %v: want the popup closed and the selection kept",
			h.m.popup,
			h.m.visual,
		)
	}
	// Through the filter and the name prompt, a cancel keeps it too.
	h.keys("gl/")
	h.typeText("x")
	h.keys("<Esc>q")
	h.keys("glk<CR><Esc>")
	if h.m.visual == nil || h.m.mode != keymap.Visual {
		t.Fatalf(
			"visual %v, mode %v: want the selection kept, in Visual mode",
			h.m.visual,
			h.m.mode,
		)
	}
	h.keys("gl<CR>")
	h.contains("2 tracks added to Empty (2 in it)")
	if h.m.visual != nil {
		t.Error("Visual mode is still on after adding")
	}
}

func TestPlaylistPickRefusesBeforeOpening(t *testing.T) {
	for _, c := range []struct {
		name, keys, want string
		setup            func(*harness)
	}{
		{"artist", "'agl", "an artist can't be added", nil},
		{"empty list", "'p<CR>gl", "not on a track", nil},
		{"offline", "'b<CR>gl", "offline · playlists can be changed once the server is back", func(h *harness) {
			h.srv.SetDown(true)
			h.keys(":sync<CR>")
		}},
		{"unsynced", "gl", "can't change playlists yet · the library is still loading", func(h *harness) {
			h.m.state.Library = nil
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t, 120, 40, nil)
			if c.setup != nil {
				c.setup(h)
			}
			h.keys(c.keys)
			h.contains(c.want)
			if h.m.popup != nil {
				t.Error("the popup opened")
			}
		})
	}
}

func TestPlaylistPickInHelpAndKeyMenu(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("'b<CR>?jjjj")
	h.contains("add to playlist")
	h.keys("q'?/")
	h.typeText("add to playlist")
	h.contains("gl", "playlist-add")
}
