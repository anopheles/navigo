// Package ui is navigo's Bubble Tea front end: the login form, the library
// and queue windows, the hint line and the now-playing bar. It wraps the
// core (internal/app), whose Apply is called only from Update, and runs
// the core's effects as tea.Cmds. Keys go through the keymap engine to
// commands in the registry, so every key does something typeable at ":".
package ui

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/creds"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/mediactl"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic"
)

// stallAfter is how long buffering may last before navigo checks the server.
const stallAfter = 10 * time.Second

// Deps is what the UI needs from outside.
type Deps struct {
	Config     config.Config
	ConfigPath string
	Player     player.Player
	Keyring    creds.Keyring
	// CacheDir holds the library cache files; empty means no cache.
	CacheDir string
	// StateDir holds the search history; empty means it isn't kept.
	StateDir string
	// LogPath is navigo.log, named in errors navigo can't explain.
	LogPath string
	// Media is the OS's media controls; nil for none.
	Media mediactl.Controls
	// OnLogin is called with the client once the server accepted the
	// credentials, so the player can sign stream URLs with it.
	OnLogin func(*subsonic.Client)
}

// PlayerMsg wraps a Player event for Program.Send.
func PlayerMsg(e player.Event) tea.Msg { return eventMsg{app.FromPlayer{Event: e}} }

type (
	eventMsg    struct{ app.Event }
	passwordMsg struct {
		password string
		err      error
	}
	loginMsg struct {
		client *subsonic.Client
		url    string
		err    error
	}
	savedMsg   struct{ err error }
	timeoutMsg struct{ seq int }
	secondMsg  struct{} // redraws the offline countdown
	stallMsg   struct{ seq int }
)

type screen int

const (
	screenMain screen = iota
	screenLogin
)

type window int

const (
	libraryWindow window = iota
	queueWindow
)

type listKind int

const (
	sectionList listKind = iota
	homeList
	homeAlbums
	artistList
	artistPage
	albumList
	albumTracks
	playlistList
	playlistTracks
	searchList
	helpList
)

// frame is one level of the library window's drill-down.
type frame struct {
	kind        listKind
	album       library.AlbumID
	artist      library.ArtistID
	playlist    library.PlaylistID
	homeKind    string   // the server's list type, for homeAlbums
	view        listView // :sort and :filter
	search      *searchView
	help        *helpView
	title       string
	cur, offset int
}

// Model is the root Bubble Tea model. It is used by pointer.
type Model struct {
	deps  Deps
	state app.State
	// pendingSave is the saved queue waiting to be written; saveArmed,
	// that the timer for it is running.
	pendingSave *queueWrite
	// connecting is the ID of the "connecting to…" message, which login
	// removes and nothing else.
	connecting int
	saveArmed  bool
	client     *subsonic.Client
	ctx        context.Context

	screen screen
	login  loginForm
	w, h   int

	focus    window
	visual   *visualSel // the Visual mode selection, if any
	lib      []frame
	paths    map[listKind][]frame // where each section was left
	queueCur int
	queueOff int

	mode     keymap.Mode
	engine   *keymap.Engine
	keys     keySettings
	registry *command.Registry[*Model]
	line     textinput.Model
	seq      int // bumps on every key, to drop stale timeouts

	// The ":" line and the search prompt. A search prompt's text is
	// selected until a key changes it, and beforeSearch is what <Esc>
	// restores. compl is the <Tab> completion being cycled through.
	cmdHist      lineHistory
	searchHist   lineHistory
	selected     bool
	beforeSearch *savedView
	compl        *completion
	// The Help filter prompt shares the search prompt's mode; helpBefore
	// is the filter <Esc> puts back.
	promptHelp bool
	helpBefore string
	// prefixShown: a pure prefix has waited long enough for its menu.
	prefixShown bool
	finding     finder
	// offerSearch is a pattern / didn't find: <CR> next searches the
	// library for it.
	offerSearch string
	// jumps is the jump list; jumpAt is where <C-o> and <C-i> are in it,
	// len(jumps) when not walking it.
	jumps  []location
	jumpAt int

	shown   message // in the status line; zero when empty
	history []message
	msgSeq  int
	popup   *popup
	hints   bool

	clock  timeSource
	timers []tea.Cmd // timers set while handling a message

	retryAt  time.Time // when the next try of the server is due, while offline
	ticking  bool      // the once-a-second redraw of the countdown is running
	stallSeq int       // the buffering-stall timer; older ones are ignored

	player sequencer
	seedFn func() uint64 // shuffle seeds; random unless a test sets it
	// send delivers messages from long-running work, such as sync
	// progress; Program.Send in navigo, a queue in tests.
	send func(tea.Msg)
	// pendingCmd collects what a command handler wants run next.
	pendingCmd tea.Cmd
}

