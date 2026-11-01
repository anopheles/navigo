package library

import (
	"math/rand/v2"
	"slices"
	"time"
)

// MixAddSize is how many tracks a mix-add adds unless a count says otherwise.
const MixAddSize = 10

// MixAdd picks up to n tracks to add to a queue: around seed as MixAround
// does, or from the whole library when seed is nil, but without an opening
// track and leaving out exclude. after and before are the tracks the batch
// goes between, nil where there is none. An artist doesn't play twice in a
// row after after unless no one else is left. before gives way: the batch
// ends on another artist if a track can move there without breaking the
// batch's own variety, and is never shortened for it.
func (s *Snapshot) MixAdd(
	seed *Seed,
	similar [][]ArtistRef,
	n int,
	now time.Time,
	r *rand.Rand,
	recommended map[TrackID]bool,
	exclude map[TrackID]bool,
	after, before *Track,
) []Track {
	var out []Track
	if seed != nil {
		steer := Steer{Exclude: exclude}
		out = s.RadioMix(*seed, similar, n, now, r, recommended, steer, after)
	} else {
		adjust := func(t Track) float64 {
			if exclude[t.ID] {
				return 0
			}
			return 1
		}
		order := s.mixOrder(now, r, recommended, nil, adjust)
		out = s.pickAfter(order, n, mixRules{prev: after})
	}
	return s.endBefore(out, after, before)
}

// endBefore moves a track to the end of out when out's last track is by
// before's artist, if one can move without putting an artist twice in a row.
func (s *Snapshot) endBefore(out []Track, after, before *Track) []Track {
	if before == nil || len(out) == 0 {
		return out
	}
	next := s.mixArtist(*before)
	last := s.mixArtist(out[len(out)-1])
	if last != next {
		return out
	}
	for j := len(out) - 2; j >= 0; j-- {
		a := s.mixArtist(out[j])
		if a == next {
			continue
		}
		prev := after
		if j > 0 {
			prev = &out[j-1]
		}
		if prev != nil && s.mixArtist(*prev) == s.mixArtist(out[j+1]) {
			continue
		}
		t := out[j]
		return append(slices.Delete(out, j, j+1), t)
	}
	return out
}
