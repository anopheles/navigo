package library

import (
	"math"
	"math/rand/v2"
	"time"
)

// PROTOTYPE (prototype/listenbrainz-lift, never merged): a whole-library
// Mix whose weights are multiplied by lift(t), with the usual variety rules.
func (s *Snapshot) ProtoMixLifted(
	n int,
	now time.Time,
	r *rand.Rand,
	lift func(Track) float64,
) []Track {
	// mixOrder multiplies by close², so pass √lift.
	return s.pick(
		s.mixOrder(now, r, func(t Track) float64 { return math.Sqrt(lift(t)) }),
		n,
		mixRules{},
	)
}
