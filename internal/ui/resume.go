package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/resume"
)

// saveAfter is how long a change to the saved queue waits for further
// ones before it is written.
const saveAfter = time.Second

// queueWrite is a saved queue waiting to be written, for the server and
// user that were logged in when it changed.
type queueWrite struct {
	saved     resume.Saved
	url, user string
}

type saveMsg struct{}

// writeSaved is the command that writes w, or removes the file for an
// empty queue.
func (m *Model) writeSaved(w queueWrite) tea.Cmd {
	dir := m.deps.StateDir
	return func() tea.Msg {
		if dir == "" {
			return nil
		}
		var err error
		if len(w.saved.Tracks) == 0 {
			err = resume.Delete(dir, w.url, w.user)
		} else {
			err = resume.Save(dir, w.url, w.user, w.saved)
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
	if cmd := m.writeSaved(queueWrite{v, srv.URL, srv.User}); cmd != nil {
		cmd()
	}
}

// flushPending writes the queue that waits for its delay, under the
// server and user it was meant for.
func (m *Model) flushPending() {
	if m.pendingSave == nil {
		return
	}
	if cmd := m.writeSaved(*m.pendingSave); cmd != nil {
		cmd()
	}
	m.pendingSave = nil
}

func (m *Model) runResume(e app.Effect) (tea.Cmd, bool) {
	srv := m.deps.Config.Server
	switch e := e.(type) {
	case app.LoadSaved:
		dir := m.deps.StateDir
		return func() tea.Msg {
			if dir == "" {
				return eventMsg{app.SavedLoaded{}}
			}
			s, err := resume.Load(dir, srv.URL, srv.User)
			return eventMsg{app.SavedLoaded{Saved: s, Found: err == nil}}
		}, true
	case app.SaveQueue:
		w := queueWrite{e.Saved, srv.URL, srv.User}
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
		return m.writeSaved(queueWrite{url: srv.URL, user: srv.User}), true
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