// New returns the model; Init starts logging in. It refuses a keys: block
// that leaves no way to quit.
func New(deps Deps) (*Model, error) {
	applyColors(deps.Config.RoleColors())
	line := newInput()
	line.Prompt = ":"
	m := &Model{
		deps:     deps,
		state:    newState(deps.Config),
		ctx:      context.Background(),
		lib:      startPath(),
		paths:    map[listKind][]frame{},
		mode:     keymap.Normal,
		registry: commands(),
		line:     line,
		hints:    deps.Config.HintsOn(),
		clock:    realClock{},

		cmdHist:    loadHistory(deps.StateDir, "cmd_history", "command history"),
		searchHist: loadHistory(deps.StateDir, "search_history", "search history"),
	}
	t, set, problems, err := buildKeys(deps.Config.Keys, m.registry)
	if err != nil {
		return nil, err
	}
	m.keys = set
	m.setTable(t)
	m.showProblems(append(problems, deps.Config.Problems()...))
	m.player.init()
	if config.LooseWithPassword(deps.ConfigPath, deps.Config) {
		m.setMsg(
			fmt.Sprintf(
				"%s holds a password and others can read it · chmod 600 it",
				deps.ConfigPath,
			),
			true,
		)
	}
	return m, nil
}

// SetSend gives the model a way to deliver messages from work that runs
// longer than one command, such as sync progress.
func (m *Model) SetSend(send func(tea.Msg)) { m.send = send }

func (m *Model) Init() tea.Cmd {
	s := m.deps.Config.Server
	p, ctx := m.deps.Player, m.ctx
	check := func() tea.Msg { return eventMsg{app.PlayerChecked{Err: p.Check(ctx), Hint: installHint()}} }
	if s.URL == "" {
		m.openLogin(s.URL, s.User, "")
		return check
	}
	m.setMsg("connecting to "+s.URL+"…", false)
	m.connecting = m.shown.id
	return tea.Batch(check, m.dispatch(app.Start{}), func() tea.Msg {
		p, err := creds.Password(m.ctx, s, m.deps.Keyring)
		return passwordMsg{p, err}
	})
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.update(msg)
	if len(m.timers) > 0 {
		cmd = tea.Batch(append(m.timers, cmd)...)
		m.timers = nil
	}
	return m, cmd
}

