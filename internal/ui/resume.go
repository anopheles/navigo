package ui

import (
	"context"
	"log/slog"
	"slices"
	"sync/atomic"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/resume"
	"github.com/anopheles/navigo/internal/subsonic"
)

// saveAfter is how long a change to the saved queue waits for further
// ones before it is written.
const saveAfter = time.Second

// serverQueueTimeout is how long startup waits for the server's play queue
// before it uses this device's file, and how long a write of it may take.
var serverQueueTimeout = 3 * time.Second

// serverPositionStep is how far a playing track's position moves before
// that alone is sent to the server.
const serverPositionStep = time.Minute

// serverPositionSlack is how far back a position may fall and still count as
// the same stretch of playing: a resume after a pause rounds it down.
const serverPositionSlack = 10 * time.Second

// queueWrite is a saved queue waiting to be written, for the server and
// user that were logged in when it changed. With keepServer set only this
// device's file is touched, and the server's play queue stays as it is.
type queueWrite struct {
	saved      resume.Saved
	url, user  string
	keepServer bool
	exact      bool
}

type saveMsg struct{}

// serverQueue is what navigo keeps of the saved queue on the server: the
// queue, its current entry and the position, not the volume or repeat.
type serverQueue struct {
	ids     []string
	current string
	pos     time.Duration
}

func serverQueueOf(v resume.Saved) serverQueue {
	q := serverQueue{pos: v.Position.Truncate(time.Millisecond)}
	for _, id := range v.Tracks {
		q.ids = append(q.ids, string(id))
	}
	if v.Current >= 0 && v.Current < len(q.ids) {
		q.current = q.ids[v.Current]
	} else {
		q.pos = 0
	}
	return q
}

// sameQueue is equal but for the position.
func (q serverQueue) sameQueue(o serverQueue) bool {
	return q.current == o.current && slices.Equal(q.ids, o.ids)
}

func (q serverQueue) equal(o serverQueue) bool {
	return q.current == o.current && q.pos == o.pos && slices.Equal(q.ids, o.ids)
}

// queuePush sends the saved queue to the server. Writes are numbered as they
// are made, so one that was overtaken never replaces a newer one, and a
// write is skipped when the server already holds that queue.
type queuePush struct {
	seq  int           // the newest write made; only the Update loop touches it
	lock chan struct{} // held while the fields below change or a send is out
	sent int           // the newest write that was sent
	last *serverQueue  // what the server holds, if known
	// read says this session knows what the server holds, or was told to
	// replace it. Until then nothing is sent, so a device that started
	// without having read the server's queue can't overwrite it.
	read atomic.Bool
}

func newQueuePush() *queuePush { return &queuePush{lock: make(chan struct{}, 1)} }

