package library

import (
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// SeedKind is what a Mix is built around.
type SeedKind int

const (
	SeedArtist SeedKind = iota + 1
	SeedAlbum
	SeedPlaylist
	SeedTrack
)

// Seed is the artist, album, playlist or track a Mix is built around.
// Name is what its row shows, for messages.
type Seed struct {
	Kind SeedKind
	ID   string
	Name string
}

// MixAround picks up to n tracks close to a seed, from the cache. A
// track's chance is its closeness² times its Mix weight. Tracks from the
// seed fill at most a third of the Mix, skip the limit per artist (and
// per album for an album seed), and one of them opens it: the seed
// itself for a track. It returns fewer than n, or none, when too few
// tracks are close; it never pads with unrelated ones.
func (s *Snapshot) MixAround(seed Seed, n int, now time.Time, r *rand.Rand) []Track {
	c := s.closeness(seed)
	if len(c.seed) == 0 {
		return nil
	}
	order := s.mixOrder(now, r, c.of)
	rules := mixRules{
		fromSeed:   c.fromSeed,
		seedCap:    max(1, n/3),
		seedAlbums: seed.Kind == SeedAlbum,
	}
	if seed.Kind == SeedTrack {
		rules.opener = &c.seed[0] // asked for, so it plays whatever its weight
	} else if i := slices.IndexFunc(order, func(t Track) bool { return c.in[t.ID] }); i >= 0 {
		rules.opener = &order[i] // a weighted pick from the seed's own tracks
	}
	return s.pick(order, n, rules)
}

// closeness scores how close a track is to a seed.
type closeness struct {
	s       *Snapshot
	seed    []Track // the seed's own tracks, each once
	in      map[TrackID]bool
	artists map[string]float64 // each artist's share of the seed, by artistKey
	genres  map[string]float64 // each genre's share of the seed, in lower case
}

func (s *Snapshot) closeness(seed Seed) closeness {
	c := closeness{s: s, in: map[TrackID]bool{}, artists: map[string]float64{}, genres: map[string]float64{}}
	for _, t := range s.seedTracks(seed) {
		if !c.in[t.ID] {
			c.in[t.ID] = true
			c.seed = append(c.seed, t)
		}
	}
	for _, t := range c.seed {
		for _, k := range s.trackArtists(t) {
			c.artists[k]++
		}
		if g := strings.ToLower(t.Genre); g != "" {
			c.genres[g]++
		}
	}
	// Counted first and divided once, so a whole seed's share is exactly 1.
	for _, m := range []map[string]float64{c.artists, c.genres} {
		for k := range m {
			m[k] /= float64(len(c.seed))
		}
	}
	return c
}

// of is a track's closeness to the seed: 3 for a track in it, plus
// 2 × √share for the seed artist with the best share, plus the share of
// the seed with its genre. 0 means it isn't a candidate.
func (c closeness) of(t Track) float64 {
	var v float64
	if c.in[t.ID] {
		v += 3
	}
	best := 0.0
	for _, k := range c.s.trackArtists(t) {
		best = max(best, c.artists[k])
	}
	if best > 0 {
		v += 2 * math.Sqrt(best)
	}
	return v + c.genres[strings.ToLower(t.Genre)]
}

// fromSeed reports whether a track is from the seed: in it, or by an
// artist who makes up at least half of it.
func (c closeness) fromSeed(t Track) bool {
	if c.in[t.ID] {
		return true
	}
	for _, k := range c.s.trackArtists(t) {
		if c.artists[k] >= 0.5 {
			return true
		}
	}
	return false
}

// trackArtists are the artists a track credits, by artistKey, each once.
func (s *Snapshot) trackArtists(t Track) []string {
	if len(t.Artists) == 0 {
		return []string{s.artistKey(t.ArtistID, "")}
	}
	out := make([]string, 0, len(t.Artists))
	for _, a := range t.Artists {
		if k := s.artistKey(a.ID, a.Name); !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// seedTracks are the tracks a seed is made of; none when the library
// doesn't have it. An artist's are every track crediting them, under any
// spelling of their name that differs only by case.
func (s *Snapshot) seedTracks(seed Seed) []Track {
	switch seed.Kind {
	case SeedTrack:
		if t, ok := s.Track(TrackID(seed.ID)); ok {
			return []Track{t}
		}
	case SeedAlbum:
		return s.AlbumTracks(AlbumID(seed.ID))
	case SeedPlaylist:
		p, ok := s.Playlist(PlaylistID(seed.ID))
		if !ok {
			return nil
		}
		var out []Track
		for _, id := range p.Tracks {
			if t, ok := s.Track(id); ok {
				out = append(out, t)
			}
		}
		return out
	case SeedArtist:
		if _, ok := s.Artist(ArtistID(seed.ID)); !ok {
			return nil
		}
		k := s.artistKey(ArtistID(seed.ID), "")
		var out []Track
		for _, t := range s.Tracks {
			if slices.Contains(s.trackArtists(t), k) {
				out = append(out, t)
			}
		}
		return out
	}
	return nil
}
