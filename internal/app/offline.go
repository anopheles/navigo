package app

import (
	"fmt"
	"slices"
	"time"

	"github.com/anopheles/navigo/internal/player"
)

// Offline: one request that can't reach the server is enough. navigo then
// browses from the cache, keeps queue changes local, and plays nothing
// until a probe (a ping) finds the server again. Probes run on a backoff,
// and right away when the user does something that needs the server.

// backoff is the wait before each retry while offline.
var backoff = []time.Duration{
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
	16 * time.Second,
	32 * time.Second,
	60 * time.Second,
}

// maxSkips is how many broken files in a row are skipped before playback
// stops, so a broken album can't spin through.
const maxSkips = 3

// ProbeReason says why the server is being checked; the result means
// something different for each.
type ProbeReason int

const (
	ProbeRetry     ProbeReason = iota // the backoff timer
	ProbeUser                         // the user wants something from the server
	ProbeReconnect                    // :reconnect
	ProbeStream                       // a stream failed to load
	ProbeStall                        // playback stalled while buffering
)

// Offline events.
type (
	// Unreachable: a request couldn't reach the server.
	Unreachable struct{ Err error }
	// StartedOffline: the server couldn't be reached to log in at startup;
	// the saved credentials are trusted until a probe checks them.
	StartedOffline struct{}
	// RetryNow: the backoff timer Seq fired.
	RetryNow struct{ Seq int }
	// Reconnect: the user asked to try the server now (:reconnect).
	Reconnect struct{}
	// BufferingStalled: playback has been buffering for 10 s.
	BufferingStalled struct{}
	// ProbeResult: the server answered (Err nil), rejected the credentials,
	// or couldn't be reached (Err set).
	ProbeResult struct {
		Why              ProbeReason
		Err              error
		WrongCredentials bool
	}
)

// Offline effects.
type (
	// ScheduleRetry asks for RetryNow{Seq} after After.
	ScheduleRetry struct {
		After time.Duration
		Seq   int
	}
	// Probe pings the server and reports a ProbeResult.
	Probe struct{ Why ProbeReason }
	// OpenLogin shows the login form: the server rejected the credentials.
	OpenLogin struct{}
)

func (s State) applyOffline(e Event) (State, []Effect) {
	switch e := e.(type) {
	case Unreachable:
		s.Sync, s.mixWait = SyncStatus{}, nil // a sync that hit it is abandoned
		return s.goOffline()
	case StartedOffline:
		return s.goOffline()
	case RetryNow:
		if !s.Offline || e.Seq != s.retrySeq {
			return s, nil // back online, or a newer retry replaced this one
		}
		return s.probe(ProbeRetry)
	case Reconnect:
		if s.PlayerCapped { // :reconnect also lets mpv start again
			s.PlayerCapped = false
			if !s.Offline {
				return s, []Effect{
					Notice{Text: "mpv can start again", Command: "play-pause", Hint: "starts it"},
				}
			}
		}
		s, out := s.probe(ProbeReconnect)
		return s, append(out, Notice{Text: "trying the server…"})
	case BufferingStalled:
		if s.Offline || s.halted || !s.Player.Playing {
			return s, nil
		}
		return s.probe(ProbeStall)
	case ProbeResult:
		return s.probed(e)
	}
	return s, nil
}

// goOffline marks the server unreachable, once, and schedules the first
// retry. A sync in progress is abandoned; the old snapshot stays.
func (s State) goOffline() (State, []Effect) {
	if s.Offline {
		return s, nil // a retry is already scheduled
	}
	s.Offline, s.retry, s.Sync, s.mixWait = true, 0, SyncStatus{}, nil
	s, out := s.scheduleRetry()
	return s, append(out, Notice{Text: "can't reach the server · browsing from the cache"})
}

func (s State) scheduleRetry() (State, []Effect) {
	s.retrySeq++
	d := backoff[min(s.retry, len(backoff)-1)]
	s.retry++
	return s, []Effect{ScheduleRetry{After: d, Seq: s.retrySeq}}
}

// probe checks the server, unless a check for the same purpose is running.
func (s State) probe(why ProbeReason) (State, []Effect) {
	if s.Probing && (why == ProbeRetry || why == ProbeUser) {
		return s, nil
	}
	s.Probing = true
	return s, []Effect{Probe{Why: why}}
}

