package ui

import (
	"errors"
	"fmt"
	"time"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

// homeRow is one of Home's rows.
type homeRow struct{ title, kind, empty string }

// homeLists are Home's rows, in the spec's order, with the server's
// album list type for each. The first, Mix, is not a list: it runs mix.
// The last, Recommended, is the user's Recommendations, from the cache.
var homeLists = []homeRow{
	{"Mix", "mix", ""},
	{"Recently added", "newest", "nothing here yet"},
	{"Most played", "frequent", "nothing played yet"},
	{"Recently played", "recent", "nothing played yet"},
	{"Random", "random", "nothing here yet"},
	{"Starred", "starred", "nothing starred yet · * stars an album"},
	{"Recommended", "recommended", ""},
}

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
	case "recommended":
		m.lib = append(m.lib, frame{kind: recommendedTracks, title: l.title})
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
	case l.Loaded && !l.Loading:
		for _, h := range homeLists {
			if h.kind == kind {
				return " " + h.empty
			}
		}
	case m.state.Offline:
		return " can't reach the server · Home's lists need it; the library works from the cache"
	}
	return " loading…"
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
	case f.kind == playlistList && f.cur < len(lib.Playlists):
		return playlist(lib.Playlists[f.cur])
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

// recommendedOf is the tracks Home's Recommended list shows, sorted and
// filtered.
func (m *Model) recommendedOf(f *frame) []library.Track {
	base := m.state.Recs().Tracks
	if !f.view.active() {
		return base
	}
	if k := m.viewKey(f); f.view.key != k || f.view.tracks == nil {
		f.view.key, f.view.tracks = k, apply(f.view, base, trackFacts)
	}
	return f.view.tracks
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
