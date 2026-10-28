package app

import (
	"errors"
	"math/rand/v2"
	"slices"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// The radio keeps the queue going: while it is on, a playing track
// within the last radioAhead rows of the queue has the radio add
// library.RadioBatch tracks to the end, mixed around the last radioSeed
// tracks played and the ones the listener asked more of. Its tracks are
// marked in the queue and are not undo steps. The listener steers it for
// the session: more like a track leans toward its artists and keeps it
// in the seed; less like one leaves its artists out, drops their
// upcoming radio tracks, and skips it if it plays.
const (
	radioAhead = 3
	radioSeed  = 5
	radioLiked = 5 // the "more like this" tracks kept in the seed
)

type (
	// SetRadio turns the radio on or off. Quiet sets it without a
	// message, as the config does.
	SetRadio struct {
		On, Quiet bool
	}
	// RadioTopUp answers RadioDue with the pick's random seed and the time
	// "played lately" counts back from, so Apply stays pure.
	RadioTopUp struct {
		Seq  int
		Seed uint64
		At   time.Time
	}
	// SteerRadio is more (or less) like Track on the radio, this session.
	SteerRadio struct {
		Track library.Track
		More  bool
	}
)

// RadioDue asks the UI for a RadioTopUp with Seq.
type RadioDue struct{ Seq int }

func (SetRadio) isEvent()   {}
func (RadioTopUp) isEvent() {}
func (SteerRadio) isEvent() {}
func (RadioDue) isEffect()  {}

// radioState is the radio's session: what it waits for, and how the
// listener steered it.
type radioState struct {
	seq    int        // of the RadioDue asked for and not answered yet; 0 if none
	lookup *radioWait // a top-up waiting on similar artists
	more   map[library.ArtistID]bool
	less   map[library.ArtistID]bool
	liked  []library.TrackID // more like these, the most recent first
	down   map[library.TrackID]bool
	last   int // the last Seq handed out
}

type radioWait struct {
	seq int // the similar-artist lookup's
	top RadioTopUp
}

func (s State) applyRadio(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SetRadio:
		s.Radio = e.On
		var out []Effect
		if e.On && s.Repeat == RepeatAll {
			s.Repeat = RepeatOff
			out = append(out, s.neighbours()...)
		}
		if !e.On {
			s.radio.seq, s.radio.lookup = 0, nil
		}
		if !e.Quiet {
			text := "radio off"
			if e.On {
				text = "radio on · it keeps the queue going from what plays"
			}
			out = append(out, Notice{Text: text})
		}
		s, more := s.topUp()
		return s, append(out, more...)
	case RadioTopUp:
		if e.Seq != s.radio.seq || e.Seq == 0 {
			return s, nil
		}
		s.radio.seq = 0
		if !s.Radio || s.Library == nil {
			return s, nil
		}
		seed := s.radioSeed()
		if !s.Offline {
			if artists := s.Library.SeedArtists(seed, mixLookups); len(artists) > 0 {
				s.mixSeq++
				s.radio.lookup = &radioWait{seq: s.mixSeq, top: e}
				return s, []Effect{FetchSimilar{Seq: s.mixSeq, Artists: artists}}
			}
		}
		return s.radioPick(e, nil)
	case SteerRadio:
		return s.steer(e)
	}
	return s, nil
}

// topUp asks for a RadioTopUp when the radio is on and the playing track
// is near the end of the queue, unless one is on its way.
func (s State) topUp() (State, []Effect) {
	if !s.Radio || s.Library == nil || s.radio.seq != 0 || s.radio.lookup != nil ||
		s.playing == 0 || len(s.Played) == 0 && len(s.radio.liked) == 0 {
		return s, nil
	}
	i := s.index(s.playing)
	if i < 0 || len(s.Queue)-1-i >= radioAhead {
		return s, nil
	}
	s.radio.last++
	s.radio.seq = s.radio.last
	return s, []Effect{RadioDue{Seq: s.radio.seq}}
}

// radioSeed is the tracks the radio mixes around: the ones asked more of,
// then the last few played.
func (s State) radioSeed() library.Seed {
	ids := slices.Clone(s.radio.liked)
	for _, t := range s.Played[:min(radioSeed, len(s.Played))] {
		if !slices.Contains(ids, t.ID) {
			ids = append(ids, t.ID)
		}
	}
	return library.Seed{Kind: library.SeedTracks, Tracks: ids}
}

// radioSimilar takes the similar artists a top-up waited on; ok is false
// when the answer isn't the radio's.
func (s State) radioSimilar(e SimilarLoaded) (State, []Effect, bool) {
	w := s.radio.lookup
	if w == nil || w.seq != e.Seq {
		return s, nil, false
	}
	s.radio.lookup = nil
	var out []Effect
	if e.Unreachable {
		s, out = s.applyOffline(
			Unreachable{Err: errors.New("similar artists: can't reach the server")},
		)
	}
	if !s.Radio || s.Library == nil {
		return s, out, true
	}
	s, more := s.radioPick(w.top, e.Lists)
	return s, append(more, out...), true
}

