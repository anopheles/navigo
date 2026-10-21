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

var errOffline = errors.New("offline · playlists can be changed once the server is back")

// shownPlaylist is the playlist whose tracks the library window shows.
func (m *Model) shownPlaylist() (library.Playlist, bool) {
	if m.state.Library == nil {
		return library.Playlist{}, false
	}
	return m.state.Library.Playlist(m.top().playlist)
}

func (m *Model) removeFromPlaylist(c command.Call) error {
	from, to := m.opRange(c)
	m.endVisual()
	p, ok := m.shownPlaylist()
	switch {
	case m.state.Offline:
		return errOffline
	case !ok || from >= to:
		return nil
	}
	m.confirm(fmt.Sprintf("Remove %s from %s?", count(to-from), p.Name), func() {
		m.event(app.PlaylistRemove{ID: p.ID, From: from, To: to})
		m.setCursor(from)
	})
	return nil
}

func (m *Model) pasteInPlaylist(c command.Call, after bool) error {
	p, ok := m.shownPlaylist()
	if !ok {
		return nil
	}
	reg := m.state.Register
	if len(reg) == 0 {
		return errors.New("nothing to paste: yank or delete something first")
	}
	var tracks []library.Track
	for range c.N() {
		tracks = append(tracks, reg...)
	}
	at := m.top().cur
	if after && len(p.Tracks) > 0 {
		at++
	}
	m.event(app.PlaylistInsert{ID: p.ID, At: at, Tracks: tracks})
	m.setCursor(at)
	return nil
}

func (m *Model) moveInPlaylist(c command.Call, down bool) error {
	p, ok := m.shownPlaylist()
	if !ok {
		return nil
	}
	f := m.top()
	from, to := f.cur, min(f.cur+1, len(p.Tracks))
	if m.visual != nil {
		from, to = m.selection()
	}
	by := c.N()
	if !down {
		by = -by
	}
	by = min(max(by, -from), len(p.Tracks)-to)
	if by == 0 {
		return nil
	}
	m.event(app.PlaylistMove{ID: p.ID, From: from, To: to, By: by})
	f.cur += by
	if m.visual != nil {
		m.visual.anchor += by
	}
	return nil
}

// deletePlaylist is dd in the playlist list, after y/n.
func (m *Model) deletePlaylist(command.Call) error {
	f := m.top()
	lib := m.state.Library
	if m.focus != libraryWindow || f.kind != playlistList || lib == nil ||
		f.cur >= len(lib.Playlists) {
		return errors.New("not on a playlist")
	}
	if m.state.Offline {
		return errOffline
	}
	p := lib.Playlists[f.cur]
	m.confirm(
		fmt.Sprintf("Delete the playlist %s?", p.Name),
		func() { m.event(app.PlaylistDelete{ID: p.ID}) },
	)
	return nil
}

// playlistRename is :playlist-rename <name>, on the playlist under the
// cursor in the list or the one open.
func (m *Model) playlistRename(c command.Call) error {
	name := strings.Join(c.Args, " ")
	if name == "" {
		return errors.New("playlist-rename: give the new name, such as playlist-rename Rock")
	}
	lib := m.state.Library
	f := m.top()
	var p library.Playlist
	switch {
	case lib == nil || m.focus != libraryWindow:
		return errors.New("not on a playlist")
	case f.kind == playlistList && f.cur < len(lib.Playlists):
		p = lib.Playlists[f.cur]
	case f.kind == playlistTracks:
		var ok bool
		if p, ok = m.shownPlaylist(); !ok {
			return errors.New("not on a playlist")
		}
	default:
		return errors.New("not on a playlist")
	}
	for _, o := range lib.Playlists {
		if o.ID != p.ID && strings.EqualFold(o.Name, name) {
			return fmt.Errorf("there's a playlist called %q already", o.Name)
		}
	}
	if m.state.Offline {
		return errOffline
	}
	m.event(app.PlaylistRename{ID: p.ID, Name: name})
	return nil
}

// playlistUndo is u and <C-r> in a playlist: changes on the server can't
// be undone, but a removal can be put back.
func (m *Model) playlistUndo(command.Call) error {
	key := ":paste-before"
	if ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), "paste-before"); len(ks) > 0 {
		key = ks[0]
	}
	m.setMsg("playlist changes can't be undone · "+key+" puts the last removal back", false)
	return nil
}

// chosenTracks is what a command acts on: the selection's tracks, or the
// row under the cursor. It ends Visual mode; artists counts artist rows
// left out.
func (m *Model) chosenTracks() (tracks []library.Track, artists int, err error) {
	switch {
	case m.visual != nil:
		tracks, artists = m.rangeTracks(m.selection())
		m.endVisual()
	case m.focus == queueWindow:
		if m.queueCur < len(m.state.Queue) {
			tracks = []library.Track{m.state.Queue[m.queueCur].Track}
		}
	default:
		f := m.top()
		var artist bool
		tracks, artist = m.rowTracks(f, f.cur)
		if artist {
			artists = 1
		}
	}
	switch {
	case len(tracks) > 0:
		return tracks, artists, nil
	case artists > 0:
		return nil, artists, errArtist
	}
	return nil, 0, errNotOnTrack
}

// playlistAdd is :playlist-add <name>: the selection or row goes to the
// end of a playlist.
func (m *Model) playlistAdd(c command.Call) error {
	name := strings.Join(c.Args, " ")
	if name == "" {
		return errors.New(
			"playlist-add: give the playlist's name, such as playlist-add Morning Mix",
		)
	}
	var p library.Playlist
	found := false
	if lib := m.state.Library; lib != nil {
		for _, o := range lib.Playlists {
			if strings.EqualFold(o.Name, name) {
				p, found = o, true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("no playlist called %q · :playlist-new %s makes one", name, name)
	}
	tracks, _, err := m.chosenTracks()
	if err != nil {
		return err
	}
	if m.state.Offline {
		return errOffline
	}
	m.event(app.PlaylistInsert{ID: p.ID, At: -1, Tracks: tracks})
	return nil
}

// playlistNew is :playlist-new <name>: a playlist of the selection or
// row, or an empty one.
func (m *Model) playlistNew(c command.Call) error {
	name := strings.Join(c.Args, " ")
	if name == "" {
		return errors.New("playlist-new: give it a name, such as playlist-new Rock")
	}
	tracks, _, err := m.chosenTracks()
	if err != nil && !errors.Is(err, errNotOnTrack) && !errors.Is(err, errArtist) {
		return err
	}
	if m.state.Offline {
		return errOffline
	}
	m.event(app.PlaylistCreate{Name: name, Tracks: tracks})
	return nil
}

// playlistNames are the names :playlist-add completes.
func (m *Model) playlistNames() []string {
	var out []string
	if lib := m.state.Library; lib != nil {
		for _, p := range lib.Playlists {
			out = append(out, p.Name)
		}
	}
	return out
}
