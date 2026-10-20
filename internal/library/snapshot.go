package library

import (
	"cmp"
	"slices"
	"strings"
	"time"
)

// Snapshot is the library as of one sync. It is immutable: a sync builds
// a new one, which replaces the old in one step. Its exported fields are
// what the cache file stores (gob); the indexes are rebuilt on load.
//
// gob matches fields by name and ignores missing ones, so any change to
// the stored shape here or in Artist, Album, Track or Playlist must bump
// cacheVersion, or old cache files would load with fields zeroed.
type Snapshot struct {
	Server, User string
	// LastScan is the server's scan time the snapshot was synced at; a
	// later scan means a full sync is due.
	LastScan time.Time
	SyncedAt time.Time

	Artists   []Artist // by sort name
	Albums    []Album  // by sort name
	Tracks    []Track  // by album, disc, number
	Playlists []Playlist
	Starred   []string // IDs of starred artists, albums and tracks

	artistByID map[ArtistID]int
	albumByID  map[AlbumID]int
	trackByID  map[TrackID]int
	albumSpan  map[AlbumID][2]int // the album's range in Tracks
	own        map[ArtistID][]int // indexes in Albums, as album artist
	appearsOn  map[ArtistID][]int // indexes in Albums, credited on tracks only
	starred    map[string]bool
	search     *searchIndex
}

// Build sorts the synced data, derives what the server doesn't send, and
// returns a ready snapshot. Artists named only on tracks are added,
// artists with neither a track nor an album are dropped, and each
// artist's AlbumCount counts the albums they are album artist of.
func Build(
	server, user string,
	lastScan time.Time,
	artists []Artist,
	albums []Album,
	tracks []Track,
	playlists []Playlist,
	starred []string,
) *Snapshot {
	s := &Snapshot{
		Server: server, User: user, LastScan: lastScan, SyncedAt: time.Now(),
		Artists: slices.Clone(artists), Albums: slices.Clone(albums), Tracks: slices.Clone(tracks),
		Playlists: slices.Clone(playlists), Starred: slices.Clone(starred),
	}

	seen := map[ArtistID]bool{}
	for _, a := range s.Artists {
		seen[a.ID] = true
	}
	credited := map[ArtistID]bool{}
	for _, t := range s.Tracks {
		for _, a := range t.Artists {
			credited[a.ID] = true
			if !seen[a.ID] && a.ID != "" {
				seen[a.ID] = true
				s.Artists = append(s.Artists, Artist{ID: a.ID, Name: a.Name})
			}
		}
	}
	own := map[ArtistID]int{}
	for _, a := range s.Albums {
		own[a.ArtistID]++
	}
	s.Artists = slices.DeleteFunc(
		s.Artists,
		func(a Artist) bool { return own[a.ID] == 0 && !credited[a.ID] },
	)
	for i := range s.Artists {
		s.Artists[i].AlbumCount = own[s.Artists[i].ID]
	}

	slices.SortStableFunc(
		s.Artists,
		func(a, b Artist) int { return compareNames(a.SortName, a.Name, b.SortName, b.Name) },
	)
	slices.SortStableFunc(s.Albums, func(a, b Album) int {
		return cmp.Or(
			compareNames(a.SortName, a.Name, b.SortName, b.Name),
			cmp.Compare(a.Artist, b.Artist),
		)
	})
	order := map[AlbumID]int{}
	for i, a := range s.Albums {
		order[a.ID] = i
	}
	slices.SortStableFunc(s.Tracks, func(a, b Track) int {
		return cmp.Or(
			cmp.Compare(order[a.AlbumID], order[b.AlbumID]),
			cmp.Compare(a.Disc, b.Disc),
			cmp.Compare(a.Number, b.Number),
		)
	})
	s.index()
	return s
}

// compareNames orders by sort name, falling back to the lower-cased name
// when the server sent none.
func compareNames(sortA, nameA, sortB, nameB string) int {
	ka, kb := sortA, sortB
	if ka == "" {
		ka = strings.ToLower(nameA)
	}
	if kb == "" {
		kb = strings.ToLower(nameB)
	}
	return cmp.Or(cmp.Compare(ka, kb), cmp.Compare(nameA, nameB))
}

