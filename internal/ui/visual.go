package ui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// visualSel is a Visual mode selection: whole rows from anchor to the
// cursor, in one list. Leaving that list ends it.
type visualSel struct {
	win    window
	depth  int // len(m.lib) when it started, for the library window
	kind   listKind
	anchor int
}

func (m *Model) startVisual() {
	if m.visual != nil {
		m.endVisual() // V again ends it, as in vim
		return
	}
	m.visual = &visualSel{win: m.focus, depth: len(m.lib), kind: m.top().kind, anchor: m.cursor()}
	m.mode = keymap.Visual
}

func (m *Model) endVisual() {
	m.visual = nil
	if m.mode == keymap.Visual {
		m.mode = keymap.Normal
	}
}

// checkVisual ends the selection once the cursor has left its list.
func (m *Model) checkVisual() {
	v := m.visual
	if v == nil {
		return
	}
	if m.focus != v.win ||
		v.win == libraryWindow && (len(m.lib) != v.depth || m.top().kind != v.kind) {
		m.endVisual()
	}
}

// cursor is the focused list's cursor row.
func (m *Model) cursor() int {
	if m.focus == queueWindow {
		return m.queueCur
	}
	return m.top().cur
}

func (m *Model) setCursor(i int) {
	if m.focus == queueWindow {
		m.queueCur = min(max(i, 0), max(len(m.state.Queue)-1, 0))
		return
	}
	f := m.top()
	f.cur = min(max(i, 0), max(m.rows(f)-1, 0))
	m.settle(f, 1)
}

// listLen is the number of rows in the focused list.
func (m *Model) listLen() int {
	if m.focus == queueWindow {
		return len(m.state.Queue)
	}
	return m.rows(m.top())
}

// selection is the selected rows, To exclusive.
func (m *Model) selection() (from, to int) {
	a, c := m.visual.anchor, m.cursor()
	return min(a, c), max(a, c) + 1
}

// inSelection reports whether row i of a window is in the selection.
func (m *Model) inSelection(w window, i int) bool {
	if m.visual == nil || m.visual.win != w || m.focus != w {
		return false
	}
	from, to := m.selection()
	return i >= from && i < to
}

// opRange is the rows an operator acts on: the selection, the rows a
// motion covers, or the cursor row and count-1 more.
func (m *Model) opRange(c command.Call) (from, to int) {
	if m.visual != nil {
		return m.selection()
	}
	cur := m.cursor()
	if c.Motion == "" {
		return cur, min(cur+c.N(), m.listLen())
	}
	// Run the motion, see where it lands, and put the cursor back.
	f := m.top()
	saved, savedOff, savedQueue, savedQueueOff := f.cur, f.offset, m.queueCur, m.queueOff
	_ = m.registry.Run(m, c.Motion, c.Count)
	target := m.cursor()
	f.cur, f.offset, m.queueCur, m.queueOff = saved, savedOff, savedQueue, savedQueueOff
	return min(cur, target), max(cur, target) + 1
}

// rowTracks is what row i of a library list stands for: a track, an
// album's tracks, or an artist (which can't be added as tracks).
func (m *Model) rowTracks(f *frame, i int) (tracks []library.Track, artist bool) {
	lib := m.state.Library
	if lib == nil || i < 0 || i >= m.rows(f) {
		return nil, false
	}
	switch f.kind {
	case artistList:
		return nil, true
	case albumList:
		return lib.AlbumTracks(m.albumsOf(f)[i].ID), false
	case homeAlbums:
		return lib.AlbumTracks(m.albumsOf(f)[i].ID), false
	case artistPage:
		if a, ok := m.artistRows(f.artist).album(i); ok {
			return lib.AlbumTracks(a.ID), false
		}
	case albumTracks:
		return m.tracksOf(f)[i : i+1], false
	case playlistList:
		return lib.PlaylistTracks(lib.Playlists[i].ID), false
	case playlistTracks:
		return lib.PlaylistTracks(f.playlist)[i : i+1], false
	case searchList:
		v := f.search
		if _, ok := v.artist(i); ok {
			return nil, true
		}
		if a, ok := v.album(i); ok {
			return lib.AlbumTracks(a.ID), false
		}
		if t, ok := v.track(i); ok {
			return []library.Track{t}, false
		}
	}
	return nil, false
}

