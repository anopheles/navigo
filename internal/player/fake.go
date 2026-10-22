package player

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Fake is an in-memory Player for tests. It records every call, keeps the
// span and transport state the calls imply, and emits the events a real
// player would for them. Finish and Die drive what mpv would do by itself.
type Fake struct {
	mu      sync.Mutex
	calls   []string
	span    Span
	playing bool
	paused  bool
	pos     time.Duration
	volume  int
	events  chan Event
	closed  bool

	// Err, when set, is returned by every call instead of acting.
	Err error
	// CheckErr is what Check returns.
	CheckErr error
}

var _ Player = (*Fake)(nil)

// NewFake returns a Fake with room for 1024 unread events.
func NewFake() *Fake {
	return &Fake{events: make(chan Event, 1024), volume: 100}
}

// Calls returns the calls so far, such as "Play(0 1 2)" (entry IDs of the
// span) or "SeekBy(10s)".
func (f *Fake) Calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

// Span returns the entries the fake holds; zero when stopped.
func (f *Fake) Span() Span {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.span
}

// State returns whether an entry is playing, whether it's paused, its
// position and the volume.
func (f *Fake) State() (playing, paused bool, pos time.Duration, volume int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.playing, f.paused, f.pos, f.volume
}

func (f *Fake) Events() <-chan Event { return f.events }

func (f *Fake) Check(context.Context) error { return f.CheckErr }

func (f *Fake) Play(_ context.Context, w Span) error {
	call := fmt.Sprintf("Play(%d %d %d)", w.Prev.ID, w.Current.ID, w.Next.ID)
	if w.Start > 0 {
		call = fmt.Sprintf("Play(%d %d %d from %v)", w.Prev.ID, w.Current.ID, w.Next.ID, w.Start)
	}
	return f.do(call, func() {
		if f.playing {
			f.emit(TrackEnded{Entry: f.span.Current.ID, Reason: Stopped})
		}
		f.span, f.playing, f.paused, f.pos = w, true, false, w.Start
		f.emit(TrackStarted{Entry: w.Current.ID})
		if w.Start > 0 { // as mpv reports the position it starts at
			f.emit(Position{Entry: w.Current.ID, Position: w.Start})
		}
	})
}

func (f *Fake) SetNeighbours(_ context.Context, prev, next Entry) error {
	return f.do(fmt.Sprintf("SetNeighbours(%d %d)", prev.ID, next.ID), func() {
		if f.playing {
			f.span.Prev, f.span.Next = prev, next
		}
	})
}

func (f *Fake) Pause(context.Context) error {
	return f.do("Pause", func() { f.setPaused(true) })
}

func (f *Fake) Resume(context.Context) error {
	return f.do("Resume", func() { f.setPaused(false) })
}

func (f *Fake) SeekTo(_ context.Context, pos time.Duration) error {
	return f.do(fmt.Sprintf("SeekTo(%v)", pos), func() { f.seek(pos) })
}

func (f *Fake) SeekBy(_ context.Context, d time.Duration) error {
	return f.do(fmt.Sprintf("SeekBy(%v)", d), func() { f.seek(f.pos + d) })
}

func (f *Fake) SetVolume(_ context.Context, percent int) error {
	return f.do(fmt.Sprintf("SetVolume(%d)", percent), func() {
		f.volume = min(max(percent, 0), 100)
		f.emit(Volume{Percent: f.volume})
	})
}

func (f *Fake) SetReplayGain(_ context.Context, g ReplayGain) error {
	return f.do(fmt.Sprintf("SetReplayGain(%s, %g)", g.Mode, g.Preamp), func() {})
}

func (f *Fake) Stop(context.Context) error {
	return f.do("Stop", func() {
		if f.playing {
			f.emit(TrackEnded{Entry: f.span.Current.ID, Reason: Stopped})
		}
		f.span, f.playing, f.pos = Span{}, false, 0
	})
}

func (f *Fake) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, "Close")
	if !f.closed {
		f.closed = true
		close(f.events)
	}
	return nil
}

// Finish plays the current entry to its end, as mpv would: it ends, and
// the next one, if any, starts, becoming current with the old one as
// previous and no next until SetNeighbours.
func (f *Fake) Finish() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.playing {
		return
	}
	f.emit(TrackEnded{Entry: f.span.Current.ID, Reason: Finished})
	if f.span.Next.ID == 0 {
		f.span, f.playing, f.pos = Span{}, false, 0
		return
	}
	f.span = Span{Prev: f.span.Current, Current: f.span.Next}
	f.pos = 0
	f.emit(TrackStarted{Entry: f.span.Current.ID})
}

// FailCurrent makes the current entry's stream fail, as mpv does when it
// can't load it: the entry ends with Failed and mpv runs on to the next
// one, which (with the server gone) fails too.
func (f *Fake) FailCurrent(reason string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.playing {
		return
	}
	f.emit(TrackEnded{Entry: f.span.Current.ID, Reason: Failed, Err: reason})
	if next := f.span.Next; next.ID != 0 {
		f.emit(TrackStarted{Entry: next.ID})
		f.emit(TrackEnded{Entry: next.ID, Reason: Failed, Err: reason})
	}
	f.span, f.playing, f.pos = Span{}, false, 0
}

// FailAudio makes the audio output fail to open, as mpv does when the
// system's sound server is gone: the entry ends with NoAudio, and so does
// the next one mpv runs on to.
func (f *Fake) FailAudio() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.playing {
		return
	}
	f.emit(TrackEnded{Entry: f.span.Current.ID, Reason: NoAudio, Err: aoInitFailed})
	if next := f.span.Next; next.ID != 0 {
		f.emit(TrackStarted{Entry: next.ID})
		f.emit(TrackEnded{Entry: next.ID, Reason: NoAudio, Err: aoInitFailed})
	}
	f.span, f.playing, f.pos = Span{}, false, 0
}

// Tick advances the position of the playing entry and reports it.
func (f *Fake) Tick(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.playing && !f.paused {
		f.pos += d
		f.emit(Position{Entry: f.span.Current.ID, Position: f.pos})
	}
}

// Die makes the player fail as if mpv had exited.
func (f *Fake) Die(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.span, f.playing, f.paused, f.pos = Span{}, false, false, 0
	f.emit(Died{Err: err})
}

// Emit sends any event, for cases the methods above don't cover.
func (f *Fake) Emit(e Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emit(e)
}

func (f *Fake) do(call string, act func()) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
	if f.Err != nil {
		return f.Err
	}
	act()
	return nil
}

func (f *Fake) setPaused(p bool) {
	if f.playing && f.paused != p {
		f.paused = p
		f.emit(Paused{Paused: p})
	}
}

func (f *Fake) seek(pos time.Duration) {
	if f.playing {
		f.pos = max(pos, 0)
		f.emit(Position{Entry: f.span.Current.ID, Position: f.pos})
	}
}

// emit must be called with f.mu held. A full buffer means the test isn't
// reading Events, which would hang a real runner too.
func (f *Fake) emit(e Event) {
	if f.closed {
		return
	}
	select {
	case f.events <- e:
	default:
		panic("player.Fake: Events buffer full; read Events in the test")
	}
}