func (s *Snapshot) index() {
	s.artistByID = make(map[ArtistID]int, len(s.Artists))
	for i, a := range s.Artists {
		s.artistByID[a.ID] = i
	}
	s.albumByID = make(map[AlbumID]int, len(s.Albums))
	for i, a := range s.Albums {
		s.albumByID[a.ID] = i
	}
	s.trackByID = make(map[TrackID]int, len(s.Tracks))
	s.albumSpan = map[AlbumID][2]int{}
	for i, t := range s.Tracks {
		s.trackByID[t.ID] = i
		sp, ok := s.albumSpan[t.AlbumID]
		if !ok {
			sp[0] = i
		}
		sp[1] = i + 1
		s.albumSpan[t.AlbumID] = sp
	}
	s.own, s.appearsOn = map[ArtistID][]int{}, map[ArtistID][]int{}
	for i, a := range s.Albums {
		s.own[a.ArtistID] = append(s.own[a.ArtistID], i)
		seen := map[ArtistID]bool{a.ArtistID: true}
		for _, t := range s.AlbumTracks(a.ID) {
			for _, c := range t.Artists {
				if !seen[c.ID] {
					seen[c.ID] = true
					s.appearsOn[c.ID] = append(s.appearsOn[c.ID], i)
				}
			}
		}
	}
	s.indexStars()
	s.buildSearch()
}

func (s *Snapshot) indexStars() {
	s.starred = make(map[string]bool, len(s.Starred))
	for _, id := range s.Starred {
		s.starred[id] = true
	}
}

// Artist returns an artist by ID.
func (s *Snapshot) Artist(id ArtistID) (Artist, bool) {
	i, ok := s.artistByID[id]
	if !ok {
		return Artist{}, false
	}
	return s.Artists[i], true
}

// Album returns an album by ID.
func (s *Snapshot) Album(id AlbumID) (Album, bool) {
	i, ok := s.albumByID[id]
	if !ok {
		return Album{}, false
	}
	return s.Albums[i], true
}

// Track returns a track by ID.
func (s *Snapshot) Track(id TrackID) (Track, bool) {
	i, ok := s.trackByID[id]
	if !ok {
		return Track{}, false
	}
	return s.Tracks[i], true
}

// AlbumTracks returns an album's tracks in disc and track order. The
// slice is shared: don't change it.
func (s *Snapshot) AlbumTracks(id AlbumID) []Track {
	sp, ok := s.albumSpan[id]
	if !ok {
		return nil
	}
	return s.Tracks[sp[0]:sp[1]:sp[1]]
}

// ArtistAlbums returns the albums an artist is album artist of, and the
// ones they appear on: credited on a track but not the album artist.
// Both are in the order of Albums.
func (s *Snapshot) ArtistAlbums(id ArtistID) (own, appearsOn []Album) {
	for _, i := range s.own[id] {
		own = append(own, s.Albums[i])
	}
	for _, i := range s.appearsOn[id] {
		appearsOn = append(appearsOn, s.Albums[i])
	}
	return own, appearsOn
}

// AppearsOn is the number of albums an artist appears on without being
// their album artist.
func (s *Snapshot) AppearsOn(id ArtistID) int { return len(s.appearsOn[id]) }

// Playlist returns a playlist by ID.
func (s *Snapshot) Playlist(id PlaylistID) (Playlist, bool) {
	i := slices.IndexFunc(s.Playlists, func(p Playlist) bool { return p.ID == id })
	if i < 0 {
		return Playlist{}, false
	}
	return s.Playlists[i], true
}

// PlaylistTracks returns a playlist's tracks in order, one per entry, so
// row i is the server's index i. A track the library no longer has is a
// placeholder titled "(not in the library)".
func (s *Snapshot) PlaylistTracks(id PlaylistID) []Track {
	p, ok := s.Playlist(id)
	if !ok {
		return nil
	}
	out := make([]Track, 0, len(p.Tracks))
	for _, t := range p.Tracks {
		tr, ok := s.Track(t)
		if !ok {
			tr = Track{ID: t, Title: "(not in the library)"}
		}
		out = append(out, tr)
	}
	return out
}

// WithPlaylist returns a copy with p added, or replacing the playlist
// with its ID, as after an edit on the server.
func (s *Snapshot) WithPlaylist(p Playlist) *Snapshot {
	n := *s
	n.Playlists = slices.Clone(s.Playlists)
	if i := slices.IndexFunc(n.Playlists, func(o Playlist) bool { return o.ID == p.ID }); i >= 0 {
		n.Playlists[i] = p
	} else {
		n.Playlists = append(n.Playlists, p)
	}
	return &n
}

// WithoutPlaylist returns a copy without a playlist.
func (s *Snapshot) WithoutPlaylist(id PlaylistID) *Snapshot {
	n := *s
	n.Playlists = slices.DeleteFunc(
		slices.Clone(s.Playlists),
		func(p Playlist) bool { return p.ID == id },
	)
	return &n
}

// IsStarred reports whether an artist, album or track ID is starred.
func (s *Snapshot) IsStarred(id string) bool { return s.starred[id] }

// WithExtras returns a copy with new playlists and stars, as a refresh
// without a full sync produces. It shares the unchanged indexes.
func (s *Snapshot) WithExtras(playlists []Playlist, starred []string) *Snapshot {
	n := *s
	n.Playlists, n.Starred = slices.Clone(playlists), slices.Clone(starred)
	n.SyncedAt = time.Now()
	n.indexStars()
	return &n
}