func (m *Model) update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case expireMsg:
		m.expire(msg.id)
		return m, nil
	case mediaMsg:
		return m, m.mediaCommand(msg.Command)
	case saveMsg:
		return m, m.saveDue()
	case secondMsg:
		m.ticking = false
		m.tick()
		return m, nil
	case stallMsg:
		if msg.seq != m.stallSeq || !m.state.Player.Buffering {
			return m, nil
		}
		m.armStall()
		return m, m.dispatch(app.BufferingStalled{})
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.fitLine()
		return m, nil
	case passwordMsg:
		s := m.deps.Config.Server
		if msg.err != nil {
			text := ""
			if !errors.Is(msg.err, creds.ErrNoPassword) {
				text = msg.err.Error()
			}
			m.openLogin(s.URL, s.User, text)
			return m, nil
		}
		return m, m.connect(s.URL, s.User, msg.password)
	case loginMsg:
		return m, m.loggedIn(msg)
	case savedMsg:
		return m, m.saved(msg)
	case eventMsg:
		if fp, ok := msg.Event.(app.FromPlayer); ok {
			if d, ok := fp.Event.(player.Died); ok { // the crash cap counts in time
				return m, m.dispatch(app.PlayerDied{Err: d.Err, At: m.clock.Now()})
			}
			if b, ok := fp.Event.(player.Buffering); ok && b.Buffering &&
				!m.state.Player.Buffering {
				m.armStall()
			}
		}
		switch msg.Event.(type) {
		case app.Synced, app.CacheLoaded:
			keep := m.cursorIDs()
			cmd := m.dispatch(msg.Event)
			for _, f := range m.searchFrames() {
				f.search.run(m.state.Library)
			}
			m.restoreCursors(keep)
			return m, cmd
		}
		return m, m.dispatch(msg.Event)
	case prefixMsg:
		m.prefixShown = msg.seq == m.seq && m.engine.Pending() != ""
		return m, nil
	case timeoutMsg:
		if msg.seq == m.seq {
			return m, m.result(m.engine.Timeout(m.mode, m.scopes()))
		}
		return m, nil
	case tea.KeyPressMsg:
		m.seq++
		m.prefixShown = false
		if m.screen == screenLogin {
			return m, m.loginKey(msg)
		}
		return m, m.key(msg)
	}
	if m.mode == keymap.Cmdline || m.mode == keymap.Search {
		return m, m.edit(msg)
	}
	return m, nil
}

// connect logs in with credentials from the config.
func (m *Model) connect(url, user, password string) tea.Cmd {
	return func() tea.Msg {
		c := subsonic.New(subsonic.Credentials{URL: url, Username: user, Password: password})
		return loginMsg{client: c, url: url, err: c.Login(m.ctx)}
	}
}

func (m *Model) loggedIn(msg loginMsg) tea.Cmd {
	if m.screen == screenLogin {
		return m.loginResult(msg)
	}
	if subsonic.IsCode(msg.err, subsonic.CodeWrongCredentials) {
		s := m.deps.Config.Server
		m.openLogin(s.URL, s.User, "the server rejected the saved username or password")
		return nil
	}
	if subsonic.IsUnreachable(msg.err) {
		// Open from the cache; the credentials are checked once the
		// server is back.
		m.client = msg.client
		if m.deps.OnLogin != nil {
			m.deps.OnLogin(msg.client)
		}
		m.clearConnecting()
		return m.dispatch(app.StartedOffline{})
	}
	if msg.err != nil {
		m.setMsg("can't log in: "+msg.err.Error(), true)
		return nil
	}
	return m.ready(msg.client)
}

// ready starts the session once the server accepted the credentials. A
// new login mid-session starts over: nothing from the old server stays.
func (m *Model) ready(c *subsonic.Client) tea.Cmd {
	var stop tea.Cmd
	if m.client != nil {
		// The old session's queue is saved under its own server and user,
		// and nothing more of it is: the config already names the new one.
		m.flushPending()
		m.state = m.state.StopSaving()
		stop = tea.Batch(m.dispatch(app.StopPlayback{}))
		m.state = newState(m.deps.Config)
		m.lib, m.paths = startPath(), map[listKind][]frame{}
		m.focus, m.queueCur, m.queueOff = libraryWindow, 0, 0
		stop = tea.Batch(stop, m.dispatch(app.Start{}))
	} else if m.screen == screenLogin && !m.state.Sync.Running && m.state.Library == nil {
		// First run: the cache wasn't tried before the login form.
		stop = m.dispatch(app.Start{})
	}
	m.client = c
	m.screen = screenMain
	if m.deps.OnLogin != nil {
		m.deps.OnLogin(c)
	}
	m.clearConnecting()
	return tea.Batch(stop, m.dispatch(app.LoggedIn{}))
}

