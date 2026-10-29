package ui

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// homeRow is one of Home's rows.
type homeRow struct{ title, kind, empty string }

// homeLists are Home's rows, in the spec's order, with the server's
// album list type for each. The first, Mix, is not a list: it runs mix.
// Played this session, Forgotten favourites, Starred tracks and the
// last, Recommended, are track lists from the app, not the server, and
// Never played is an album list from the cache.
var homeLists = []homeRow{
	{"Mix", "mix", ""},
	{"Recently added", "newest", "nothing here yet · add music to the server, then :sync"},
	{"Most played", "frequent", "nothing played yet · play an album and it shows up here"},
	{"Recently played", "recent", "nothing played yet · play an album and it shows up here"},
	{"Played this session", playedTracks, ""},
	{"Random albums", "random", "nothing here yet · add music to the server, then :sync"},
	{"Forgotten favourites", forgottenTracks, ""},
	{"Never played", neverPlayed, ""},
	{"Starred albums", "starred", "nothing starred yet"},
	{"Starred tracks", starredTracks, ""},
	{"Recommended", "recommended", ""},
}

// The kinds of Home's track lists other than Recommended.
const (
	starredTracks   = "starred-tracks"
	playedTracks    = "played"
	forgottenTracks = "forgotten"
	neverPlayed     = "never-played" // an album list
)

// homeRows are the rows Home shows: Recommended only while
// Recommendations are on.
func (m *Model) homeRows() []homeRow {
	if m.state.Recs().On {
		return homeLists
	}
	return homeLists[:len(homeLists)-1]
}

// openHomeList opens one of Home's lists and asks the server for it
// again; until it answers, the last answer shows.
func (m *Model) openHomeList(i int, c command.Call) {
	l := m.homeRows()[i]
	switch l.kind {
	case "mix":
		m.mixAround(c, nil) // Home's Mix row is always the whole library
		return
	case starredTracks, playedTracks, forgottenTracks, "recommended":
		m.lib = append(m.lib, frame{kind: homeTracks, homeKind: l.kind, title: l.title})
		return
	case neverPlayed:
		m.lib = append(m.lib, frame{kind: homeAlbums, homeKind: l.kind, title: l.title})
		return
	}
	m.lib = append(m.lib, frame{kind: homeAlbums, homeKind: l.kind, title: l.title})
	m.event(app.LoadAlbumList{Kind: l.kind})
}

// homeAlbums is the albums one of Home's lists shows. An album the
// library has is taken from it, so stars and play counts match.
func (m *Model) homeAlbums(kind string) []library.Album {
	l := m.state.Home[kind]
	lib := m.state.Library
	if lib == nil {
		return l.Albums
	}
	out := make([]library.Album, len(l.Albums))
	for i, a := range l.Albums {
		if known, ok := lib.Album(a.ID); ok {
			a = known
		}
		out[i] = a
	}
	return out
}

// emptyHome says why a list of Home's has no albums.
func (m *Model) emptyHome(kind string) string {
	l := m.state.Home[kind]
	switch {
	case kind == neverPlayed && m.state.Library == nil:
		return m.emptyList()
	case kind == neverPlayed:
		return " you've played something from every album"
	case l.Loaded && !l.Loading:
		for _, h := range homeLists {
			if h.kind == kind && kind == "starred" {
				return " " + h.empty + m.starHint(
					[]string{"albums", "containers", "global"},
					"an album",
				)
			}
			if h.kind == kind {
				return " " + h.empty
			}
		}
	case m.state.Offline:
		return " can't reach the server · Home's lists need it; the library works from the cache"
	}
	return " loading…"
}

// starHint is the key that stars what, in scopes, for an empty starred
// list, as " · * stars an album".
func (m *Model) starHint(scopes []string, what string) string {
	ks := m.engine.Table().Keys(keymap.Normal, scopes, "star")
	if len(ks) == 0 {
		return " · :star stars " + what
	}
	return " · " + ks[0] + " stars " + what
}

// mix is gm and :mix: a Mix of the count's size, or MixSize, around the
// row under the cursor, else what plays, else the whole library. With
// the argument library (:mix library) it always mixes the whole library.
func (m *Model) mix(c command.Call) error {
	if err := mixArgs(c.Args); err != nil {
		return err
	}
	var seed *library.Seed
	if len(c.Args) == 0 {
		seed = m.mixSeed()
	}
	m.mixAround(c, seed)
	return nil
}

func (m *Model) mixAround(c command.Call, seed *library.Seed) {
	n := library.MixSize
	if c.Count > 0 {
		n = c.Count
	}
	m.event(app.Mix{N: n, Seed: m.seed(), At: time.Now(), Around: seed})
}

