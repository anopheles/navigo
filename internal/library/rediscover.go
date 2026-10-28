package library

import (
	"math/rand/v2"
	"time"
)

// forgottenAfter is how long a favourite goes unplayed before it counts
// as forgotten.
const forgottenAfter = 180 * 24 * time.Hour

// neverPlayedPerArtist caps one album artist's share of Never played.
const neverPlayedPerArtist = 2

// ForgottenFavourites picks up to n tracks that are starred or rated 4★
// or more and haven't played in six months (or ever), as a Mix picks:
// starred and 5★ ones more often, with a Mix's variety rules.
func (s *Snapshot) ForgottenFavourites(n int, now time.Time, r *rand.Rand) []Track {
	favourite := func(t Track) float64 {
		liked := s.IsStarred(string(t.ID)) || t.Rating >= 4
		forgotten := t.Played.IsZero() || now.Sub(t.Played) > forgottenAfter
		if liked && forgotten {
			return 1
		}
		return 0
	}
	return s.pick(s.mixOrder(now, r, nil, nil, favourite), n, mixRules{})
}

// NeverPlayed picks up to n albums none of whose tracks has played, at
// random, at most neverPlayedPerArtist of one album artist.
func (s *Snapshot) NeverPlayed(n int, r *rand.Rand) []Album {
	var cands []Album
	for _, a := range s.Albums {
		if s.albumUnplayed(a.ID) {
			cands = append(cands, a)
		}
	}
	r.Shuffle(len(cands), func(i, j int) { cands[i], cands[j] = cands[j], cands[i] })
	byArtist := map[string]int{}
	var out []Album
	for _, a := range cands {
		if len(out) == n {
			break
		}
		k := s.artistKey(a.ArtistID, a.Artist)
		if byArtist[k] >= neverPlayedPerArtist {
			continue
		}
		byArtist[k]++
		out = append(out, a)
	}
	return out
}

// albumUnplayed reports whether an album has tracks and none has played.
func (s *Snapshot) albumUnplayed(id AlbumID) bool {
	ts := s.AlbumTracks(id)
	for _, t := range ts {
		if t.PlayCount > 0 || !t.Played.IsZero() {
			return false
		}
	}
	return len(ts) > 0
}