// dispatch runs an event through the core and the resulting effects.
func (m *Model) dispatch(e app.Event) tea.Cmd {
	var effects []app.Effect
	m.state, effects = app.Apply(m.state, e)
	m.clamp()
	var cmds []tea.Cmd
	for _, eff := range effects {
		if cmd := m.run(eff); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// run turns one effect into a command. Player effects keep their order
// even though Bubble Tea runs commands concurrently.
func (m *Model) run(e app.Effect) tea.Cmd {
	ctx, c, p := m.ctx, m.client, m.deps.Player
	if cmd, ok := m.runResume(e); ok {
		return cmd
	}
	switch e := e.(type) {
	case app.Notice:
		m.notice(e)
		return nil
	case app.LoadCache:
		dir, srv := m.deps.CacheDir, m.deps.Config.Server
		return func() tea.Msg {
			if dir == "" {
				return eventMsg{app.CacheLoaded{}}
			}
			snap, _ := library.Load(dir, srv.URL, srv.User) // none or unusable: sync from scratch
			return eventMsg{app.CacheLoaded{Library: snap}}
		}
	case app.StartSync:
		srv, send := m.deps.Config.Server, m.send
		progress := func(done, total int) {
			if send != nil {
				send(eventMsg{app.SyncProgress{Done: done, Total: total}})
			}
		}
		return func() tea.Msg {
			if c == nil {
				return eventMsg{app.Synced{Err: errors.New("not logged in")}}
			}
			due, st, err := library.Due(ctx, c, e.Old, srv.URL, srv.User)
			if subsonic.IsUnreachable(err) {
				return eventMsg{app.Unreachable{Err: err}}
			}
			if err != nil {
				return eventMsg{app.Synced{Err: err}}
			}
			var snap *library.Snapshot
			if due || e.Full {
				snap, err = library.Sync(ctx, c, srv.URL, srv.User, st, progress)
			} else {
				snap, err = library.Refresh(ctx, c, e.Old)
			}
			if subsonic.IsUnreachable(err) {
				return eventMsg{app.Unreachable{Err: err}}
			}
			return eventMsg{app.Synced{Library: snap, Err: err}}
		}
	case app.SaveCache:
		dir := m.deps.CacheDir
		return func() tea.Msg {
			if dir != "" {
				if err := library.Save(dir, e.Library); err != nil {
					return eventMsg{app.Failed{What: "couldn't save the library cache", Err: err}}
				}
			}
			return nil
		}
	case app.Scrobble:
		return func() tea.Msg {
			if c == nil {
				return nil
			}
			// Fire and forget, but a failure to reach the server counts.
			err := c.Scrobble(ctx, string(e.Track), e.Submission)
			switch {
			case subsonic.IsUnreachable(err):
				return eventMsg{app.Unreachable{Err: err}}
			case err == nil && e.Submission:
				return eventMsg{app.Scrobbled{Track: e.Track, At: time.Now()}}
			}
			return nil
		}
	case app.Probe:
		return func() tea.Msg {
			if c == nil {
				return eventMsg{app.ProbeResult{Why: e.Why, Err: errors.New("not logged in")}}
			}
			err := c.Ping(ctx)
			switch {
			case subsonic.IsCode(err, subsonic.CodeWrongCredentials):
				return eventMsg{app.ProbeResult{Why: e.Why, WrongCredentials: true}}
			case subsonic.IsUnreachable(err):
				return eventMsg{app.ProbeResult{Why: e.Why, Err: err}}
			}
			return eventMsg{app.ProbeResult{Why: e.Why}} // the server answered
		}
	case app.PublishNowPlaying:
		if m.deps.Media != nil {
			m.deps.Media.Update(mediaStatus(e.NowPlayingInfo))
		}
		return nil
	case app.ScheduleUncap:
		return m.clock.After(e.After, func(time.Time) tea.Msg { return eventMsg{app.Uncap{}} })
	case app.ScheduleRetry:
		m.retryAt = m.clock.Now().Add(e.After)
		m.tick()
		seq := e.Seq
		return m.clock.After(
			e.After,
			func(time.Time) tea.Msg { return eventMsg{app.RetryNow{Seq: seq}} },
		)
	case app.FetchAlbumList:
		return func() tea.Msg {
			if c == nil {
				return eventMsg{app.AlbumListLoaded{Kind: e.Kind, Err: errors.New("not logged in")}}
			}
			list, err := c.AlbumList(ctx, subsonic.ListType(e.Kind), subsonic.HomeSize)
			albums := make([]library.Album, len(list))
			for i, a := range list {
				albums[i] = library.AlbumFrom(a)
			}
			return eventMsg{
				app.AlbumListLoaded{
					Kind:        e.Kind,
					Albums:      albums,
					Err:         err,
					Unreachable: subsonic.IsUnreachable(err),
				},
			}
		}
	case app.WritePlaylist:
		return func() tea.Msg {
			if c == nil {
				return eventMsg{app.PlaylistWritten{Write: e, Err: errors.New("not logged in")}}
			}
			var newID string
			var err error
			switch e.Kind {
			case app.UpdatePlaylist:
				add := make([]string, len(e.Add))
				for i, id := range e.Add {
					add[i] = string(id)
				}
				err = c.UpdatePlaylist(ctx, string(e.ID), add, e.Remove)
			case app.ReplacePlaylist, app.CreatePlaylist:
				songs := make([]string, len(e.Songs))
				for i, id := range e.Songs {
					songs[i] = string(id)
				}
				newID, err = c.CreatePlaylist(ctx, e.Name, string(e.ID), songs)
			case app.DeletePlaylist:
				err = c.DeletePlaylist(ctx, string(e.ID))
			}
			return eventMsg{
				app.PlaylistWritten{
					Write:       e,
					NewID:       library.PlaylistID(newID),
					Err:         err,
					Unreachable: subsonic.IsUnreachable(err),
				},
			}
		}
	case app.WriteStar:
		return func() tea.Msg {
			if c == nil {
				return eventMsg{app.StarsWritten{Write: e, Err: errors.New("not logged in")}}
			}
			err := c.Star(ctx, e.On, e.Artists, e.Albums, e.Tracks)
			return eventMsg{
				app.StarsWritten{Write: e, Err: err, Unreachable: subsonic.IsUnreachable(err)},
			}
		}
	case app.OpenLogin:
		s := m.deps.Config.Server
		m.openLogin(s.URL, s.User, "the server rejected the saved username or password")
		return nil
	}

	var call func() error
	switch e := e.(type) {
	case app.Play:
		call = func() error { return p.Play(ctx, e.Span) }
	case app.SetNeighbours:
		call = func() error { return p.SetNeighbours(ctx, e.Prev, e.Next) }
	case app.Pause:
		call = func() error { return p.Pause(ctx) }
	case app.Resume:
		call = func() error { return p.Resume(ctx) }
	case app.Seek:
		call = func() error { return p.SeekBy(ctx, e.By) }
	case app.SeekTo:
		call = func() error { return p.SeekTo(ctx, e.Pos) }
	case app.Stop:
		call = func() error { return p.Stop(ctx) }
	case app.SetVolume:
		call = func() error { return p.SetVolume(ctx, e.Percent) }
	case app.SetReplayGain:
		call = func() error { return p.SetReplayGain(ctx, e.ReplayGain) }
	default:
		return nil
	}
	ticket := m.player.ticket()
	return func() tea.Msg {
		var err error
		m.player.run(ticket, func() { err = call() })
		if err != nil {
			return eventMsg{app.PlayerFailed{Err: err}}
		}
		return nil
	}
}

// tick keeps the offline countdown redrawing once a second.
func (m *Model) tick() {
	if m.ticking || !m.state.Offline {
		return
	}
	m.ticking = true
	m.timers = append(
		m.timers,
		m.clock.After(time.Second, func(time.Time) tea.Msg { return secondMsg{} }),
	)
}

// armStall checks again in 10 s whether playback is still buffering.
func (m *Model) armStall() {
	m.stallSeq++
	seq := m.stallSeq
	m.timers = append(
		m.timers,
		m.clock.After(stallAfter, func(time.Time) tea.Msg { return stallMsg{seq} }),
	)
}

// notice shows a core message, with the key for its command if it has one.
func (m *Model) notice(n app.Notice) {
	text := n.Text
	if n.Unexpected {
		slog.Error(n.Text)
		if m.deps.LogPath != "" {
			text += " · details in " + m.deps.LogPath
		}
	}
	if n.Command != "" {
		key := ":" + n.Command
		// A command for the queue, such as paste, may not be bound here.
		if ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), n.Command); len(ks) > 0 {
			key = ks[0]
		} else if ks := m.engine.Table().Keys(keymap.Normal, []string{"queue", "global"}, n.Command); len(ks) > 0 {
			key = ks[0] + " in the queue"
		}
		text += " · " + key + " " + n.Hint
	}
	m.setMsg(text, n.Err)
}

