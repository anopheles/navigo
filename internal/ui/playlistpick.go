package ui

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// playlistPick is the playlist popup of :playlist-add alone (gl): what it
// adds, and the playlists it shows under its "new playlist…" row.
type playlistPick struct {
	tracks []library.Track
	what   string // what goes in, for the title: "Blue Hour", "12 tracks"
	visual bool   // opened in Visual mode: picking ends it, a cancel doesn't
	filter string
	before string // the filter when / started, for <Esc>
	rows   []library.Playlist
}

// openPlaylistPick refuses what :playlist-add would refuse, then opens
// the popup. The selection stays until a row is picked.
func (m *Model) openPlaylistPick() error {
	if m.state.Library == nil {
		m.notice(m.state.Unsynced("can't change playlists yet"))
		return nil
	}
	if m.state.Offline {
		return errOffline
	}
	tracks, _, err := m.peekTracks()
	if err != nil {
		return err
	}
	k := &playlistPick{tracks: tracks, what: m.addedName(tracks), visual: m.visual != nil}
	m.openChoice("add "+k.what+" to…", nil, m.pickRow)
	m.pick = k
	m.refreshPick()
	return nil
}

// addedName is what the popup's title says goes in: the selection's
// count, else the track, album or playlist under the cursor.
func (m *Model) addedName(tracks []library.Track) string {
	if m.visual == nil {
		if len(tracks) == 1 {
			return tracks[0].Title
		}
		if m.focus == libraryWindow {
			f := m.top()
			if it, ok := m.rowItem(f, f.cur); ok {
				return it.name
			}
			if p, ok := m.rowPlaylist(f, f.cur); ok {
				return p.Name
			}
		}
	}
	return count(len(tracks))
}

// rowPlaylist is the playlist row i of a list shows, if it shows one.
func (m *Model) rowPlaylist(f *frame, i int) (library.Playlist, bool) {
	lib := m.state.Library
	switch {
	case lib == nil:
	case f.kind == playlistList && i < len(lib.Playlists):
		return lib.Playlists[i], true
	case f.kind == searchList && f.search != nil:
		return f.search.playlist(i)
	}
	return library.Playlist{}, false
}

// refreshPick lists the playlists that match the filter, the last used
// first, and puts the cursor on the first of them.
func (m *Model) refreshPick() {
	k, p := m.pick, m.popup
	if k == nil || p == nil {
		return
	}
	k.rows = matchPlaylists(m.pickOrder(), k.filter)
	newRow := "new playlist…"
	if k.filter != "" {
		newRow = fmt.Sprintf("new playlist %q", k.filter)
	}
	w := max(min(m.w-4, 100)-4, 0)
	p.lines = []string{dimStyle.Render(newRow)}
	for _, pl := range k.rows {
		n := count(len(pl.Tracks))
		p.lines = append(p.lines, fit(pl.Name, max(w-len(n)-2, 0))+"  "+dimStyle.Render(n))
	}
	p.sel, p.off = min(1, len(p.lines)-1), 0
}

// pickOrder is the playlists, those added to this session first, the
// most recent first, then the rest as the Playlists section has them.
func (m *Model) pickOrder() []library.Playlist {
	lib := m.state.Library
	if lib == nil {
		return nil
	}
	out := make([]library.Playlist, 0, len(lib.Playlists))
	for _, id := range m.recentPlaylists {
		if p, ok := lib.Playlist(id); ok {
			out = append(out, p)
		}
	}
	for _, p := range lib.Playlists {
		if !slices.Contains(m.recentPlaylists, p.ID) {
			out = append(out, p)
		}
	}
	return out
}

// usedPlaylist puts a playlist first in the popup's order.
func (m *Model) usedPlaylist(id library.PlaylistID) {
	m.recentPlaylists = slices.Insert(
		slices.DeleteFunc(m.recentPlaylists, func(o library.PlaylistID) bool { return o == id }),
		0,
		id,
	)
}

// playlistWritten notes a playlist created with tracks as used, once the
// server has given it an ID.
func (m *Model) playlistWritten(w app.PlaylistWritten) {
	if w.Err == nil && w.Write.Kind == app.CreatePlaylist && w.NewID != "" &&
		len(w.Write.Songs) > 0 {
		m.usedPlaylist(w.NewID)
	}
}

