package app

import (
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/resume"
)

// Events and effects for Resume: the saved queue is read at startup and
// written as the queue changes.
type (
	// SavedLoaded is the saved queue to bring back, from the server or this
	// device's file; Found is false when there was none. Settings is true
	// when Saved's Volume and Repeat were read from this device's file,
	// and false when only the queue is known, so the volume and repeat
	// stay as they are.
	// Unsynced is true when the server's queue was asked for and could not
	// be read, so this session does not sync it.
	SavedLoaded struct {
		Saved    resume.Saved
		Found    bool
		Settings bool
		Unsynced bool
	}
	// SavedGraceElapsed: the login had not answered savedGrace after the
	// start, so the queue is restored from this device's file without it.
	SavedGraceElapsed struct{}
	// LoginFailed: the login failed in a way that is neither an
	// unreachable server nor rejected credentials.
	LoginFailed struct{}
	// ConfigureResume turns Resume on or off, at startup and on
	// :reload-config.
	ConfigureResume struct{ On, Sync bool }
	// LoggedOut forgets the saved queue and stops saving it.
	LoggedOut struct{}
)

type (
	// LoadSaved reads the saved queue, answered with SavedLoaded. With
	// Server set the server's queue is asked for first and wins when it
	// has one; without, only this device's file is read.
	LoadSaved struct{ Server bool }
	// SaveQueue writes the saved queue to this device's file and to the
	// server; an empty one removes the file and clears the server's queue.
	// KeepServer says the server's queue stays as it is, because this one
	// was cut down by a restore that dropped tracks the cache lacked.
	// Exact says the position is where playback stands now, not rounded,
	// so the server gets it even when little has changed.
	SaveQueue struct {
		Saved      resume.Saved
		KeepServer bool
		Exact      bool
	}
	// DeleteSaved removes this device's saved queue, and any write still
	// waiting. The server's queue stays for the other devices.
	DeleteSaved struct{}
	// ScheduleSavedGrace asks for SavedGraceElapsed after After.
	ScheduleSavedGrace struct{ After time.Duration }
)

func (SavedLoaded) isEvent()         {}
func (ConfigureResume) isEvent()     {}
func (LoggedOut) isEvent()           {}
func (SavedGraceElapsed) isEvent()   {}
func (LoginFailed) isEvent()         {}
func (LoadSaved) isEffect()          {}
func (SaveQueue) isEffect()          {}
func (DeleteSaved) isEffect()        {}
func (ScheduleSavedGrace) isEffect() {}

// savedGrace is how long the start waits for the login before it restores
// from this device's file, when the queue would sync.
const savedGrace = 3 * time.Second

// unsyncedNote is what the user is told when the queue won't sync.
const unsyncedNote = "queue not synced this session · couldn't get the server's queue"

// positionStep is how coarsely the position of a playing track is saved,
// so playing doesn't rewrite the file every second.
const positionStep = 10 * time.Second

// saved is what Resume keeps of this state. The position is the playing
// one (rounded down, but exact once paused), or where a restored or
// interrupted track will start from.
func (s State) saved() resume.Saved { return s.savedAt(positionStep) }

// KeepsServerQueue says a write now would cut the server's queue down: the
// queue is still the one a restore left after dropping tracks.
func (s State) KeepsServerQueue() bool {
	return s.heldBack != nil && slices.Equal(s.saved().Tracks, s.heldBack)
}

// SaveNow is the saved queue with the exact position, for when navigo
// quits. ok is false when nothing is being saved.
func (s State) SaveNow() (v resume.Saved, ok bool) {
	return s.savedAt(time.Nanosecond), s.ResumeOn && s.saving
}

// savedAt is saved with the playing position rounded down to step.
func (s State) savedAt(step time.Duration) resume.Saved {
	v := resume.Saved{
		Current: -1,
		Volume:  s.Player.Volume,
		Repeat:  int(s.Repeat),
	}
	for _, q := range s.Queue {
		v.Tracks = append(v.Tracks, q.Track.ID)
	}
	if s.Current >= 0 && s.Current < len(s.Queue) {
		v.Current = s.Current
	}
	switch {
	case s.Player.Playing && s.Player.Paused:
		v.Position = s.Player.Position
	case s.Player.Playing:
		v.Position = s.Player.Position.Truncate(step)
	case s.resume != nil:
		v.Position = s.resume.pos
	}
	if v.Current < 0 {
		v.Position = 0
	}
	return v
}

