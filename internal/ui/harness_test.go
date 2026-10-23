package ui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/zalando/go-keyring"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// harness runs the real model, core and runner in-process, with the real
// subsonic client against the fake server and the fake Player (layer A).
// Commands run synchronously, so every step settles before the next.
type harness struct {
	t      *testing.T
	m      *Model
	srv    *subsonictest.Server
	fake   *player.Fake
	kr     memKeyring
	path   string
	quit   bool
	logins int
	cache  string
	state  string
	// queued holds messages sent from inside a command (sync progress);
	// they are delivered once that command returns.
	queued   []tea.Msg
	progress []string // the empty-list text seen while syncing
	// midSync, when set, runs after each sync progress message.
	midSync func(*harness)
	clock   *fakeClock
	raw     []string // what was written straight to the terminal
}

// fakeClock records timers instead of sleeping; advance fires them.
type fakeClock struct {
	now    time.Time
	timers []timer
}

type timer struct {
	at time.Time
	fn func(time.Time) tea.Msg
}

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) After(d time.Duration, fn func(time.Time) tea.Msg) tea.Cmd {
	c.timers = append(c.timers, timer{c.now.Add(d), fn})
	return nil
}

// advance moves the clock on by d and delivers the timers that fall due.
func (h *harness) advance(d time.Duration) {
	h.t.Helper()
	h.clock.now = h.clock.now.Add(d)
	for {
		i := slices.IndexFunc(
			h.clock.timers,
			func(t timer) bool { return !t.at.After(h.clock.now) },
		)
		if i < 0 {
			return
		}
		t := h.clock.timers[i]
		h.clock.timers = slices.Delete(h.clock.timers, i, i+1)
		h.send(t.fn(t.at))
	}
}

type memKeyring map[string]string

func (k memKeyring) Get(service, key string) (string, error) {
	if p, ok := k[service+"/"+key]; ok {
		return p, nil
	}
	return "", keyring.ErrNotFound
}

func (k memKeyring) Delete(service, key string) error {
	if _, ok := k[service+"/"+key]; !ok {
		return keyring.ErrNotFound
	}
	delete(k, service+"/"+key)
	return nil
}

func (k memKeyring) Set(service, key, secret string) error {
	k[service+"/"+key] = secret
	return nil
}

// newHarness starts navigo at w×h and, once the main screen shows, goes
// to Albums, where most tests begin. edit, if set, changes the config,
// which by default logs in to the fake server with a password in it.
func newHarness(t *testing.T, w, h int, edit func(*config.Config, *subsonictest.Server)) *harness {
	t.Helper()
	hs := homeHarness(t, w, h, edit)
	hs.toAlbums()
	return hs
}

// toAlbums goes to the Albums section unless the login form shows.
func (h *harness) toAlbums() {
	h.t.Helper()
	if h.m.screen == screenMain {
		h.keys("'b")
	}
}

// homeHarness starts navigo at w×h and leaves it where it starts: Home.
func homeHarness(t *testing.T, w, h int, edit func(*config.Config, *subsonictest.Server)) *harness {
	t.Helper()
	return setupHarness(t, w, h, edit, nil)
}