// radioPick adds a batch to the end of the queue, outside the undo
// history, and tells the player what follows.
func (s State) radioPick(e RadioTopUp, similar [][]library.ArtistRef) (State, []Effect) {
	r := rand.New(rand.NewPCG(e.Seed, e.Seed^0x9e3779b97f4a7c15))
	steer := library.Steer{
		Exclude: map[library.TrackID]bool{},
		More:    s.radio.more,
		Less:    s.radio.less,
	}
	for _, t := range s.Played {
		steer.Exclude[t.ID] = true
	}
	for _, q := range s.Queue {
		steer.Exclude[q.Track.ID] = true
	}
	for id := range s.radio.down {
		steer.Exclude[id] = true
	}
	var after *library.Track
	if len(s.Queue) > 0 {
		after = &s.Queue[len(s.Queue)-1].Track
	}
	picked := s.Library.RadioMix(
		s.radioSeed(), similar, library.RadioBatch, e.At, r, s.recs.matched, steer, after,
	)
	if len(picked) == 0 {
		return s, []Effect{Notice{Text: "the radio has nothing close left to add"}}
	}
	var added []QueueEntry
	added, s.nextID = entries(s.nextID, picked)
	for i := range added {
		added[i].Radio = true
	}
	s.Queue = append(slices.Clone(s.Queue), added...)
	return s, append(
		s.neighbours(),
		Notice{Text: "the radio added " + count(len(added)), Quiet: true},
	)
}

// steer is more or less like a track for the rest of the session.
func (s State) steer(e SteerRadio) (State, []Effect) {
	if !s.Radio {
		return s, []Effect{Notice{Text: "the radio is off · :radio turns it on", Err: true}}
	}
	artists := trackArtists(e.Track)
	name := e.Track.Artist
	if name == "" {
		name = e.Track.Title
	}
	s.radio.more, s.radio.less = cloneSet(s.radio.more), cloneSet(s.radio.less)
	if e.More {
		for _, a := range artists {
			s.radio.more[a] = true
			delete(s.radio.less, a)
		}
		s.radio.liked = slices.Insert(
			slices.DeleteFunc(
				slices.Clone(s.radio.liked),
				func(id library.TrackID) bool { return id == e.Track.ID },
			),
			0,
			e.Track.ID,
		)
		s.radio.liked = s.radio.liked[:min(len(s.radio.liked), radioLiked)]
		return s, []Effect{Notice{Text: "more like " + name + " on the radio"}}
	}
	for _, a := range artists {
		s.radio.less[a] = true
		delete(s.radio.more, a)
	}
	s.radio.liked = slices.DeleteFunc(
		slices.Clone(s.radio.liked),
		func(id library.TrackID) bool { return id == e.Track.ID },
	)
	s.radio.down = cloneSet(s.radio.down)
	s.radio.down[e.Track.ID] = true

	// The radio's upcoming tracks by those artists go, outside the undo
	// history like the radio's own additions.
	cur := s.index(s.playing)
	kept := make([]QueueEntry, 0, len(s.Queue))
	for i, q := range s.Queue {
		turnedDown := slices.ContainsFunc(
			trackArtists(q.Track),
			func(a library.ArtistID) bool { return s.radio.less[a] },
		)
		if q.Radio && i > cur && turnedDown {
			continue
		}
		kept = append(kept, q)
	}
	s.Queue = kept
	if s.playing != 0 {
		s.Current = s.index(s.playing)
	}
	out := []Effect{Notice{Text: "no more " + name + " on the radio"}}
	if _, row, playing := s.NowPlaying(); playing && row >= 0 &&
		s.Queue[row].Track.ID == e.Track.ID {
		var skip []Effect
		s, skip = apply(s, NextTrack{})
		out = append(skip, out...)
	} else {
		out = append(s.neighbours(), out...)
	}
	s, more := s.topUp()
	return s, append(out, more...)
}

// trackArtists are the artists a track credits.
func trackArtists(t library.Track) []library.ArtistID {
	if len(t.Artists) == 0 {
		return []library.ArtistID{t.ArtistID}
	}
	out := make([]library.ArtistID, 0, len(t.Artists))
	for _, a := range t.Artists {
		out = append(out, a.ID)
	}
	return out
}

func cloneSet[K comparable](m map[K]bool) map[K]bool {
	out := make(map[K]bool, len(m)+1)
	for k, v := range m {
		out[k] = v
	}
	return out
}

// repeatEndsRadio turns the radio off when repeat all came on: the queue
// never runs out then.
func (s State) repeatEndsRadio(out []Effect) (State, []Effect) {
	if s.Radio && s.Repeat == RepeatAll {
		s.Radio = false
		s.radio.seq, s.radio.lookup = 0, nil
		out = append(out, Notice{Text: "repeat all · radio off"})
	}
	return s, out
}
