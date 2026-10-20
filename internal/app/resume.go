package app

import (
	"fmt"
	"reflect"
	"time"

	"github.com/anopheles/navigo/internal/resume"
)

// Events and effects for Resume: the saved queue is read at startup and
// written as the queue changes.
type (
	// SavedLoaded is the saved queue the file held; Found is false when
	// there was none.
	SavedLoaded struct {
		Saved resume.Saved
		Found bool
	}
	// ConfigureResume turns Resume on or off, at startup and on
	// :reload-config.
	ConfigureResume struct{ On bool }
	// LoggedOut forgets the saved queue and stops saving it.
	LoggedOut struct{}
)

type (
	// LoadSaved reads the saved queue, answered with SavedLoaded.
	LoadSaved struct{}
	// SaveQueue writes the saved queue; an empty one removes the file.
	SaveQueue struct{ Saved resume.Saved }
	// DeleteSaved removes the saved queue, and any write still waiting.
	DeleteSaved struct{}
)

func (SavedLoaded) isEvent()     {}
func (ConfigureResume) isEvent() {}
func (LoggedOut) isEvent()       {}
func (LoadSaved) isEffect()      {}
func (SaveQueue) isEffect()      {}
func (DeleteSaved) isEffect()    {}

// positionStep is how coarsely the position of a playing track is saved,
// so playing doesn't rewrite the file every second.
const positionStep = 10 * time.Second

// saved is what Resume keeps of this state. The position is the playing
// one (rounded down, but exact once paused), or where a restored or
// interrupted track will start from.
func (s State) saved() resume.Saved { return s.savedAt(positionStep) }

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
	case SavedLoaded:
		s.savedLoaded = true
		if e.Found {
			s.pending = &e.Saved
		} else {
			s.restored, s.saving = true, true
		}
	case ConfigureResume:
		was := s.ResumeOn
		s.ResumeOn = e.On
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
	p := *s.pending
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
	s.Player.Volume = min(max(p.Volume, 0), 100)
	if r := Repeat(p.Repeat); r >= RepeatOff && r <= RepeatOne {
		s.Repeat = r
	}
	text := fmt.Sprintf("restored %d %s", len(q), plural(len(q), "track"))
	if len(q) < len(p.Tracks) {
		text = fmt.Sprintf("restored %d of %d tracks", len(q), len(p.Tracks))
	}
	return s, []Effect{
		SetVolume{Percent: s.Player.Volume},
		Notice{Text: text, Command: "play-pause", Hint: hint},
	}
}

// withSaving adds the effects Resume needs after e was applied: the
// restore, and a save when what is kept changed.
func withSaving(old, n State, out []Effect) (State, []Effect) {
	n, more := n.restoreSaved()
	out = append(out, more...)
	if n.ResumeOn && n.saving {
		if v := n.saved(); !old.saving || !reflect.DeepEqual(v, old.saved()) {
			out = append(out, SaveQueue{Saved: v})
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