func (s State) probed(e ProbeResult) (State, []Effect) {
	s.Probing = false
	var out, more []Effect
	if e.WrongCredentials {
		s.Offline = false
		return s, []Effect{OpenLogin{}}
	}
	if e.Err != nil {
		if e.Why == ProbeStall {
			s = s.halt()
			out = append(out, Stop{})
		}
		if s.Offline {
			s, more = s.scheduleRetry()
		} else {
			s, more = s.goOffline()
		}
		return s, append(out, more...)
	}

	// The server answered.
	wasOffline := s.Offline
	s.Offline, s.retry = false, 0
	switch {
	case wasOffline:
		out = append(out, Notice{Text: "back online"})
	case e.Why == ProbeReconnect:
		out = append(out, Notice{Text: "connected to the server"})
	}
	switch {
	case !s.loggedIn:
		// Started offline: the saved credentials are good after all.
		s.loggedIn = true
		if s.cacheChecked {
			s, more = s.startSync(false)
		}
	case wasOffline:
		s, more = s.startSync(false)
	}
	out = append(out, more...)
	if e.Why == ProbeStream {
		s, more = s.skipBroken()
		out = append(out, more...)
	}
	return s, out
}

// streamFailed stops playback when a stream fails, so mpv can't run through
// the queue, and checks whether the server or the file is to blame.
func (s State) streamFailed(title, reason string) (State, []Effect) {
	s.failedTitle, s.failedErr = title, reason
	stale := []player.EntryID{s.playing}
	if i := s.index(s.playing); i >= 0 && i+1 < len(s.Queue) {
		stale = append(stale, s.Queue[i+1].ID)
	}
	s.stale = stale
	s = s.halt()
	s, out := s.probe(ProbeStream)
	return s, append([]Effect{Stop{}}, out...)
}

// noAudio stops playback when the audio output can't be opened. Unlike a
// broken file nothing is skipped, since every track would fail the same.
func (s State) noAudio(title string) (State, []Effect) {
	s.NoAudio = true
	s = s.halt()
	return s, []Effect{
		Stop{},
		Notice{
			Text:    "couldn't play " + title + " · no audio output · check the system's sound",
			Command: "play-pause", Hint: "tries again", Err: true,
		},
	}
}

// skipBroken moves past a file the server has but can't be played.
func (s State) skipBroken() (State, []Effect) {
	s.skips++
	what := fmt.Sprintf("couldn't play %s (%s)", s.failedTitle, s.failedErr)
	switch {
	case s.skips >= maxSkips:
		return s, []Effect{
			Notice{
				Text: fmt.Sprintf(
					"%s · %d tracks in a row failed, so playback stopped",
					what,
					s.skips,
				),
				Err: true,
			},
		}
	case s.Current+1 >= len(s.Queue):
		return s, []Effect{Notice{Text: what, Err: true}}
	}
	s.Current++
	s, out := s.play(s.Current)
	return s, append(out, Notice{Text: what + " · skipped", Err: true})
}

// halt forgets the playing entry; player events are ignored until the next
// Play, as mpv may still report entries it ran on to.
func (s State) halt() State {
	s.halted = true
	s.playing = 0
	s.Player = PlayerState{Volume: s.Player.Volume}
	return s
}

// play starts queue row i, or explains that it can't while offline.
func (s State) play(i int) (State, []Effect) { return s.playFrom(i, 0) }

// playFrom plays queue row i from a position, unless something stops it:
// mpv missing, too old or crashing, or the server gone.
func (s State) playFrom(i int, start time.Duration) (State, []Effect) {
	if out := s.cantPlay(); out != nil {
		return s, out
	}
	if s.Offline {
		s, out := s.probe(ProbeUser)
		return s, append(
			out,
			Notice{
				Text:    "offline · the queue is ready",
				Command: "play-pause",
				Hint:    "plays it once the server is back",
			},
		)
	}
	s.resume = nil
	s.halted = false
	s = s.rekeyStale()
	sp := s.span(i)
	sp.Start = start
	return s, []Effect{Play{Span: sp}}
}

// rekeyStale gives queue entries mpv may still report on new IDs, so its
// late events about the old ones can't be mistaken for the new play.
func (s State) rekeyStale() State {
	if len(s.stale) == 0 ||
		!slices.ContainsFunc(
			s.Queue,
			func(q QueueEntry) bool { return slices.Contains(s.stale, q.ID) },
		) {
		return s
	}
	q := slices.Clone(s.Queue)
	for j := range q {
		if slices.Contains(s.stale, q[j].ID) {
			s.nextID++
			q[j].ID = s.nextID
		}
	}
	s.Queue = q
	return s
}

func (Unreachable) isEvent()      {}
func (StartedOffline) isEvent()   {}
func (RetryNow) isEvent()         {}
func (Reconnect) isEvent()        {}
func (BufferingStalled) isEvent() {}
func (ProbeResult) isEvent()      {}

func (ScheduleRetry) isEffect() {}
func (Probe) isEffect()         {}
func (OpenLogin) isEffect()     {}
