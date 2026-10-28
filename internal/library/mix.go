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

// recommendedWeight multiplies the Mix weight of the user's
// Recommendations; ListenBrainz's score doesn't count.
const recommendedWeight = 3

// Mix picks up to n tracks at random from the whole library, leaning
// toward starred and well-rated tracks that haven't played lately, and
// toward the user's Recommendations (recommended may be nil). It
// takes at most mixPerArtist tracks of an artist and mixPerAlbum of an
// album, never puts an artist twice in a row, and leaves out tracks
// rated one star. Artists whose names differ only by case count as one.
// It returns fewer than n when the rules run out of tracks.
func (s *Snapshot) Mix(n int, now time.Time, r *rand.Rand, recommended map[TrackID]bool) []Track {
	return s.pick(s.mixOrder(now, r, recommended, nil, nil), n, mixRules{})
}

// mixOrder is the library's tracks in a weighted random order
// (Efraimidis–Spirakis): each track's key is -ln(u)/weight, and the
// smallest keys come first. A track's weight is its Mix weight, times
// closeness² when closeness is given, times adjust when that is given;
// tracks of weight 0 are left out.
func (s *Snapshot) mixOrder(
	now time.Time,
	r *rand.Rand,
	recommended map[TrackID]bool,
	close func(Track) float64,
	adjust func(Track) float64,
) []Track {
	type keyed struct {
		key float64
		i   int
	}
	order := make([]keyed, 0, len(s.Tracks))
	for i, t := range s.Tracks {
		w := s.mixWeight(t, now)
		if recommended[t.ID] {
			w *= recommendedWeight
		}
		if close != nil && w > 0 {
			c := close(t)
			w *= c * c
		}
		if adjust != nil && w > 0 {
			w *= adjust(t)
		}
		if w > 0 {
			order = append(order, keyed{-math.Log(1-r.Float64()) / w, i})
		}
	}
	slices.SortFunc(order, func(a, b keyed) int { return cmp.Compare(a.key, b.key) })
	out := make([]Track, len(order))
	for j, k := range order {
		out[j] = s.Tracks[k.i]
	}
	return out
}

// mixRules are what a Mix around a seed adds to the variety rules.
type mixRules struct {
	opener *Track // plays first, whatever the rules say
	// fromSeed tracks are at most seedCap of the Mix and skip the limit
	// per artist, and per album too when seedAlbums is set.
	fromSeed   func(Track) bool
	seedCap    int
	seedAlbums bool
	// prev is the track the Mix follows, for never one artist twice in a
	// row across it.
	prev *Track
}

// pick takes up to n tracks from order, in order, keeping the variety
// rules: at most mixPerArtist tracks of an artist and mixPerAlbum of an
// album, and never an artist twice in a row.
func (s *Snapshot) pick(order []Track, n int, rules mixRules) []Track {
	var (
		out      []Track
		held     []Track // the artist just played; tried again after the next pick
		byArtist = map[string]int{}
		byAlbum  = map[AlbumID]int{}
		seedN    int
	)
	fromSeed := func(t Track) bool { return rules.fromSeed != nil && rules.fromSeed(t) }
	fits := func(t Track) bool {
		if !fromSeed(t) {
			return byArtist[s.mixArtist(t)] < mixPerArtist && byAlbum[t.AlbumID] < mixPerAlbum
		}
		return seedN < rules.seedCap && (rules.seedAlbums || byAlbum[t.AlbumID] < mixPerAlbum)
	}
	take := func(t Track) {
		out = append(out, t)
		if fromSeed(t) {
			seedN++
		} else {
			byArtist[s.mixArtist(t)]++
		}
		byAlbum[t.AlbumID]++
	}
	afterLast := func(t Track) bool {
		switch {
		case len(out) > 0:
			return s.mixArtist(out[len(out)-1]) != s.mixArtist(t)
		case rules.prev != nil:
			return s.mixArtist(*rules.prev) != s.mixArtist(t)
		}
		return true
	}
	if rules.opener != nil && n > 0 {
		take(*rules.opener)
	}
	for _, t := range order {
		if len(out) == n {
			break
		}
		if rules.opener != nil && t.ID == rules.opener.ID || !fits(t) {
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

// mixArtist is who a track is by for a Mix's variety rules: see artistKey.
func (s *Snapshot) mixArtist(t Track) string { return s.artistKey(t.ArtistID, "") }

// artistKey is one artist for a Mix: their name in lower case, so
// "ELETTRA STORM" and "Elettra Storm" are one artist, or their ID when
// no name is known. name, if given, is the name a track credits.
func (s *Snapshot) artistKey(id ArtistID, name string) string {
	if name == "" {
		if a, ok := s.Artist(id); ok {
			name = a.Name
		}
	}
	if name == "" {
		return "id:" + string(id)
	}
	return strings.ToLower(name)
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