// key handles a key press on the main screen.
func (m *Model) key(k tea.KeyPressMsg) tea.Cmd {
	if m.mode == keymap.Cmdline || m.mode == keymap.Search {
		return m.lineKey(k)
	}
	if m.popup != nil && m.popup.yes != nil {
		return m.confirmKey(k)
	}
	token := keymap.FromTerminal(k.String(), k.Text)
	if token == "\x00" { // Ctrl on Windows conhost arrives as NUL
		return nil
	}
	if m.popup != nil && m.popup.keyMenu != nil {
		if cmd, done := m.keyMenuKey(token); done {
			return cmd
		}
	}
	if q := m.offerSearch; q != "" {
		m.offerSearch = ""
		if token == "<CR>" && m.engine.Pending() == "" {
			return m.exec("search "+q, 0)
		}
	}
	return m.result(m.engine.Feed(m.mode, m.scopes(), token))
}

func (m *Model) result(r keymap.Result) tea.Cmd {
	switch r.Kind {
	case keymap.Run:
		if p := m.popup; p != nil && p.keyMenu != nil {
			if c, ok := m.registry.Get(
				keymap.Binding{Command: r.Command}.Name(),
			); !ok ||
				!c.Motion {
				m.closePopup() // a listed key runs where the menu was opened
			}
		}
		return m.execCall(
			r.Command,
			command.Call{Count: r.Count, Motion: r.Motion, Double: r.Double},
		)
	case keymap.Rejected:
		m.setMsg(r.Text, true)
	case keymap.Pending:
		if r.Ambiguous {
			seq := m.seq
			return m.clock.After(m.keys.timeout, func(time.Time) tea.Msg { return timeoutMsg{seq} })
		}
		return m.armPrefix()
	}
	return nil
}

