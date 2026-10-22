package app

import (
	"fmt"
	"log/slog"
	"strconv"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/listenbrainz"
)

// Recommendations (ADR 0008): when listenbrainz.user is set, navigo reads
// the user's Recommendations from ListenBrainz at the startup sync check
// (when the stored list is a day old) and on :sync, keeps the last list in
// its own file, and matches it to the library whenever either changes.
// ListenBrainz failing never makes navigo offline.

// Events for Recommendations.
type (
	// ConfigureListenBrainz sets the ListenBrainz user and API on
	// :reload-config; an empty User turns Recommendations off.
	ConfigureListenBrainz struct{ User, API string }
	// RecsLoaded is the stored list, or nil when there is none.
	RecsLoaded struct{ Stored *listenbrainz.Stored }
	// RecsFetched is how a FetchRecs for User and API went.
	RecsFetched struct {
		User, API string
		Outcome   FetchOutcome
		Stored    listenbrainz.Stored // for FetchNew and FetchUnchanged
		Err       error               // for FetchFailed, for the log
	}
)

// FetchOutcome is how a fetch of the Recommendations ended.
type FetchOutcome int

const (
	FetchNew          FetchOutcome = iota // a new list
	FetchUnchanged                        // the stored list is still current
	FetchNotDue                           // the stored list is less than a day old
	FetchNoUser                           // 404: no such ListenBrainz user
	FetchNotGenerated                     // 204: none made for the user yet
	FetchFailed                           // unreachable, 5xx, or a reply that can't be read
)

// Effects for Recommendations.
type (
	// LoadRecs reads the stored list, answered with RecsLoaded.
	LoadRecs struct{}
	// FetchRecs asks ListenBrainz for the list, answered with RecsFetched.
	// Unless Force is set, a list in Old fetched less than a day ago is
	// kept without asking.
	FetchRecs struct {
		User, API string
		Old       *listenbrainz.Stored
		Force     bool
	}
	// SaveRecs writes the stored list.
	SaveRecs struct{ Stored listenbrainz.Stored }
	// DeleteRecs removes the stored list.
	DeleteRecs struct{}
)

func (ConfigureListenBrainz) isEvent() {}
func (RecsLoaded) isEvent()            {}
func (RecsFetched) isEvent()           {}
func (LoadRecs) isEffect()             {}
func (FetchRecs) isEffect()            {}
func (SaveRecs) isEffect()             {}
func (DeleteRecs) isEffect()           {}

// recsState is what the core keeps of Recommendations.
type recsState struct {
	user, api string // user is empty when Recommendations are off
	stored    *listenbrainz.Stored
	loaded    bool // the stored list has been read
	fetching  bool
	// want is a fetch asked for before the stored list was read:
	// wantNone, wantDue or wantForced.
	want int
	// matched are the library tracks the list names, and matchedRecs how
	// many Recommendations named at least one.
	matched     map[library.TrackID]bool
	matchedRecs int
}

const (
	wantNone = iota
	wantDue
	wantForced
)

// WithListenBrainz is the state with Recommendations on for user at api,
// or off when user is empty; for the state navigo starts with.
func (s State) WithListenBrainz(user, api string) State {
	s.recs = recsState{user: user, api: api}
	return s
}

// Recommended reports whether a track is one of the user's Recommendations.
func (s State) Recommended(id library.TrackID) bool { return s.recs.matched[id] }

func (s State) recsOn() bool { return s.recs.user != "" }

func (s State) applyRecs(e Event) (State, []Effect) {
	switch e := e.(type) {
	case ConfigureListenBrainz:
		if e.User == s.recs.user && e.API == s.recs.api {
			return s, nil
		}
		wasOn := s.recsOn()
		s.recs = recsState{user: e.User, api: e.API, loaded: true}
		if !s.recsOn() {
			if wasOn {
				return s, []Effect{DeleteRecs{}}
			}
			return s, nil
		}
		return s.fetchRecs(true)
	case RecsLoaded:
		s.recs.loaded = true
		if st := e.Stored; st != nil && st.User == s.recs.user && st.URL == s.recs.api {
			s.recs.stored = st
		}
		want := s.recs.want
		s.recs.want = wantNone
		if want != wantNone {
			return s.fetchRecs(want == wantForced)
		}
		return s, nil
	case RecsFetched:
		if e.User != s.recs.user || e.API != s.recs.api {
			return s, nil // asked for before the config changed
		}
		s.recs.fetching = false
		return s.recsFetched(e)
	}
	return s, nil
}

// fetchRecs asks ListenBrainz for the list: always when forced, else only
// when the stored one is a day old. Before the stored list is read, the
// fetch waits for it.
func (s State) fetchRecs(force bool) (State, []Effect) {
	switch {
	case !s.recsOn() || s.recs.fetching:
		return s, nil
	case !s.recs.loaded:
		want := wantDue
		if force {
			want = wantForced
		}
		s.recs.want = max(s.recs.want, want)
		return s, nil
	}
	s.recs.fetching = true
	return s, []Effect{
		FetchRecs{User: s.recs.user, API: s.recs.api, Old: s.recs.stored, Force: force},
	}
}

func (s State) recsFetched(e RecsFetched) (State, []Effect) {
	switch e.Outcome {
	case FetchNew:
		st := e.Stored
		s.recs.stored = &st
		s = s.matchRecs()
		text := fmt.Sprintf("%s Recommendations from ListenBrainz", thousands(len(st.Recs)))
		if s.Library != nil {
			text = fmt.Sprintf(
				"%s of %s Recommendations are in the library",
				thousands(s.recs.matchedRecs), thousands(len(st.Recs)),
			)
		}
		return s, []Effect{SaveRecs{Stored: st}, Notice{Text: text, Quiet: true}}
	case FetchUnchanged:
		st := e.Stored
		s.recs.stored = &st
		return s, []Effect{SaveRecs{Stored: st}}
	case FetchNoUser:
		s.recs.stored = nil
		return s, []Effect{DeleteRecs{}, Notice{
			Text: fmt.Sprintf(
				"ListenBrainz has no user %q · check listenbrainz.user in config.yaml", s.recs.user,
			),
			Err: true,
		}}
	case FetchNotGenerated:
		s.recs.stored = nil
		return s, []Effect{DeleteRecs{}, Notice{Text: fmt.Sprintf(
			"no Recommendations from ListenBrainz for %s yet · they can take a day or two after your first listens",
			s.recs.user,
		)}}
	case FetchFailed:
		if e.Err != nil {
			slog.Warn("ListenBrainz", "err", e.Err)
		}
		text := "couldn't reach ListenBrainz · no Recommendations yet"
		if st := s.recs.stored; st != nil {
			text = "couldn't reach ListenBrainz · keeping the Recommendations from " +
				st.LastUpdated.Local().Format("2 Jan")
		}
		return s, []Effect{Notice{Text: text, Quiet: true}}
	}
	return s, nil
}

// withRecs matches the list to the library again when either changed.
func withRecs(old, n State) State {
	if n.Library == old.Library && n.recs.stored == old.recs.stored {
		return n
	}
	return n.matchRecs()
}

func (s State) matchRecs() State {
	if s.Library == nil || s.recs.stored == nil {
		s.recs.matched, s.recs.matchedRecs = nil, 0
		return s
	}
	s.recs.matched, s.recs.matchedRecs = s.Library.Recommended(s.recs.stored.Recs)
	return s
}

// thousands writes n with commas, as in 1,000.
func thousands(n int) string {
	d := strconv.Itoa(n)
	for i := len(d) - 3; i > 0; i -= 3 {
		d = d[:i] + "," + d[i:]
	}
	return d
}