// rangeTracks is the tracks of rows from..to of the focused list, and
// how many artist rows were left out.
func (m *Model) rangeTracks(from, to int) (tracks []library.Track, artists int) {
	if m.focus == queueWindow {
		for _, q := range m.state.Queue[from:min(to, len(m.state.Queue))] {
			tracks = append(tracks, q.Track)
		}
		return tracks, 0
	}
	for i := from; i < to; i++ {
		ts, artist := m.rowTracks(m.top(), i)
		tracks = append(tracks, ts...)
		if artist {
			artists++
		}
	}
	return tracks, artists
}

var errArtist = errors.New("an artist can't be added · open it and add their albums")

// skippedArtists adds a note about artist rows left out.
func (m *Model) skippedArtists(n int) {
	switch {
	case n == 1:
		m.appendMsg(" · 1 artist skipped")
	case n > 1:
		m.appendMsg(fmt.Sprintf(" · %d artists skipped", n))
	}
}

// yank is y: the rows' tracks go into the register.
func (m *Model) yank(c command.Call) error {
	from, to := m.opRange(c)
	tracks, artists := m.rangeTracks(from, to)
	m.endVisual()
	m.setCursor(from)
	if len(tracks) == 0 {
		if artists > 0 {
			return errArtist
		}
		return nil
	}
	m.event(app.Yank{Tracks: tracks})
	m.skippedArtists(artists)
	return nil
}

var errQueueOnly = errors.New("only the queue and playlists can be edited · y yanks")

// onPlaylist reports whether the cursor is in a playlist's tracks.
func (m *Model) onPlaylist() bool {
	return m.focus == libraryWindow && m.top().kind == playlistTracks
}

// remove is d and x: queue rows go into the register, and so do playlist
// rows, once the removal is confirmed.
func (m *Model) remove(c command.Call) error {
	if m.onPlaylist() {
		return m.removeFromPlaylist(c)
	}
	if m.focus != queueWindow {
		m.endVisual()
		return errQueueOnly
	}
	from, to := m.opRange(c)
	m.endVisual()
	m.event(app.Remove{From: from, To: to})
	m.setCursor(from)
	return nil
}

// paste is p (after) and P (before the cursor row) in the queue or a
// playlist.
func (m *Model) paste(c command.Call, after bool) error {
	if m.onPlaylist() {
		return m.pasteInPlaylist(c, after)
	}
	at := m.queueCur
	if after && len(m.state.Queue) > 0 {
		at++
	}
	before := len(m.state.Queue)
	m.event(app.Paste{At: at, Times: c.N()})
	if len(m.state.Queue) > before && m.focus == queueWindow {
		m.queueCur = at
	}
	return nil
}

// moveRows is J and K: the cursor row or the selection moves down or up.
func (m *Model) moveRows(c command.Call, down bool) error {
	if m.onPlaylist() {
		return m.moveInPlaylist(c, down)
	}
	if m.focus != queueWindow {
		return errQueueOnly
	}
	from, to := m.queueCur, min(m.queueCur+1, len(m.state.Queue))
	if m.visual != nil {
		from, to = m.selection()
	}
	by := c.N()
	if !down {
		by = -by
	}
	by = min(max(by, -from), len(m.state.Queue)-to)
	if by == 0 {
		return nil
	}
	m.event(app.Move{From: from, To: to, By: by})
	m.queueCur += by
	if m.visual != nil {
		m.visual.anchor += by
	}
	return nil
}

// visualHints is the hint line in Visual mode, with counts.
func (m *Model) visualHints() string {
	from, to := m.selection()
	type item struct{ name, label string }
	var items []item
	if m.focus == queueWindow {
		n := count(to - from)
		items = []item{
			{"delete", "remove " + n},
			{"yank", "yank " + n},
			{"move-down", "move down"},
			{"move-up", "move up"},
			{"cancel", "cancel"},
		}
	} else {
		tracks, _ := m.rangeTracks(from, to)
		n := count(len(tracks))
		items = []item{
			{"add-end", "add " + n + " to queue"},
			{"add-next", "play " + n + " next"},
			{"yank", "yank " + n},
			{"cancel", "cancel"},
		}
	}
	var parts []string
	for _, it := range items {
		if ks := m.engine.Table().Keys(keymap.Visual, m.scopes(), it.name); len(ks) > 0 {
			parts = append(parts, hint(ks[0], it.label))
		}
	}
	return " " + strings.Join(parts, "  ")
}

func count(n int) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}
