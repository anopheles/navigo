package ui

import (
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

type cmd = command.Command[*Model]

// commands is the registry. Labels are what the hint line shows next to a
// key; Desc is for the key menu and help.
func commands() *command.Registry[*Model] {
	r := command.NewRegistry[*Model]()
	for _, c := range []cmd{
		{Name: "down", Label: "down", Desc: "Move the cursor down.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(c.N()); return nil }},
		{Name: "up", Label: "up", Desc: "Move the cursor up.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(-c.N()); return nil }},
		{Name: "first", Label: "first row", Desc: "Go to the first row, or to row N with a count.", Motion: true, Run: func(m *Model, c command.Call) error { m.goRow(c.Count, 0); return nil }},
		{Name: "last", Label: "last row", Desc: "Go to the last row, or to row N with a count.", Motion: true, Run: func(m *Model, c command.Call) error { m.goRow(c.Count, -1); return nil }},
		{Name: "half-page-down", Label: "half page down", Desc: "Move down half a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(c.N() * m.bodyHeight() / 2); return nil }},
		{Name: "half-page-up", Label: "half page up", Desc: "Move up half a page.", Motion: true, Run: func(m *Model, c command.Call) error { m.move(-c.N() * m.bodyHeight() / 2); return nil }},

		{Name: "open", Label: "open", Desc: "Go into the row under the cursor.", Run: (*Model).open},
		{Name: "back", Label: "back", Desc: "Go back out to the list you came from.", Run: func(m *Model, _ command.Call) error { m.back(); return nil }},
		{Name: "albums", Label: "albums", Desc: "Show the Albums section.", Run: func(m *Model, _ command.Call) error { m.showAlbums(); return nil }},
		{Name: "queue", Label: "queue", Desc: "Move to the queue window.", Run: func(m *Model, _ command.Call) error { m.focus = queueWindow; return nil }},
		{Name: "window", Label: "other window", Desc: "Move to the other window.", Run: func(m *Model, _ command.Call) error { m.focus = 1 - m.focus; return nil }},

		{Name: "play-now", Label: "play", Desc: "Replace the queue with this list and play from the track under the cursor.", Run: (*Model).playNow},
		{Name: "add-end", Label: "add to queue", Desc: "Add the track under the cursor to the end of the queue.", Run: func(m *Model, c command.Call) error { return m.add(c, false) }},
		{Name: "add-next", Label: "play next", Desc: "Add the track under the cursor after the current track.", Run: func(m *Model, c command.Call) error { return m.add(c, true) }},
		{Name: "play-row", Label: "play", Desc: "Play the queue row under the cursor.", Run: func(m *Model, _ command.Call) error { m.event(app.PlayRow{Index: m.queueCur}); return nil }},
		{Name: "play-pause", Label: "play/pause", Desc: "Pause or resume playback, or start the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.PlayPause{}); return nil }},
		{Name: "next-track", Label: "next track", Desc: "Play the next track in the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.NextTrack{}); return nil }},
		{Name: "prev-track", Label: "previous track", Desc: "Play the previous track, or restart this one after 3 seconds.", Run: func(m *Model, _ command.Call) error { m.event(app.PrevTrack{}); return nil }},
		{Name: "seek", Label: "seek", Desc: "Seek by a number of seconds, such as seek +10; a count multiplies it.", Run: seek},
		{Name: "reconnect", Label: "reconnect", Desc: "Try the server now instead of waiting for the next retry.", Run: func(m *Model, _ command.Call) error { m.event(app.Reconnect{}); return nil }},
		{Name: "sync", Label: "sync", Desc: "Sync the whole library from the server.", Run: func(m *Model, _ command.Call) error { m.event(app.SyncNow{}); return nil }},
		{Name: "stop", Label: "stop", Desc: "Stop playback.", Run: func(m *Model, _ command.Call) error { m.event(app.StopPlayback{}); return nil }},
		{Name: "undo", Label: "undo", Desc: "Undo the last change to the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.Undo{}); return nil }},
		{Name: "redo", Label: "redo", Desc: "Redo the last undone change to the queue.", Run: func(m *Model, _ command.Call) error { m.event(app.Redo{}); return nil }},

		{Name: "cmdline", Label: "command", Desc: "Type a command.", Run: func(m *Model, _ command.Call) error { m.enterLine(); return nil }},
		{Name: "cancel", Label: "cancel", Desc: "Cancel pending keys and clear the message.", Run: func(m *Model, _ command.Call) error { m.clearMsg(); return nil }},
		{Name: "close", Label: "close", Desc: "Close the popup.", Run: func(m *Model, _ command.Call) error { m.closePopup(); return nil }},
		{Name: "messages", Label: "messages", Desc: "Show the messages of this session.", Run: func(m *Model, _ command.Call) error { m.showMessages(); return nil }},
		{Name: "interrupt", Label: "cancel", Desc: "Like cancel; reminds you how to quit.", Run: func(m *Model, _ command.Call) error { m.setMsg("type :q to quit", false); return nil }},
		{Name: "login", Label: "log in", Desc: "Log in to a server, or as another user.", Run: func(m *Model, _ command.Call) error {
			s := m.deps.Config.Server
			m.openLogin(s.URL, s.User, "")
			return nil
		}},
		{Name: "quit", Aliases: []string{"q", "qa", "q!", "qa!"}, Label: "quit", Desc: "Quit navigo.", Run: func(m *Model, _ command.Call) error { m.then(tea.Quit); return nil }},
	} {
		r.Add(c)
	}
	return r
}

// then queues a tea.Cmd to run after the current command.
func (m *Model) then(c tea.Cmd) { m.pendingCmd = tea.Batch(m.pendingCmd, c) }

// event sends an event to the core from a command.
func (m *Model) event(e app.Event) { m.then(m.dispatch(e)) }

func (m *Model) move(d int) {
	if m.popup != nil {
		m.scrollPopup(d)
		return
	}
	if m.focus == queueWindow {
		m.queueCur = min(max(m.queueCur+d, 0), max(len(m.state.Queue)-1, 0))
		return
	}
	f := m.top()
	f.cur = min(max(f.cur+d, 0), max(m.rows(f)-1, 0))
}

// goRow moves to row count (1-based) when a count was typed, else to def
// (-1 for the last row).
func (m *Model) goRow(count, def int) {
	if m.popup != nil {
		m.popup.off = 0
		if def < 0 {
			m.scrollPopup(len(m.popup.lines))
		}
		return
	}
	n := m.rows(m.top())
	if m.focus == queueWindow {
		n = len(m.state.Queue)
	}
	row := def
	if count > 0 {
		row = count - 1
	} else if def < 0 {
		row = n - 1
	}
	row = min(max(row, 0), max(n-1, 0))
	if m.focus == queueWindow {
		m.queueCur = row
	} else {
		m.top().cur = row
	}
}

func (m *Model) open(command.Call) error {
	f := m.top()
	switch f.kind {
	case sectionList:
		if f.cur == 0 {
			m.lib = append(m.lib, frame{kind: albumList, title: "Albums"})
		}
	case albumList:
		lib := m.state.Library
		if lib == nil || f.cur >= len(lib.Albums) {
			return nil
		}
		a := lib.Albums[f.cur]
		m.lib = append(m.lib, frame{kind: albumTracks, album: a.ID, title: a.Name})
	}
	return nil
}

func (m *Model) back() {
	if m.focus == libraryWindow && len(m.lib) > 1 {
		m.lib = m.lib[:len(m.lib)-1]
	}
}

func (m *Model) showAlbums() {
	m.focus = libraryWindow
	for i, f := range m.lib {
		if f.kind == albumList {
			m.lib = m.lib[:i+1]
			return
		}
	}
	m.lib = append(m.lib[:1], frame{kind: albumList, title: "Albums"})
}

// trackRow returns the album tracks shown and the cursor row, if the
// cursor is on a track in the library window. An album still loading has
// no rows yet, which is not an error.
func (m *Model) trackRow() ([]library.Track, int, error) {
	f := m.top()
	if m.focus != libraryWindow || f.kind != albumTracks {
		return nil, 0, errNotOnTrack
	}
	var ts []library.Track
	if m.state.Library != nil {
		ts = m.state.Library.AlbumTracks(f.album)
	}
	if f.cur >= len(ts) {
		return nil, 0, errNoRows
	}
	return ts, f.cur, nil
}

var (
	errNotOnTrack = errors.New("not on a track")
	errNoRows     = errors.New("no rows yet")
)

func (m *Model) playNow(command.Call) error {
	ts, i, err := m.trackRow()
	if err != nil {
		return quiet(err)
	}
	m.event(app.PlayNow{Tracks: ts, Start: i})
	return nil
}

func (m *Model) add(c command.Call, next bool) error {
	ts, i, err := m.trackRow()
	if err != nil {
		return quiet(err)
	}
	add := make([]library.Track, c.N())
	for j := range add {
		add[j] = ts[i]
	}
	if next {
		m.event(app.AddNext{Tracks: add})
	} else {
		m.event(app.AddEnd{Tracks: add})
	}
	return nil
}

// quiet drops errNoRows: a key pressed before the list has loaded does
// nothing.
func quiet(err error) error {
	if errors.Is(err, errNoRows) {
		return nil
	}
	return err
}

func seek(m *Model, c command.Call) error {
	if len(c.Args) != 1 {
		return errors.New("seek: give seconds, such as seek +10")
	}
	d, err := seconds(c.Args[0])
	if err != nil {
		return err
	}
	m.event(app.SeekBy{D: d * time.Duration(c.N())})
	return nil
}

func (m *Model) enterLine() {
	m.mode = keymap.Cmdline
	m.line.Reset()
	m.then(m.line.Focus())
}
