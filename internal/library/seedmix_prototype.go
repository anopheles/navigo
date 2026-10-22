package library

// PROTOTYPE, throwaway: lives only on branch prototype/seeded-mix, for
// "Does cache-only closeness feel like a radio on a real library?" (#68).
// A rough picker for a Mix around a seed, from the cache alone.

import (
	"cmp"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"
)

// ProtoSeed is what a Mix is built around.
type ProtoSeed struct {
	Kind    string // artist, album, playlist, track
	ID      string
	Name    string
	Tracks  []Track              // the seed's own tracks
	Artists map[ArtistID]float64 // seed artists, by share of the seed
	Genres  map[string]float64   // seed genres, by share of the seed's tracks
	Year    int                  // median year of the seed's tracks; 0 if none
	// Similar is the server's similar artists, by rank (1 = most); set by
	// the caller for the hybrid picker.
	Similar  map[string]float64 // by lower-case name
	names    map[string]float64 // seed artists by lower-case name
	inSeed   map[TrackID]bool
	playlist PlaylistID // the seed playlist, left out of the playlist signal
}

// ProtoWeights are the prototype's knobs.
type ProtoWeights struct {
	Seed, Artist, Linked, Playlist, Genre, Year, Similar float64
	Power                                                float64 // closeness^Power × the Mix weight
	SeedCap, PerArtist, PerAlbum                         int
	MaxPlaylist                                          int // playlists bigger than this don't count
}

// ProtoPick is one scored track, with the signals that fired.
type ProtoPick struct {
	Track     Track
	Closeness float64
	Weight    float64
	Why       []string
	FromSeed  bool // in the seed, or credits a seed artist
}

// ProtoResolve finds a seed by kind and a name or ID. It returns the
// other matches too, so the caller can show them.
func (s *Snapshot) ProtoResolve(kind, q string, r *rand.Rand) (*ProtoSeed, []string, error) {
	lq := strings.ToLower(q)
	match := func(names ...string) (exact, part bool) {
		for _, n := range names {
			ln := strings.ToLower(n)
			if ln == lq {
				exact = true
			}
			if strings.Contains(ln, lq) {
				part = true
			}
		}
		return
	}
	pick := func(n int, name func(i int) []string, id func(i int) string) (int, []string) {
		var exacts, parts []int
		for i := range n {
			if id(i) == q {
				return i, nil
			}
			e, p := match(name(i)...)
			if e {
				exacts = append(exacts, i)
			} else if p {
				parts = append(parts, i)
			}
		}
		all := append(exacts, parts...)
		if q == "" && n > 0 { // random
			all = []int{r.IntN(n)}
		}
		if len(all) == 0 {
			return -1, nil
		}
		var others []string
		for _, i := range all[1:min(len(all), 8)] {
			others = append(others, strings.Join(name(i), " — "))
		}
		return all[0], others
	}

	seed := &ProtoSeed{Kind: kind, inSeed: map[TrackID]bool{}, Artists: map[ArtistID]float64{}}
	var others []string
	switch kind {
	case "artist":
		i, o := pick(len(s.Artists), func(i int) []string { return []string{s.Artists[i].Name} },
			func(i int) string { return string(s.Artists[i].ID) })
		if i < 0 {
			return nil, nil, fmt.Errorf("no artist matches %q", q)
		}
		a := s.Artists[i]
		seed.ID, seed.Name, others = string(a.ID), a.Name, o
		own := map[AlbumID]bool{}
		for _, al := range s.Albums {
			if al.ArtistID == a.ID {
				own[al.ID] = true
			}
		}
		for _, t := range s.Tracks {
			if t.Credits(a.ID) || t.ArtistID == a.ID || own[t.AlbumID] {
				seed.Tracks = append(seed.Tracks, t)
			}
		}
		seed.Artists[a.ID] = 1
	case "album":
		i, o := pick(len(s.Albums), func(i int) []string { return []string{s.Albums[i].Artist, s.Albums[i].Name} },
			func(i int) string { return string(s.Albums[i].ID) })
		if i < 0 {
			return nil, nil, fmt.Errorf("no album matches %q", q)
		}
		al := s.Albums[i]
		seed.ID, seed.Name, others = string(al.ID), al.Artist+" — "+al.Name, o
		seed.Tracks = s.AlbumTracks(al.ID)
		seed.Artists[al.ArtistID] = 1
	case "playlist":
		i, o := pick(len(s.Playlists), func(i int) []string { return []string{s.Playlists[i].Name} },
			func(i int) string { return string(s.Playlists[i].ID) })
		if i < 0 {
			return nil, nil, fmt.Errorf("no playlist matches %q", q)
		}
		p := s.Playlists[i]
		seed.ID, seed.Name, others, seed.playlist = string(p.ID), p.Name, o, p.ID
		for _, t := range s.PlaylistTracks(p.ID) {
			if t.Artist != "" || t.Album != "" {
				seed.Tracks = append(seed.Tracks, t)
			}
		}
	case "track":
		i, o := pick(len(s.Tracks), func(i int) []string { return []string{s.Tracks[i].Artist, s.Tracks[i].Title} },
			func(i int) string { return string(s.Tracks[i].ID) })
		if i < 0 {
			return nil, nil, fmt.Errorf("no track matches %q", q)
		}
		t := s.Tracks[i]
		seed.ID, seed.Name, others = string(t.ID), t.Artist+" — "+t.Title, o
		seed.Tracks = []Track{t}
	default:
		return nil, nil, fmt.Errorf("kind must be artist, album, playlist or track, not %q", kind)
	}
	if len(seed.Tracks) == 0 {
		return nil, nil, fmt.Errorf("%s %q has no tracks in the cache", kind, seed.Name)
	}

	// Shares of the seed's artists, genres, and its median year.
	credit := map[ArtistID]int{}
	seed.Genres = map[string]float64{}
	var years []int
	for _, t := range seed.Tracks {
		seed.inSeed[t.ID] = true
		for _, a := range trackArtists(t) {
			credit[a]++
		}
		for _, g := range splitGenres(t.Genre) {
			seed.Genres[g]++
		}
		if t.Year > 0 {
			years = append(years, t.Year)
		}
	}
	n := float64(len(seed.Tracks))
	for a, c := range credit {
		seed.Artists[a] = max(seed.Artists[a], float64(c)/n)
	}
	for g := range seed.Genres {
		seed.Genres[g] /= n
	}
	seed.names = map[string]float64{}
	for a, v := range seed.Artists {
		if ar, ok := s.Artist(a); ok {
			k := strings.ToLower(ar.Name)
			seed.names[k] = max(seed.names[k], v)
		}
	}
	if len(years) > 0 {
		sort.Ints(years)
		seed.Year = years[len(years)/2]
	}
	return seed, others, nil
}