// acquire takes the lock, or gives up when ctx ends first.
func (p *queuePush) acquire(ctx context.Context) bool {
	select {
	case p.lock <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (p *queuePush) release() { <-p.lock }

// knows records that the server holds q.
func (p *queuePush) knows(q serverQueue) {
	p.lock <- struct{}{}
	p.last = &q
	p.read.Store(true)
	p.release()
}

// allow lets this session replace whatever the server holds, for when
// sync_queue is turned on while navigo runs.
func (p *queuePush) allow() { p.read.Store(true) }

// send stores v as the server's play queue. It is best effort: a failure
// is logged and is neither shown nor counted as the server being offline.
//
// One deadline covers waiting for a send still out and the send itself.
// While a track plays, a change of position alone is sent once it has moved
// serverPositionStep; exact positions (paused, stopped, quitting) always go.
func (p *queuePush) send(
	ctx context.Context,
	c *subsonic.Client,
	seq int,
	v resume.Saved,
	exact bool,
) {
	ctx, cancel := context.WithTimeout(ctx, serverQueueTimeout)
	defer cancel()
	if !p.acquire(ctx) {
		slog.Warn("couldn't save the play queue on the server", "err", ctx.Err())
		return
	}
	defer p.release()
	if seq < p.sent || !p.read.Load() {
		return
	}
	p.sent = seq
	q := serverQueueOf(v)
	if p.last != nil && p.last.equal(q) {
		return
	}
	if !exact && p.last != nil && p.last.sameQueue(q) &&
		q.pos-p.last.pos > -serverPositionSlack && q.pos-p.last.pos < serverPositionStep {
		return
	}
	if err := c.SavePlayQueue(ctx, q.ids, q.current, q.pos); err != nil {
		slog.Warn("couldn't save the play queue on the server", "err", err)
		p.last = nil
		return
	}
	p.last = &q
}

// loadSaved is the command that answers LoadSaved: with the server asked,
// its play queue when it has one, else this device's file. The volume and
// repeat always come from the file.
func (m *Model) loadSaved(server bool) tea.Cmd {
	ctx, c, dir, srv, push := m.ctx, m.client, m.deps.StateDir, m.deps.Config.Server, m.push
	return func() tea.Msg {
		var local resume.Saved
		found := false
		if dir != "" {
			var err error
			local, err = resume.Load(dir, srv.URL, srv.User)
			found = err == nil
		}
		loaded := app.SavedLoaded{Saved: local, Found: found, Settings: found}
		if !server || c == nil {
			return eventMsg{loaded}
		}
		ctx, cancel := context.WithTimeout(ctx, serverQueueTimeout)
		defer cancel()
		pq, err := c.PlayQueue(ctx)
		if err != nil {
			slog.Warn("couldn't read the play queue from the server", "err", err)
			return eventMsg{loaded}
		}
		push.knows(serverQueue{ids: pq.IDs, current: pq.Current, pos: pq.Position})
		if len(pq.IDs) == 0 {
			return eventMsg{loaded}
		}
		remote := resume.Saved{Current: slices.Index(pq.IDs, pq.Current), Position: pq.Position}
		for _, id := range pq.IDs {
			remote.Tracks = append(remote.Tracks, library.TrackID(id))
		}
		if remote.Current < 0 {
			remote.Position = 0
		}
		if found {
			remote.Volume, remote.Repeat = local.Volume, local.Repeat
		}
		return eventMsg{app.SavedLoaded{Saved: remote, Found: true, Settings: found}}
	}
}

// writeSaved is the command that writes w: this device's file, or its
// removal for an empty queue, and the server's play queue.
func (m *Model) writeSaved(w queueWrite) tea.Cmd {
	dir, c, ctx, p := m.deps.StateDir, m.client, m.ctx, m.push
	push := !w.keepServer && c != nil && m.state.SyncOn && !m.state.Offline
	seq := 0
	if push {
		p.seq++
		seq = p.seq
	}
	return func() tea.Msg {
		var err error
		switch {
		case dir == "":
		case len(w.saved.Tracks) == 0:
			err = resume.Delete(dir, w.url, w.user)
		default:
			err = resume.Save(dir, w.url, w.user, w.saved)
		}
		if push {
			p.send(ctx, c, seq, w.saved, w.exact)
		}
		if err != nil {
			return eventMsg{app.Failed{What: "couldn't save the queue", Err: err}}
		}
		return nil
	}
}

// FlushSaved writes the saved queue as it is now, with the exact
// position. It runs when navigo quits.
func (m *Model) FlushSaved() {
	m.pendingSave = nil
	v, ok := m.state.SaveNow()
	if !ok {
		return
	}
	srv := m.deps.Config.Server
	if cmd := m.writeSaved(queueWrite{
		saved: v, url: srv.URL, user: srv.User, keepServer: m.state.KeepsServerQueue(), exact: true,
	}); cmd != nil {
		cmd()
	}
}

// flushPending writes the queue that waits for its delay, under the
// server and user it was meant for.
func (m *Model) flushPending() {
	if m.pendingSave == nil {
		return
	}
	w := *m.pendingSave
	w.keepServer = true // a slow old server must not hold up the new login
	if cmd := m.writeSaved(w); cmd != nil {
		cmd()
	}
	m.pendingSave = nil
}

func (m *Model) runResume(e app.Effect) (tea.Cmd, bool) {
	srv := m.deps.Config.Server
	switch e := e.(type) {
	case app.LoadSaved:
		return m.loadSaved(e.Server), true
	case app.SaveQueue:
		w := queueWrite{
			saved: e.Saved, url: srv.URL, user: srv.User, keepServer: e.KeepServer, exact: e.Exact,
		}
		if len(e.Saved.Tracks) == 0 {
			m.pendingSave = nil
			return m.writeSaved(w), true
		}
		m.pendingSave = &w
		if m.saveArmed {
			return nil, true
		}
		m.saveArmed = true
		return m.clock.After(saveAfter, func(time.Time) tea.Msg { return saveMsg{} }), true
	case app.DeleteSaved:
		m.pendingSave = nil
		return m.writeSaved(queueWrite{url: srv.URL, user: srv.User, keepServer: true}), true
	}
	return nil, false
}

// saveDue writes the queue that waited out the delay.
func (m *Model) saveDue() tea.Cmd {
	m.saveArmed = false
	w := m.pendingSave
	m.pendingSave = nil
	if w == nil {
		return nil
	}
	return m.writeSaved(*w)
}