// setupHarness is homeHarness with setup run before Init, such as to make
// the fake player's startup check fail.
func setupHarness(
	t *testing.T,
	w, h int,
	edit func(*config.Config, *subsonictest.Server),
	setup func(*harness),
) *harness {
	t.Helper()
	srv := subsonictest.NewServer(t)
	cfg := config.Config{
		SyncQueue: true,
		Server: config.Server{
			URL:      srv.URL,
			User:     subsonictest.Username,
			Password: subsonictest.Password,
		},
	}
	if edit != nil {
		edit(&cfg, srv)
	}
	hs := &harness{
		t:    t,
		srv:  srv,
		fake: player.NewFake(),
		kr:   memKeyring{},
		path: filepath.Join(t.TempDir(), "navigo", "config.yaml"),
	}
	hs.cache = filepath.Join(filepath.Dir(hs.path), "cache")
	hs.state = filepath.Join(filepath.Dir(hs.path), "state")
	m, err := New(
		Deps{
			Config:     cfg,
			ConfigPath: hs.path,
			Player:     hs.fake,
			Keyring:    hs.kr,
			CacheDir:   hs.cache,
			StateDir:   hs.state,
			LogPath:    "/logs/navigo.log",
			OnLogin:    func(*subsonic.Client) { hs.logins++ },
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	hs.m = m
	hs.m.SetSend(func(msg tea.Msg) { hs.queued = append(hs.queued, msg) })
	hs.clock = &fakeClock{now: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)}
	hs.m.clock = hs.clock
	hs.send(tea.WindowSizeMsg{Width: w, Height: h})
	if setup != nil {
		setup(hs)
	}
	hs.run(hs.m.Init())
	return hs
}

// restart starts navigo again on the same server, config and cache, as a
// second launch would.
func (h *harness) restart() *harness {
	h.t.Helper()
	return h.launch(h.cache, h.state)
}

// otherDevice starts navigo on the same server, user and config, but with
// a cache and state directory of its own, as on a second computer.
func (h *harness) otherDevice() *harness {
	h.t.Helper()
	return h.launch(h.t.TempDir(), h.t.TempDir())
}

// launch starts navigo with the given cache and state directories.
func (h *harness) launch(cache, state string) *harness {
	h.t.Helper()
	n := &harness{
		t:     h.t,
		srv:   h.srv,
		fake:  player.NewFake(),
		kr:    h.kr,
		path:  h.path,
		cache: cache,
		state: state,
	}
	m, err := New(
		Deps{
			Config:     h.m.deps.Config,
			ConfigPath: h.path,
			Player:     n.fake,
			Keyring:    n.kr,
			CacheDir:   n.cache,
			StateDir:   n.state,
			OnLogin:    func(*subsonic.Client) { n.logins++ },
		},
	)
	if err != nil {
		h.t.Fatal(err)
	}
	n.m = m
	n.m.SetSend(func(msg tea.Msg) { n.queued = append(n.queued, msg) })
	n.clock = &fakeClock{now: h.clock.now}
	n.m.clock = n.clock
	n.send(tea.WindowSizeMsg{Width: h.m.w, Height: h.m.h})
	n.run(n.m.Init())
	n.toAlbums()
	return n
}

// requests counts the fake server's requests to endpoint so far.
func (h *harness) requests(endpoint string) int {
	n := 0
	for _, r := range h.srv.Requests() {
		if r.Endpoint == endpoint {
			n++
		}
	}
	return n
}

func (h *harness) send(msg tea.Msg) {
	h.t.Helper()
	_, cmd := h.m.Update(msg)
	h.run(cmd)
	h.pump()
}

// run executes a command and feeds its messages back in, depth first.
func (h *harness) run(cmd tea.Cmd) {
	h.t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	for len(h.queued) > 0 {
		q := h.queued[0]
		h.queued = h.queued[1:]
		h.send(q)
		h.progress = append(h.progress, h.m.emptyList())
		if h.midSync != nil {
			h.midSync(h)
		}
	}
	switch msg := msg.(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	case tea.QuitMsg:
		h.quit = true
	case tea.RawMsg: // what the program would write to the terminal as it is
		h.raw = append(h.raw, fmt.Sprint(msg.Msg))
	default:
		h.send(msg)
	}
}

// pump delivers the fake player's events, as cmd/navigo's forwarder does.
func (h *harness) pump() {
	h.t.Helper()
	for {
		select {
		case e := <-h.fake.Events():
			h.send(PlayerMsg(e))
		default:
			return
		}
	}
}

// keys types a key sequence in vim notation, such as "3j" or ":q<CR>".
func (h *harness) keys(seq string) {
	h.t.Helper()
	tokens, err := keymap.Parse(seq)
	if err != nil {
		h.t.Fatal(err)
	}
	for _, tok := range tokens {
		h.send(tea.KeyPressMsg(keyFor(tok)))
	}
}

// typeText types literal text, as into the login form.
func (h *harness) typeText(s string) {
	h.t.Helper()
	for _, r := range s {
		h.send(tea.KeyPressMsg(tea.Key{Code: r, Text: string(r)}))
	}
}

func keyFor(tok string) tea.Key {
	switch tok {
	case "<CR>":
		return tea.Key{Code: tea.KeyEnter}
	case "<Esc>":
		return tea.Key{Code: tea.KeyEscape}
	case "<Tab>":
		return tea.Key{Code: tea.KeyTab}
	case "<S-Tab>":
		return tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}
	case "<Space>":
		return tea.Key{Code: tea.KeySpace, Text: " "}
	case "<BS>":
		return tea.Key{Code: tea.KeyBackspace}
	case "<Up>":
		return tea.Key{Code: tea.KeyUp}
	case "<Down>":
		return tea.Key{Code: tea.KeyDown}
	case "<lt>":
		return tea.Key{Code: '<', Text: "<"}
	case "<F1>":
		return tea.Key{Code: tea.KeyF1}
	}
	if strings.HasPrefix(tok, "<C-") {
		return tea.Key{Code: []rune(tok)[3], Mod: tea.ModCtrl}
	}
	r := []rune(tok)[0]
	return tea.Key{Code: r, Text: tok}
}

// screen is the rendered screen without styles.
func (h *harness) screen() string { return ansi.Strip(h.m.render()) }

func (h *harness) contains(want ...string) {
	h.t.Helper()
	s := h.screen()
	for _, w := range want {
		if !strings.Contains(s, w) {
			h.t.Errorf("screen lacks %q:\n%s", w, s)
		}
	}
}

func (h *harness) lacks(unwanted ...string) {
	h.t.Helper()
	s := h.screen()
	for _, w := range unwanted {
		if strings.Contains(s, w) {
			h.t.Errorf("screen has %q:\n%s", w, s)
		}
	}
}

// cursorRow is the text of the focused list's cursor row.
func (h *harness) cursorRow() string {
	f := h.m.top()
	if h.m.focus == queueWindow {
		return h.m.state.Queue[h.m.queueCur].Track.Title
	}
	return ansi.Strip(h.m.libraryRow(f, f.cur, 200))
}

func appSynced(s *library.Snapshot) app.Event { return app.Synced{Library: s} }
