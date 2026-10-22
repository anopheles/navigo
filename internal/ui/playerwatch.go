package ui

import (
	"sync/atomic"
	"time"
)

// playerPatience is how long a player call may go unanswered before the
// now-playing bar says mpv isn't responding, as when it hangs opening
// the audio output.
const playerPatience = 5 * time.Second

// playerWatch follows one player call, to tell the core when it is slow
// to come back and when it finally does.
type playerWatch struct {
	started, done chan struct{}
	state         atomic.Int32 // watchRunning, watchAnswered or watchStuck
}

const (
	watchRunning int32 = iota
	watchAnswered
	watchStuck
)

func newPlayerWatch() *playerWatch {
	return &playerWatch{started: make(chan struct{}), done: make(chan struct{})}
}

// start marks the call as running; the patience counts from here, not
// from when it queued behind other calls.
func (w *playerWatch) start() { close(w.started) }

// answered marks the call as returned. It reports whether the core was
// told the call was stuck, and so must now hear it isn't.
func (w *playerWatch) answered() bool {
	close(w.done)
	return !w.state.CompareAndSwap(watchRunning, watchAnswered)
}

// wait blocks until the call has run for the patience given by after, or
// returned; it reports whether it is stuck. A call that answers just as
// time runs out counts as answered.
func (w *playerWatch) wait(after func() <-chan time.Time) bool {
	<-w.started
	select {
	case <-w.done:
		return false
	case <-after():
		return w.state.CompareAndSwap(watchRunning, watchStuck)
	}
}
