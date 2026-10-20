package ui

import (
	"errors"
	"fmt"
	"slices"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

// sections are the library window's top-level entries, in the spec's
// order; Home, Playlists, Search and Help come later.
var sections = []section{{"Artists", artistList}, {"Albums", albumList}}

type section struct {
	title string
	kind  listKind
}

// sectionPath is the section list with its cursor on a section, then
// that section's list.
func sectionPath(kind listKind) []frame {
	i := slices.IndexFunc(sections, func(s section) bool { return s.kind == kind })
	return []frame{{kind: sectionList, title: "Library", cur: i}, {kind: kind, title: sections[i].title}}
}

// startPath is the library window at startup.
func startPath() []frame { return sectionPath(albumList) }

// showSection switches the library window to a section, back where it was
// left; asked for the section it is in, it goes to the section's list.
func (m *Model) showSection(kind listKind) {
	m.focus = libraryWindow
	if len(m.lib) > 1 && m.lib[1].kind == kind {
		m.lib = m.lib[:2]
		return
	}
	m.stash()
	if p, ok := m.paths[kind]; ok {
		m.lib = slices.Clone(p)
		return
	}
	m.lib = sectionPath(kind)
}

// stash remembers the path of the section the library window is in.
func (m *Model) stash() {
	if len(m.lib) > 1 {
		m.paths[m.lib[1].kind] = slices.Clone(m.lib)
	}
}

// goTo replaces the library window's path with a section and frames
// below it, as ga and gr do, and focuses the library window.
func (m *Model) goTo(path ...frame) {
	m.stash()
	m.focus = libraryWindow
	m.lib = append(sectionPath(path[0].kind)[:1], path...)
	m.clamp()
}

// artistRows is an artist page: their own albums, then, when there are
// any, the Appears on heading and the albums they appear on.
type artistRows struct{ own, appearsOn []library.Album }

func (m *Model) artistRows(id library.ArtistID) artistRows {
	if m.state.Library == nil {
		return artistRows{}
	}
	own, appears := m.state.Library.ArtistAlbums(id)
	return artistRows{own, appears}
}

func (r artistRows) len() int {
	if len(r.appearsOn) == 0 {
		return len(r.own)
	}
	return len(r.own) + 1 + len(r.appearsOn)
}

// heading is the row of the Appears on heading, or -1.
func (r artistRows) heading() int {
	if len(r.appearsOn) == 0 {
		return -1
	}
	return len(r.own)
}

// album is the album on row i; false for the heading.
func (r artistRows) album(i int) (library.Album, bool) {
	switch h := r.heading(); {
	case i < len(r.own):
		return r.own[i], true
	case h >= 0 && i > h && i-h-1 < len(r.appearsOn):
		return r.appearsOn[i-h-1], true
	}
	return library.Album{}, false
}

// index is the row of an album, or -1.
func (r artistRows) index(id library.AlbumID) int {
	for i := range r.len() {
		if a, ok := r.album(i); ok && a.ID == id {
			return i
		}
	}
	return -1
}

// skipped reports whether row i is one the cursor passes over.
func (m *Model) skipped(f *frame, i int) bool {
	return f.kind == artistPage && i == m.artistRows(f.artist).heading()
}

// settle moves the cursor off a skipped row, in direction dir if it can.
func (m *Model) settle(f *frame, dir int) {
	if !m.skipped(f, f.cur) {
		return
	}
	n := m.rows(f)
	for _, c := range []int{f.cur + dir, f.cur - dir} {
		if c >= 0 && c < n && !m.skipped(f, c) {
			f.cur = c
			return
		}
	}
}

// groups are the first rows of a list's groups, for { and }.
func (m *Model) groups(f *frame) []int {
	if f.kind == artistPage {
		if h := m.artistRows(f.artist).heading(); h > 0 {
			return []int{0, h + 1}
		} else if h == 0 {
			return []int{1}
		}
	}
	return []int{0}
}

// group moves to the start of the next group (dir 1), or of this or the
// previous group (dir -1); past the last group it goes to the end.
func (m *Model) group(dir, count int) {
	if m.popup != nil || m.focus == queueWindow { // one group: its ends
		if dir > 0 {
			m.goRow(0, -1)
		} else {
			m.goRow(0, 0)
		}
		return
	}
	f := m.top()
	gs := m.groups(f)
	for range count {
		next := -1
		if dir > 0 {
			if i := slices.IndexFunc(gs, func(g int) bool { return g > f.cur }); i >= 0 {
				next = gs[i]
			} else {
				next = m.rows(f) - 1
			}
		} else {
			next = 0
			for _, g := range gs {
				if g < f.cur {
					next = g
				}
			}
		}
		f.cur = max(next, 0)
		m.settle(f, dir)
	}
}

// openArtist opens an artist's page from the Artists list.
func (m *Model) openArtist(a library.Artist) {
	f := frame{kind: artistPage, artist: a.ID, title: a.Name}
	m.lib = append(m.lib, f)
	m.settle(m.top(), 1)
}

// openAlbum opens an album's tracks. From an artist page, an album they
// only appear on opens on their first track.
func (m *Model) openAlbum(a library.Album, artist library.ArtistID) {
	f := frame{kind: albumTracks, album: a.ID, title: a.Name}
	if artist != "" && a.ArtistID != artist && m.state.Library != nil {
		if i := slices.IndexFunc(m.state.Library.AlbumTracks(a.ID), func(t library.Track) bool { return t.Credits(artist) }); i >= 0 {
			f.cur = i
		}
	}
	m.lib = append(m.lib, f)
}

var errNotOnAlbum = errors.New("not on a track or an album")

// rowTrack is the track under the cursor, in the queue or an album.
func (m *Model) rowTrack() (library.Track, bool) {
	if m.focus == queueWindow {
		if m.queueCur < len(m.state.Queue) {
			return m.state.Queue[m.queueCur].Track, true
		}
		return library.Track{}, false
	}
	ts, i, err := m.trackRow()
	if err != nil {
		return library.Track{}, false
	}
	return ts[i], true
}

// rowAlbum is the album row under the cursor in the library window.
func (m *Model) rowAlbum() (library.Album, bool) {
	f := m.top()
	lib := m.state.Library
	if m.focus != libraryWindow || lib == nil {
		return library.Album{}, false
	}
	switch f.kind {
	case albumList:
		if f.cur < len(lib.Albums) {
			return lib.Albums[f.cur], true
		}
	case artistPage:
		return m.artistRows(f.artist).album(f.cur)
	}
	return library.Album{}, false
}

// goAlbum is ga: the album of the row, with the cursor on its track.
func (m *Model) goAlbum(command.Call) error {
	lib := m.state.Library
	var id library.AlbumID
	var track library.TrackID
	if t, ok := m.rowTrack(); ok {
		id, track = t.AlbumID, t.ID
	} else if a, ok := m.rowAlbum(); ok {
		id = a.ID
	} else {
		return errNotOnAlbum
	}
	a, ok := library.Album{}, false
	if lib != nil {
		a, ok = lib.Album(id)
	}
	if !ok {
		return errors.New("that album isn't in the library")
	}
	tracks := frame{kind: albumTracks, album: a.ID, title: a.Name}
	if i := slices.IndexFunc(lib.AlbumTracks(a.ID), func(t library.Track) bool { return t.ID == track }); i >= 0 {
		tracks.cur = i
	}
	m.goTo(frame{kind: albumList, title: "Albums", cur: slices.IndexFunc(lib.Albums, func(b library.Album) bool { return b.ID == a.ID })}, tracks)
	return nil
}

// goArtist is gr: the artist of the row, with the cursor on the row's
// album. A track with several artists asks which one.
func (m *Model) goArtist(command.Call) error {
	var refs []library.ArtistRef
	var album library.AlbumID
	if t, ok := m.rowTrack(); ok {
		refs, album = t.Artists, t.AlbumID
		if len(refs) == 0 && t.ArtistID != "" {
			refs = []library.ArtistRef{{ID: t.ArtistID, Name: t.Artist}}
		}
	} else if a, ok := m.rowAlbum(); ok {
		refs, album = []library.ArtistRef{{ID: a.ArtistID, Name: a.Artist}}, a.ID
	} else {
		return errNotOnAlbum
	}
	switch len(refs) {
	case 0:
		return errors.New("no artist is credited here")
	case 1:
		return m.showArtist(refs[0].ID, album)
	}
	names := make([]string, len(refs))
	for i, r := range refs {
		names[i] = r.Name
	}
	m.openChoice("Go to which artist?", names, func(i int) error { return m.showArtist(refs[i].ID, album) })
	return nil
}

// showArtist opens an artist's page in the Artists section, with the
// cursor on an album if it is there.
func (m *Model) showArtist(id library.ArtistID, album library.AlbumID) error {
	lib := m.state.Library
	var a library.Artist
	ok := false
	if lib != nil {
		a, ok = lib.Artist(id)
	}
	if !ok {
		return errors.New("that artist isn't in the library")
	}
	page := frame{kind: artistPage, artist: a.ID, title: a.Name, cur: max(m.artistRows(a.ID).index(album), 0)}
	m.goTo(frame{kind: artistList, title: "Artists", cur: slices.IndexFunc(lib.Artists, func(b library.Artist) bool { return b.ID == a.ID })}, page)
	m.settle(m.top(), 1)
	return nil
}

// artistCount is an Artists row's right side: own albums, or the albums
// they appear on.
func artistCount(own, appears int) string {
	switch {
	case own == 1:
		return "1 album"
	case own > 1:
		return fmt.Sprintf("%d albums", own)
	}
	return fmt.Sprintf("appears on %d", appears)
}