// exec runs a command line from a binding or the ":" line.
func (m *Model) exec(line string, count int) tea.Cmd {
	return m.execCall(line, command.Call{Count: count})
}

// execCall runs a command line with an operator's count and motion.
func (m *Model) execCall(line string, with command.Call) tea.Cmd {
	m.pendingCmd = nil
	if call, err := command.Parse(line); err == nil && m.popup == nil {
		if c, ok := m.registry.Get(call.Name); ok && c.Jump {
			m.recordJump()
		}
	}
	err := m.registry.RunCall(m, line, with)
	m.checkVisual()
	if err != nil {
		m.setMsg(err.Error(), true)
		return nil
	}
	cmd := m.pendingCmd
	m.pendingCmd = nil
	return cmd
}

// lineKey handles a key on the ":" line or a prompt. Keys bound in the
// cmdline table run their line command and never type; others edit.
func (m *Model) lineKey(k tea.KeyPressMsg) tea.Cmd {
	key := "" // what the prompt's handler is told: enter, esc, up, …
	if a, ok := m.lineBound(k); ok {
		switch a {
		case "nop":
			return nil
		case "accept":
			key = "enter"
		case "cancel":
			key = "esc"
		case "complete":
			key = "tab"
		case "complete-back":
			key = "shift+tab"
		case "history-back":
			key = "up"
		case "history-forward":
			key = "down"
		default:
			m.editLine(a)
			return nil
		}
	} else if k.String() == "backspace" {
		key = "backspace"
	}
	var cmd tea.Cmd
	var done bool
	switch {
	case m.finding.prompting:
		cmd, done = m.findKey(key)
	case m.promptHelp:
		cmd, done = m.helpKey(key)
	case m.mode == keymap.Search:
		cmd, done = m.searchKey(key, k)
	default:
		cmd, done = m.cmdKey(key)
	}
	if done || key != "" && key != "backspace" {
		return cmd
	}
	return m.edit(k)
}