// applyResume handles the events of Resume.
func (s State) applyResume(e Event) (State, []Effect) {
	switch e := e.(type) {
	case SavedGraceElapsed, LoginFailed:
		return s.loadSaved(false)
	case SavedLoaded:
		s.savedLoaded = true
		if e.Found {
			s.pending = &e
		} else {
			s.restored, s.saving = true, true
		}
		if e.Unsynced {
			s.queueUnsynced = true
			return s, []Effect{Notice{Text: unsyncedNote}}
		}
	case ConfigureResume:
		was := s.ResumeOn
		s.ResumeOn, s.SyncOn = e.On, e.Sync
		switch {
		case !e.On:
			s.saving = false
			return s, []Effect{DeleteSaved{}}
		case !was:
			// Turned on mid-run: there is nothing to restore, and the
			// queue is saved from now on.
			s.restored, s.saving = true, true
		}
	case LoggedOut:
		s.saving = false
		return s, []Effect{DeleteSaved{}}
	}
	return s, nil
}

// restoreSaved brings the saved queue back once both it and the library are
// there: loaded paused, never playing.
func (s State) restoreSaved() (State, []Effect) {
	if !s.ResumeOn || s.restored || s.pending == nil || s.Library == nil {
		return s, nil
	}
	loaded := *s.pending
	p := loaded.Saved
	s.pending, s.restored, s.saving = nil, true, true
	if len(s.Queue) > 0 { // the user got there first
		return s, nil
	}
	var q []QueueEntry
	cur := -1
	for i, id := range p.Tracks {
		t, ok := s.Library.Track(id)
		if !ok {
			continue
		}
		if i == p.Current {
			cur = len(q)
		}
		s.nextID++
		q = append(q, QueueEntry{ID: s.nextID, Track: t})
	}
	if len(q) == 0 {
		if len(p.Tracks) > 0 {
			s.heldBack = []library.TrackID{}
		}
		return s, nil
	}
	pos := p.Position
	if cur < 0 && p.Current >= 0 && p.Current < len(p.Tracks) {
		// The current track is gone: start from the one that followed it.
		for i := p.Current; i < len(p.Tracks) && cur < 0; i++ {
			for j, e := range q {
				if e.Track.ID == p.Tracks[i] {
					cur = j
					break
				}
			}
		}
		pos = 0
	}
	s.Queue, s.undo, s.redo, s.resume = q, nil, nil, nil
	s.Current = -1
	hint := "plays it"
	if cur >= 0 {
		s.Current = cur
		s.resume = &resumePoint{entry: q[cur].ID, pos: pos}
		if pos >= time.Second {
			hint = "plays from " + mmss(pos)
		}
	}
	if loaded.Settings {
		s.Player.Volume = min(max(p.Volume, 0), 100)
		if r := Repeat(p.Repeat); r >= RepeatOff && r <= RepeatOne {
			s.Repeat = r
		}
	}
	text := fmt.Sprintf("restored %d %s", len(q), plural(len(q), "track"))
	if len(q) < len(p.Tracks) {
		text = fmt.Sprintf("restored %d of %d tracks", len(q), len(p.Tracks))
		s.heldBack = s.saved().Tracks
	}
	if s.queueUnsynced {
		text += " (not synced this session)"
	}
	out := []Effect{Notice{Text: text, Command: "play-pause", Hint: hint}}
	if loaded.Settings {
		out = append([]Effect{SetVolume{Percent: s.Player.Volume}}, out...)
	}
	return s, out
}

// loadSaved asks for the saved queue, once per session: when the login has
// answered, with server set if it was reached, so the server's queue can
// be asked for.
func (s State) loadSaved(server bool) (State, []Effect) {
	if !s.ResumeOn || s.savedAsked {
		return s, nil
	}
	s.savedAsked = true
	out := []Effect{LoadSaved{Server: server && s.SyncOn}}
	if s.SyncOn && !server && !s.Offline {
		// Offline explains itself; any other way to here leaves the queue
		// unsynced without the user knowing.
		s.queueUnsynced = true
		out = append(out, Notice{Text: unsyncedNote})
	}
	return s, out
}

// withSaving adds the effects Resume needs after e was applied: the
// restore, and a save when what is kept changed.
func withSaving(old, n State, out []Effect) (State, []Effect) {
	n, more := n.restoreSaved()
	out = append(out, more...)
	if n.ResumeOn && n.saving {
		if v := n.saved(); !old.saving || !reflect.DeepEqual(v, old.saved()) {
			keep := n.KeepsServerQueue()
			if !keep {
				n.heldBack = nil
			}
			out = append(out, SaveQueue{
				Saved:      v,
				KeepServer: keep,
				Exact:      !n.Player.Playing || n.Player.Paused,
			})
		}
	}
	return n, out
}

func mmss(d time.Duration) string {
	sec := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

// StopSaving stops the saved queue from being written, for a session that
// is about to be replaced.
func (s State) StopSaving() State {
	s.saving = false
	return s
}