func mixArgs(a []string) error {
	if len(a) > 1 || len(a) == 1 && a[0] != "library" {
		return errors.New("mix: give library, or nothing to mix around the row")
	}
	return nil
}

func mixWords(*Model) []string { return []string{"library"} }

// mixSeed is what gm mixes around: the artist, album, playlist or track
// under the cursor, else what plays; nil for the whole library.
func (m *Model) mixSeed() *library.Seed {
	if s, ok := m.rowSeed(); ok {
		return &s
	}
	if t, _, ok := m.state.NowPlaying(); ok {
		return &library.Seed{Kind: library.SeedTrack, ID: string(t.ID), Name: t.Title}
	}
	return nil
}

// mixFrom names what gm would mix from, for the key menu: the seed's
// name, or "the whole library".
func (m *Model) mixFrom() string {
	if s := m.mixSeed(); s != nil {
		return s.Name
	}
	return "the whole library"
}

// rowSeed is the row under the cursor as a seed. A track row is always a
// track seed; sections, Home's lists, headings and Help are not seeds.
func (m *Model) rowSeed() (library.Seed, bool) {
	track := func(t library.Track) (library.Seed, bool) {
		return library.Seed{Kind: library.SeedTrack, ID: string(t.ID), Name: t.Title}, true
	}
	if m.focus == queueWindow {
		if m.queueCur < len(m.state.Queue) {
			return track(m.state.Queue[m.queueCur].Track)
		}
		return library.Seed{}, false
	}
	f, lib := m.top(), m.state.Library
	if lib == nil {
		return library.Seed{}, false
	}
	playlist := func(p library.Playlist) (library.Seed, bool) {
		return library.Seed{Kind: library.SeedPlaylist, ID: string(p.ID), Name: p.Name}, true
	}
	switch {
	case f.kind == playlistList && f.cur < len(m.playlistsOf(f)):
		return playlist(m.playlistsOf(f)[f.cur])
	case f.kind == searchList:
		if p, ok := f.search.playlist(f.cur); ok {
			return playlist(p)
		}
	}
	it, ok := m.rowItem(f, f.cur)
	if !ok {
		return library.Seed{}, false
	}
	kind := map[library.Group]library.SeedKind{
		library.ArtistGroup: library.SeedArtist,
		library.AlbumGroup:  library.SeedAlbum,
		library.TrackGroup:  library.SeedTrack,
	}[it.group]
	return library.Seed{Kind: kind, ID: it.id, Name: it.name}, kind != 0
}

// mixRowNote follows Home's Mix row while the first sync runs, since a
// Mix needs the whole library: whether one waits, and how far along it is.
func (m *Model) mixRowNote(i int) string {
	sy := m.state.Sync
	if m.homeRows()[i].kind != "mix" || m.state.Library != nil || !sy.Running {
		return ""
	}
	note := " · once the library has synced"
	if m.state.MixWaiting() {
		note = " · starts once the library has synced"
	}
	if sy.Total > 0 {
		note += fmt.Sprintf(" · %d / %d tracks", sy.Done, sy.Total)
	}
	return note
}

// onMixRow reports whether the cursor is on Home's Mix row.
func (m *Model) onMixRow() bool {
	f := m.top()
	return f.kind == homeList && f.cur < len(m.homeRows()) && m.homeRows()[f.cur].kind == "mix"
}

// homeTracksOf is the tracks one of Home's track lists shows, sorted and
// filtered.
func (m *Model) homeTracksOf(f *frame) []library.Track {
	base := m.homeTracksBase(f)
	if !f.view.active() {
		return base
	}
	if k := m.viewKey(f); f.view.key != k || f.view.tracks == nil {
		f.view.key, f.view.tracks = k, apply(f.view, base, m.homeTrackFacts(f))
	}
	return f.view.tracks
}

// homeTracksBase is a Home track list's tracks in its own order. Starred
// tracks keeps the tracks starred when it opened, so one unstarred there
// stays (without its ★) until the list opens again.
func (m *Model) homeTracksBase(f *frame) []library.Track {
	lib := m.state.Library
	var choose func() []library.Track
	switch f.homeKind {
	case playedTracks:
		return m.playedTracks()
	case starredTracks:
		choose = func() []library.Track { return lib.StarredTracks() }
	case forgottenTracks:
		choose = func() []library.Track {
			now := m.clock.Now()
			return lib.ForgottenFavourites(homeListSize, now, m.rand())
		}
	default:
		return m.state.Recs().Tracks
	}
	if lib == nil {
		return nil
	}
	v := &f.frozen
	if v.tracks == nil {
		v.tracks = []library.TrackID{}
		for _, t := range choose() {
			v.tracks = append(v.tracks, t.ID)
		}
	}
	if v.lib != lib || v.rows == nil {
		v.lib, v.rows = lib, make([]library.Track, 0, len(v.tracks))
		for _, id := range v.tracks {
			if t, ok := lib.Track(id); ok {
				v.rows = append(v.rows, t)
			}
		}
	}
	return v.rows
}