// edit passes a message to the line editor; in the search prompt, a
// changed text searches again.
func (m *Model) edit(msg tea.Msg) tea.Cmd {
	if _, paste := msg.(tea.PasteMsg); paste && m.selected {
		m.line.SetValue("") // a paste replaces the selected query too
	}
	before := m.line.Value()
	var cmd tea.Cmd
	m.line, cmd = m.line.Update(msg)
	m.lineChanged(before)
	return cmd
}

// lineChanged filters Help or searches again when a prompt's text changed.
func (m *Model) lineChanged(before string) {
	switch {
	case m.line.Value() == before, m.finding.prompting:
	case m.promptHelp:
		m.filterHelp(m.line.Value())
	case m.mode == keymap.Search:
		m.selected = false
		m.liveSearch(m.line.Value())
	}
}

func (m *Model) leaveLine() {
	m.mode = keymap.Normal
	m.selected, m.compl, m.promptHelp, m.finding.prompting = false, nil, false, false
	m.line.Reset()
	m.line.Blur()
}

// scopes is the scope chain for where the cursor is.
func (m *Model) scopes() []string {
	if m.popup != nil {
		if m.popup.keyMenu != nil {
			return m.popup.keyMenu
		}
		return []string{"popup", "global"}
	}
	if m.focus == queueWindow {
		return []string{"queue", "global"}
	}
	switch m.top().kind {
	case albumTracks:
		return []string{"album-tracks", "tracks", "global"}
	case albumList:
		return []string{"albums", "containers", "global"}
	case artistList, artistPage:
		return []string{"artists", "containers", "global"}
	case homeList:
		return []string{"home", "containers", "global"}
	case homeAlbums:
		return []string{"albums", "containers", "global"}
	case playlistList:
		return []string{"playlists", "containers", "global"}
	case playlistTracks:
		return []string{"playlist-tracks", "tracks", "global"}
	case helpList:
		return []string{"help", "global"}
	case searchList:
		v := m.top().search
		if _, ok := v.track(m.top().cur); ok {
			return []string{"search", "tracks", "global"}
		}
		if _, ok := v.album(m.top().cur); ok {
			return []string{"search", "albums", "containers", "global"}
		}
		if _, ok := v.artist(m.top().cur); ok {
			return []string{"search", "artists", "containers", "global"}
		}
		// A playlist's own keys (dd, J, K) belong to the Playlists section.
		if _, ok := v.playlist(m.top().cur); ok {
			return []string{"search", "containers", "global"}
		}
		return []string{"search", "containers", "global"}
	}
	return []string{"sections", "containers", "global"}
}

