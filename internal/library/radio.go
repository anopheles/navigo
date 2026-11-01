package library

import (
	"math/rand/v2"
	"time"
)

// RadioBatch is how many tracks the radio adds at a time.
const RadioBatch = 10

// steerMore is how much more like an artist raises its tracks' chance
// on the radio. Less like one leaves its tracks out.
const steerMore = 2.0

// Steer is how the listener steered the radio this session: tracks it
// never adds again (played, queued, or turned down), artists to lean
// toward, and artists to leave out.
type Steer struct {
	Exclude    map[TrackID]bool
	More, Less map[ArtistID]bool
}

// RadioMix picks up to n tracks to follow after, close to a seed, as
// MixAround does but without an opening track, leaving out
// steer.Exclude and steer.Less, and leaning toward steer.More. after is the
// queue's last track, so its artist doesn't play twice in a row unless
// no one else is close; nil if none.
func (s *Snapshot) RadioMix(
	seed Seed,
	similar [][]ArtistRef,
	n int,
	now time.Time,
	r *rand.Rand,
	recommended map[TrackID]bool,
	steer Steer,
	after *Track,
) []Track {
	c := s.closeness(seed, similar)
	if len(c.seed) == 0 {
		return nil
	}
	more, less := s.artistKeys(steer.More), s.artistKeys(steer.Less)
	adjust := func(t Track) float64 {
		if steer.Exclude[t.ID] {
			return 0
		}
		f := 1.0
		for _, k := range s.trackArtists(t) {
			if less[k] {
				return 0
			}
			if more[k] {
				f = steerMore
			}
		}
		return f
	}
	order := s.mixOrder(now, r, recommended, c.of, adjust)
	rules := mixRules{fromSeed: c.fromSeed, seedCap: max(1, n/3), prev: after}
	return s.pickAfter(order, n, rules)
}

// pickAfter is pick, except that when only the artist of the track it
// follows is left, that artist plays twice in a row rather than nothing
// being picked.
func (s *Snapshot) pickAfter(order []Track, n int, rules mixRules) []Track {
	out := s.pick(order, n, rules)
	if len(out) == 0 && rules.prev != nil {
		rules.prev = nil
		out = s.pick(order, n, rules)
	}
	return out
}

func (s *Snapshot) artistKeys(ids map[ArtistID]bool) map[string]bool {
	out := make(map[string]bool, len(ids))
	for id := range ids {
		out[s.artistKey(id, "")] = true
	}
	return out
}