// matchPlaylists keeps the playlists whose name matches filter, as the
// ":" line matches commands: a name that starts with it, then names with
// a word starting with each of its words, then names holding its letters
// in order. Ties keep their order.
func matchPlaylists(ps []library.Playlist, filter string) []library.Playlist {
	q := strings.ToLower(strings.TrimSpace(filter))
	if q == "" {
		return ps
	}
	type hit struct {
		p    library.Playlist
		tier int
	}
	var hits []hit
	for _, p := range ps {
		if t, ok := nameTier(q, strings.ToLower(p.Name)); ok {
			hits = append(hits, hit{p, t})
		}
	}
	slices.SortStableFunc(hits, func(a, b hit) int { return cmp.Compare(a.tier, b.tier) })
	out := make([]library.Playlist, len(hits))
	for i, h := range hits {
		out[i] = h.p
	}
	return out
}

func nameTier(q, name string) (int, bool) {
	if strings.HasPrefix(name, q) {
		return 0, true
	}
	words := strings.FieldsFunc(
		name,
		func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) },
	)
	all := true
	for _, w := range strings.Fields(q) {
		if !slices.ContainsFunc(words, func(x string) bool { return strings.HasPrefix(x, w) }) {
			all = false
			break
		}
	}
	if all {
		return 1, true
	}
	rest := []rune(strings.ReplaceAll(q, " ", ""))
	for _, r := range name {
		if len(rest) > 0 && r == rest[0] {
			rest = rest[1:]
		}
	}
	return 2, len(rest) == 0
}

// pickRow is <CR> in the popup: the top row asks for a new playlist's
// name, any other adds to that playlist.
func (m *Model) pickRow(i int) error {
	k := m.pick
	m.pick = nil
	if k == nil {
		return nil
	}
	if i == 0 {
		m.startNewPlaylistName(k)
		return nil
	}
	if i-1 >= len(k.rows) {
		return nil
	}
	if k.visual {
		m.endVisual()
	}
	return m.addToPlaylist(k.rows[i-1], k.tracks)
}

// addToPlaylist sends tracks to the end of a playlist.
func (m *Model) addToPlaylist(p library.Playlist, tracks []library.Track) error {
	if m.state.Offline {
		return errOffline
	}
	m.usedPlaylist(p.ID)
	m.event(app.PlaylistInsert{ID: p.ID, At: -1, Tracks: tracks})
	return nil
}

// startPickFilter is / in the playlist popup: a prompt that filters it as
// you type.
func (m *Model) startPickFilter() error {
	k := m.pick
	if k == nil || m.popup == nil {
		return errors.New("nothing to filter here")
	}
	k.before = k.filter
	m.promptPick = true
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "filter playlists: "
	m.fitLine()
	m.line.SetValue(k.filter)
	m.line.CursorEnd()
	m.then(m.line.Focus())
	return nil
}

// pickFilterKey handles the filter prompt's keys other than editing.
func (m *Model) pickFilterKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		before := ""
		if m.pick != nil {
			before = m.pick.before
		}
		m.leaveLine()
		m.filterPick(before)
		return nil, true
	case "enter":
		m.leaveLine()
		return nil, true
	}
	return nil, false
}

func (m *Model) filterPick(filter string) {
	if m.pick == nil {
		return
	}
	m.pick.filter = filter
	m.refreshPick()
}

// newName is the prompt for a new playlist's name, from the popup's top
// row.
type newName struct{ pick *playlistPick }

func (m *Model) startNewPlaylistName(k *playlistPick) {
	m.naming = &newName{k}
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "new playlist: "
	m.fitLine()
	m.line.SetValue(k.filter)
	m.line.CursorEnd()
	m.then(m.line.Focus())
}

// newNameKey handles the name prompt's keys other than editing: <CR>
// makes the playlist, <Esc> changes nothing.
func (m *Model) newNameKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		m.leaveLine()
		m.setMsg("nothing changed", false)
		return nil, true
	case "enter":
		k := m.naming.pick
		name := strings.TrimSpace(m.line.Value())
		m.leaveLine()
		if err := m.makePlaylist(name, k); err != nil {
			m.setMsg(err.Error(), true)
		}
		cmd := m.pendingCmd
		m.pendingCmd = nil
		return cmd, true
	}
	return nil, false
}

func (m *Model) makePlaylist(name string, k *playlistPick) error {
	if name == "" {
		return errors.New("a playlist needs a name · nothing changed")
	}
	if lib := m.state.Library; lib != nil {
		for _, o := range lib.Playlists {
			if strings.EqualFold(o.Name, name) {
				return fmt.Errorf("there's a playlist called %q already", o.Name)
			}
		}
	}
	if m.state.Offline {
		return errOffline
	}
	if k.visual {
		m.endVisual()
	}
	m.event(app.PlaylistCreate{Name: name, Tracks: k.tracks})
	return nil
}