func (m *Model) top() *frame { return &m.lib[len(m.lib)-1] }

// rows is the number of rows in a list.
func (m *Model) rows(f *frame) int {
	lib := m.state.Library
	switch {
	case f.kind == sectionList:
		return len(sections)
	case f.kind == helpList && f.help != nil:
		return len(f.help.rows)
	case f.kind == homeList:
		return len(homeLists)
	case f.kind == homeAlbums:
		return len(m.albumsOf(f))
	case lib == nil:
		return 0
	case f.kind == artistList:
		return len(m.artistsOf(f))
	case f.kind == artistPage:
		return m.artistRows(f.artist).len()
	case f.kind == albumList:
		return len(m.albumsOf(f))
	case f.kind == albumTracks:
		return len(m.tracksOf(f))
	case f.kind == playlistList:
		return len(lib.Playlists)
	case f.kind == playlistTracks:
		return len(lib.PlaylistTracks(f.playlist))
	case f.kind == searchList && f.search != nil:
		return len(f.search.rows)
	}
	return 0
}

// cursorIDs remembers what each list's cursor is on, so a new snapshot
// can put the cursors back on the same items.
func (m *Model) cursorIDs() []string {
	ids := make([]string, len(m.lib))
	lib := m.state.Library
	if lib == nil {
		return ids
	}
	for i := range m.lib {
		if f := &m.lib[i]; f.cur < m.rows(f) {
			ids[i] = m.rowID(f, f.cur)
		}
	}
	return ids
}

// rowID is the ID of what row i of a list shows, or "".
func (m *Model) rowID(f *frame, i int) string {
	lib := m.state.Library
	switch f.kind {
	case homeList:
		return homeLists[i].kind
	case homeAlbums:
		return string(m.albumsOf(f)[i].ID)
	case artistList:
		return string(m.artistsOf(f)[i].ID)
	case artistPage:
		a, _ := m.artistRows(f.artist).album(i)
		return string(a.ID)
	case albumList:
		return string(m.albumsOf(f)[i].ID)
	case albumTracks:
		return string(m.tracksOf(f)[i].ID)
	case playlistList:
		return string(lib.Playlists[i].ID)
	case playlistTracks:
		return string(lib.PlaylistTracks(f.playlist)[i].ID)
	case searchList:
		return f.search.rowID(i)
	case helpList:
		r := f.help.rows[i]
		return r.heading + r.key + " " + r.line
	}
	return ""
}

func (m *Model) restoreCursors(ids []string) {
	lib := m.state.Library
	if lib == nil {
		return
	}
	for i := range m.lib {
		f := &m.lib[i]
		if i >= len(ids) || ids[i] == "" {
			continue
		}
		for j := range m.rows(f) {
			if m.rowID(f, j) == ids[i] {
				f.cur = j
				break
			}
		}
	}
	m.clamp()
}

// clamp keeps cursors inside lists that changed length.
func (m *Model) clamp() {
	for i := range m.lib {
		f := &m.lib[i]
		f.cur = min(f.cur, max(m.rows(f)-1, 0))
		m.settle(f, 1)
	}
	m.queueCur = min(m.queueCur, max(len(m.state.Queue)-1, 0))
}

// bodyHeight is the number of list rows each window shows.
func (m *Model) bodyHeight() int {
	h := m.h - 5 // title row, the rule, now-playing bar (2), status line
	if m.hints {
		h--
	}
	return max(h, 1)
}

func seconds(arg string) (time.Duration, error) {
	n, err := strconv.Atoi(strings.TrimPrefix(arg, "+"))
	if err != nil {
		return 0, fmt.Errorf("seek: %q is not a number of seconds", arg)
	}
	return time.Duration(n) * time.Second, nil
}