func trackArtists(t Track) []ArtistID {
	var out []ArtistID
	for _, a := range t.Artists {
		if a.ID != "" {
			out = append(out, a.ID)
		}
	}
	if len(out) == 0 && t.ArtistID != "" {
		out = append(out, t.ArtistID)
	}
	return out
}

func splitGenres(g string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(g, func(r rune) bool { return r == ';' || r == ',' || r == '/' }) {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// protoLinks links two artists credited on the same track, or on the same
// small album (an album with more than 4 artists is taken as a
// compilation and links nobody through the album).
func (s *Snapshot) protoLinks() map[ArtistID]map[ArtistID]bool {
	links := map[ArtistID]map[ArtistID]bool{}
	link := func(set []ArtistID) {
		for _, a := range set {
			for _, b := range set {
				if a != b {
					if links[a] == nil {
						links[a] = map[ArtistID]bool{}
					}
					links[a][b] = true
				}
			}
		}
	}
	for _, al := range s.Albums {
		set := map[ArtistID]bool{}
		if al.ArtistID != "" {
			set[al.ArtistID] = true
		}
		for _, t := range s.AlbumTracks(al.ID) {
			ta := trackArtists(t)
			link(ta)
			for _, a := range ta {
				set[a] = true
			}
		}
		if len(set) <= 4 {
			ids := make([]ArtistID, 0, len(set))
			for a := range set {
				ids = append(ids, a)
			}
			link(ids)
		}
	}
	return links
}

// ProtoScore scores every track in the library against the seed. A
// track with closeness 0 is not a candidate.
func (s *Snapshot) ProtoScore(seed *ProtoSeed, w ProtoWeights, now time.Time) map[TrackID]ProtoPick {
	links := s.protoLinks()
	linkedTo := func(a ArtistID) bool {
		for sa := range seed.Artists {
			if links[sa][a] {
				return true
			}
		}
		return false
	}
	// How many usable playlists (not the seed's, not huge) hold a seed
	// track and also each candidate.
	coPlaylists := map[TrackID]int{}
	for _, p := range s.Playlists {
		if p.ID == seed.playlist || len(p.Tracks) > w.MaxPlaylist {
			continue
		}
		if !slices.ContainsFunc(p.Tracks, func(t TrackID) bool { return seed.inSeed[t] }) {
			continue
		}
		seen := map[TrackID]bool{}
		for _, t := range p.Tracks {
			if !seen[t] {
				seen[t] = true
				coPlaylists[t]++
			}
		}
	}

	out := map[TrackID]ProtoPick{}
	for _, t := range s.Tracks {
		var c float64
		var why []string
		fromSeed := false
		if seed.inSeed[t.ID] {
			c += w.Seed
			why = append(why, "seed")
			fromSeed = true
		}
		best, linked := 0.0, false
		sim := 0.0
		for _, a := range trackArtists(t) {
			name := ""
			if ar, ok := s.Artist(a); ok {
				name = strings.ToLower(ar.Name)
			}
			share := max(seed.Artists[a], seed.names[name])
			best = max(best, share)
			if share == 0 && linkedTo(a) {
				linked = true
			}
			sim = max(sim, seed.Similar[name])
		}
		if best > 0 {
			c += w.Artist * math.Sqrt(best)
			why = append(why, fmt.Sprintf("artist(%.0f%%)", best*100))
			if best >= 0.5 {
				fromSeed = true
			}
		} else if sim > 0 {
			c += w.Similar * sim
			why = append(why, fmt.Sprintf("similar(%.0f%%)", sim*100))
		} else if linked {
			c += w.Linked
			why = append(why, "linked")
		}
		if n := coPlaylists[t.ID]; n > 0 && !seed.inSeed[t.ID] {
			c += w.Playlist * float64(min(n, 3)) / 3
			why = append(why, fmt.Sprintf("playlist×%d", n))
		}
		g := 0.0
		for _, tg := range splitGenres(t.Genre) {
			g = max(g, seed.Genres[tg])
		}
		if g > 0 {
			c += w.Genre * g
			why = append(why, fmt.Sprintf("genre(%.0f%%)", g*100))
		}
		if seed.Year > 0 && t.Year > 0 {
			d := math.Abs(float64(t.Year - seed.Year))
			if y := math.Exp(-d / 8); y > 0.3 && c > 0 { // era alone is not a reason
				c += w.Year * y
				why = append(why, fmt.Sprintf("year%+d", t.Year-seed.Year))
			}
		}
		if c == 0 {
			continue
		}
		mw := s.mixWeight(t, now)
		out[t.ID] = ProtoPick{Track: t, Closeness: c, Weight: math.Pow(c, w.Power) * mw, Why: why, FromSeed: fromSeed}
	}
	return out
}

// ProtoMix picks up to n tracks from the scored candidates, weighted at
// random like Mix, with Mix's variety rules, except that tracks from the
// seed may take up to SeedCap places and are not held to PerArtist and
// PerAlbum.
func (s *Snapshot) ProtoMix(scored map[TrackID]ProtoPick, n int, w ProtoWeights, r *rand.Rand) []ProtoPick {
	type keyed struct {
		key float64
		p   ProtoPick
	}
	order := make([]keyed, 0, len(scored))
	for _, p := range scored {
		if p.Weight > 0 {
			order = append(order, keyed{0, p})
		}
	}
	// Map order is random; sort on ID first so a -rand seed repeats.
	slices.SortFunc(order, func(a, b keyed) int { return cmp.Compare(a.p.Track.ID, b.p.Track.ID) })
	for i := range order {
		order[i].key = -math.Log(1-r.Float64()) / order[i].p.Weight
	}
	slices.SortFunc(order, func(a, b keyed) int { return cmp.Compare(a.key, b.key) })

	var (
		out      []ProtoPick
		held     []ProtoPick
		fromSeed int
		byArtist = map[ArtistID]int{}
		byAlbum  = map[AlbumID]int{}
	)
	fits := func(p ProtoPick) bool {
		if p.FromSeed {
			return fromSeed < w.SeedCap
		}
		return byArtist[p.Track.ArtistID] < w.PerArtist && byAlbum[p.Track.AlbumID] < w.PerAlbum
	}
	take := func(p ProtoPick) {
		out = append(out, p)
		if p.FromSeed {
			fromSeed++
		}
		byArtist[p.Track.ArtistID]++
		byAlbum[p.Track.AlbumID]++
	}
	afterLast := func(p ProtoPick) bool {
		return len(out) == 0 || out[len(out)-1].Track.ArtistID != p.Track.ArtistID
	}
	for _, k := range order {
		if len(out) == n {
			break
		}
		if !fits(k.p) {
			continue
		}
		if !afterLast(k.p) {
			held = append(held, k.p)
			continue
		}
		take(k.p)
		for len(out) < n {
			j := slices.IndexFunc(held, func(h ProtoPick) bool { return fits(h) && afterLast(h) })
			if j < 0 {
				break
			}
			take(held[j])
			held = slices.Delete(held, j, j+1)
		}
	}
	// Whatever is still held goes at the end rather than being lost.
	for _, h := range held {
		if len(out) < n && fits(h) {
			take(h)
		}
	}
	return out
}