// homeListSize is how many rows a Home list chosen from the cache shows,
// as many as the server's lists.
const homeListSize = 50

// rand is a random source for a pick made in the UI.
func (m *Model) rand() *rand.Rand {
	s := m.seed()
	return rand.New(rand.NewPCG(s, s^0x9e3779b97f4a7c15))
}

// neverPlayedAlbums is Home's Never played list, chosen when it opened.
func (m *Model) neverPlayedAlbums(f *frame) []library.Album {
	lib := m.state.Library
	if lib == nil {
		return nil
	}
	v := &f.frozen
	if v.albums == nil {
		v.albums = []library.AlbumID{}
		for _, a := range lib.NeverPlayed(homeListSize, m.rand()) {
			v.albums = append(v.albums, a.ID)
		}
	}
	if v.lib != lib || v.albumRows == nil {
		v.lib, v.albumRows = lib, make([]library.Album, 0, len(v.albums))
		for _, id := range v.albums {
			if a, ok := lib.Album(id); ok {
				v.albumRows = append(v.albumRows, a)
			}
		}
	}
	return v.albumRows
}

// homeTrackFacts is what :sort and :filter look at in a Home track list;
// Starred tracks adds when each was starred.
func (m *Model) homeTrackFacts(f *frame) func(library.Track) facts {
	lib := m.state.Library
	if f.homeKind != starredTracks || lib == nil {
		return trackFacts
	}
	return func(t library.Track) facts {
		x := trackFacts(t)
		x.starred = lib.StarredAt(string(t.ID))
		return x
	}
}

// playedTracks is the tracks played this session, as the library has
// them now, so stars and ratings are current.
func (m *Model) playedTracks() []library.Track {
	lib := m.state.Library
	if lib == nil {
		return m.state.Played
	}
	out := make([]library.Track, len(m.state.Played))
	for i, t := range m.state.Played {
		if known, ok := lib.Track(t.ID); ok {
			t = known
		}
		out[i] = t
	}
	return out
}

// keepPlayedCursor runs change, a track starting, keeping the cursor of
// an open Played this session list on its track as the list grows above
// it.
func (m *Model) keepPlayedCursor(change func() tea.Cmd) tea.Cmd {
	f := m.top()
	if f.kind != homeTracks || f.homeKind != playedTracks || f.cur >= m.rows(f) {
		return change()
	}
	id := m.rowID(f, f.cur)
	cmd := change()
	for i := range m.rows(f) {
		if m.rowID(f, i) == id {
			f.cur = i
			break
		}
	}
	return cmd
}

// showPlayed is :played: Home's Played this session list.
func (m *Model) showPlayed(c command.Call) error {
	m.showSection(homeList)
	m.lib = m.lib[:2]
	for i, r := range m.homeRows() {
		if r.kind == playedTracks {
			m.top().cur = i
			m.openHomeList(i, c)
		}
	}
	return nil
}

// emptyHomeTracks says why one of Home's track lists has no rows.
func (m *Model) emptyHomeTracks(f *frame) string {
	switch {
	case f.homeKind == playedTracks:
		return " nothing played yet this session · <CR> on a track plays it"
	case f.homeKind == forgottenTracks && m.state.Library == nil:
		return m.emptyList()
	case f.homeKind == forgottenTracks:
		return " nothing forgotten · favourites unplayed for 6 months show up here"
	case f.homeKind != starredTracks:
		return m.emptyRecs()
	case m.state.Library == nil:
		return m.emptyList()
	}
	return " nothing starred yet" + m.starHint(
		[]string{"tracks", "global"},
		"the track under the cursor",
	)
}

// emptyRecs says why the Recommended list has no rows.
func (m *Model) emptyRecs() string {
	switch v := m.state.Recs(); {
	case m.state.Library == nil:
		return m.emptyList()
	case !v.On:
		return " Recommendations are off · set listenbrainz.user in config.yaml"
	case v.Fetched:
		if v.Total == 1 {
			return " your one Recommendation isn't in the library"
		}
		return fmt.Sprintf(
			" none of your %s Recommendations are in the library",
			app.Thousands(v.Total),
		)
	case v.NotGenerated:
		return " ListenBrainz has no Recommendations for " + v.User + " yet"
	}
	return " no Recommendations yet · navigo asks ListenBrainz at startup and on :sync"
}
