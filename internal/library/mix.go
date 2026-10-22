package library

import (
	"cmp"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"time"
)

// MixSize is how many tracks a Mix picks when no count is given.
const MixSize = 50

// A Mix's variety rules: how many of its tracks one artist or one album
// may have.
const (
	mixPerArtist = 3
	mixPerAlbum  = 2
)

// Mix picks up to n tracks at random from the whole library, leaning
// toward starred and well-rated tracks that haven't played lately. It
// takes at most mixPerArtist tracks of an artist and mixPerAlbum of an
// album, never puts an artist twice in a row, and leaves out tracks
// rated one star. Artists whose names differ only by case count as one.
// It returns fewer than n when the rules run out of tracks.
func (s *Snapshot) Mix(n int, now time.Time, r *rand.Rand) []Track {
	// A weighted random order (Efraimidis–Spirakis): each track's key is
	// -ln(u)/weight, and the smallest keys come first.
	type keyed struct {
		key float64
		i   int
	}
	order := make([]keyed, 0, len(s.Tracks))
	for i, t := range s.Tracks {
		if w := s.mixWeight(t, now); w > 0 {
			order = append(order, keyed{-math.Log(1-r.Float64()) / w, i})
		}
	}
	slices.SortFunc(order, func(a, b keyed) int { return cmp.Compare(a.key, b.key) })

	var (
		out      []Track
		held     []Track // the artist just played; tried again after the next pick
		byArtist = map[string]int{}
		byAlbum  = map[AlbumID]int{}
	)
	fits := func(t Track) bool {
		return byArtist[s.mixArtist(t)] < mixPerArtist && byAlbum[t.AlbumID] < mixPerAlbum
	}
	take := func(t Track) {
		out = append(out, t)
		byArtist[s.mixArtist(t)]++
		byAlbum[t.AlbumID]++
	}
	afterLast := func(t Track) bool { return len(out) == 0 || s.mixArtist(out[len(out)-1]) != s.mixArtist(t) }
	for _, k := range order {
		if len(out) == n {
			break
		}
		t := s.Tracks[k.i]
		if !fits(t) {
			continue
		}
		if !afterLast(t) {
			held = append(held, t)
			continue
		}
		take(t)
		// Held tracks can follow now, in their order, while they fit.
		for len(out) < n {
			j := slices.IndexFunc(held, func(h Track) bool { return fits(h) && afterLast(h) })
			if j < 0 {
				break
			}
			take(held[j])
			held = slices.Delete(held, j, j+1)
		}
	}
	return out
}

// mixArtist is who a track is by for a Mix's variety rules: its
// artist's name in lower case, so "ELETTRA STORM" and "Elettra Storm"
// are one artist, or the artist's ID when the name isn't known.
func (s *Snapshot) mixArtist(t Track) string {
	if a, ok := s.Artist(t.ArtistID); ok && a.Name != "" {
		return strings.ToLower(a.Name)
	}
	return "id:" + string(t.ArtistID)
}

// mixWeight is how likely a Mix is to pick a track, relative to an
// unstarred, unrated track not played in 90 days; 0 leaves it out.
func (s *Snapshot) mixWeight(t Track, now time.Time) float64 {
	w := 1.0
	if s.IsStarred(string(t.ID)) {
		w *= 3
	}
	switch t.Rating {
	case 1:
		return 0
	case 2:
		w *= 0.5
	case 4:
		w *= 1.5
	case 5:
		w *= 2
	}
	return w * staleness(t.Played, now)
}

// staleness is 0.1 for a track played in the last week, rising in a
// straight line to 1 at 90 days; a track never played is 1.
func staleness(played, now time.Time) float64 {
	const week, fresh = 7 * 24 * time.Hour, 90 * 24 * time.Hour
	since := now.Sub(played)
	switch {
	case played.IsZero() || since >= fresh:
		return 1
	case since <= week:
		return 0.1
	}
	return 0.1 + 0.9*float64(since-week)/float64(fresh-week)
}
