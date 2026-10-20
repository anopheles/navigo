package player

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/mpv"
)

// ErrMPVTooOld is returned when mpv is older than 0.38, the first version
// with the named-argument loadfile navigo relies on.
var ErrMPVTooOld = errors.New("mpv 0.38 or newer is needed")

// ErrNoMediaControls is a warning from Check on macOS and Windows: mpv
// plays, but its media integration (Now Playing, SMTC) needs 0.39.
var ErrNoMediaControls = errors.New("the OS's media controls need mpv 0.39 or newer")

// ErrMPVMissing is returned when there is no mpv to run.
var ErrMPVMissing = errors.New("mpv not found")

// MPVOptions configure the mpv adapter.
type MPVOptions struct {
	// Path and Args are passed to mpv.Start; Args is player.mpv_args.
	Path string
	Args []string
	// StreamURL signs a stream URL for a track, with a fresh salt each
	// call. It is called whenever an entry is loaded into mpv.
	StreamURL func(library.TrackID) string
}

// Observer IDs for the properties MPV follows.
const (
	obsPause int64 = iota + 1
	obsTimePos
	obsDuration
	obsVolume
	obsCoreIdle
)

var observed = map[int64]string{
	obsPause:    "pause",
	obsTimePos:  "time-pos",
	obsDuration: "duration",
	obsVolume:   "volume",
	obsCoreIdle: "core-idle",
}

// MPV is the Player backed by an mpv process. mpv is started on the first
// Play and again on the Play after it died.
type MPV struct {
	opts    MPVOptions
	events  chan Event
	closing chan struct{}
	loops   sync.WaitGroup
	once    sync.Once

	op sync.Mutex // serialises changes to mpv's playlist

	mu      sync.Mutex
	proc    *mpv.Process
	entries map[int64]Entry // mpv playlist_entry_id → entry
	// live holds the IDs still in mpv's playlist. Events from entries a
	// later Play replaced are stale (mpv may have run on to them while a
	// stream failed) and must not start anything.
	live      map[int64]bool
	current   int64 // mpv ID of the playing entry, 0 if none
	paused    bool
	coreIdle  bool
	buffering bool
	pos       time.Duration
	duration  time.Duration
	lastSec   int64
	volume    int
	// wantVolume is the volume last asked for, set on every mpv started,
	// or -1 to leave mpv's own.
	wantVolume int
}

var _ Player = (*MPV)(nil)

// NewMPV returns an mpv-backed Player. It starts no process yet.
func NewMPV(opts MPVOptions) *MPV {
	return &MPV{
		opts:    opts,
		events:  make(chan Event, 64),
		closing: make(chan struct{}),
		entries: map[int64]Entry{},
		live:    map[int64]bool{},
		volume:  -1,

		wantVolume: -1,
	}
}

func (m *MPV) Events() <-chan Event { return m.events }

func (m *MPV) Play(ctx context.Context, w Span) error {
	m.op.Lock()
	defer m.op.Unlock()
	p, err := m.ensure(ctx)
	if err != nil {
		return err
	}
	if err := p.Set(ctx, "pause", false); err != nil {
		return err
	}
	m.mu.Lock()
	m.live = map[int64]bool{}
	m.mu.Unlock()
	if _, err := m.loadAt(ctx, p, w.Current, "replace", nil, w.Start); err != nil {
		return err
	}
	if w.Next.ID != 0 {
		if _, err := m.load(ctx, p, w.Next, "append", nil); err != nil {
			return err
		}
	}
	if w.Prev.ID != 0 {
		if _, err := m.load(ctx, p, w.Prev, "insert-at", 0); err != nil {
			return err
		}
	}
	return nil
}

func (m *MPV) SetNeighbours(ctx context.Context, prev, next Entry) error {
	m.op.Lock()
	defer m.op.Unlock()
	p := m.process()
	if p == nil {
		return nil
	}
	return m.reconcile(ctx, p, &prev, &next)
}

func (m *MPV) Pause(ctx context.Context) error  { return m.set(ctx, "pause", true) }
func (m *MPV) Resume(ctx context.Context) error { return m.set(ctx, "pause", false) }

// SetVolume sets the volume now if mpv runs, and on any mpv started
// later, so a volume chosen before the first track still applies.
func (m *MPV) SetVolume(ctx context.Context, percent int) error {
	v := min(max(percent, 0), 100)
	m.mu.Lock()
	m.wantVolume = v
	m.mu.Unlock()
	return m.set(ctx, "volume", v)
}

func (m *MPV) SeekTo(ctx context.Context, pos time.Duration) error {
	return m.command(ctx, "seek", pos.Seconds(), "absolute")
}

func (m *MPV) SeekBy(ctx context.Context, d time.Duration) error {
	return m.command(ctx, "seek", d.Seconds(), "relative")
}

func (m *MPV) Stop(ctx context.Context) error {
	m.op.Lock()
	defer m.op.Unlock()
	m.mu.Lock()
	m.live = map[int64]bool{}
	m.mu.Unlock()
	return m.command(ctx, "stop")
}

// Close quits mpv and closes Events.
func (m *MPV) Close() error {
	var err error
	m.once.Do(func() {
		close(m.closing)
		if p := m.process(); p != nil {
			err = p.Close()
		}
		m.loops.Wait()
		close(m.events)
	})
	return err
}

func (m *MPV) process() *mpv.Process {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.proc
}

func (m *MPV) set(ctx context.Context, property string, value any) error {
	p := m.process()
	if p == nil {
		return nil
	}
	return p.Set(ctx, property, value)
}

func (m *MPV) command(ctx context.Context, args ...any) error {
	p := m.process()
	if p == nil {
		return nil
	}
	_, err := p.Command(ctx, args...)
	return err
}

// Check runs mpv --version: ErrMPVMissing if there is no mpv at the path
// (or on PATH), ErrMPVTooOld if it is older than 0.38.
func (m *MPV) Check(ctx context.Context) error {
	path := m.opts.Path
	if path == "" {
		path = "mpv"
	}
	bin, err := exec.LookPath(path)
	if err != nil {
		return fmt.Errorf("%w at %s", ErrMPVMissing, path)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--no-config", "--version").Output()
	if err != nil {
		return fmt.Errorf("%s --version: %w", bin, err)
	}
	version, _, _ := strings.Cut(string(out), "\n")
	version, _, _ = strings.Cut(version, " Copyright")
	version = strings.TrimSpace(version)
	if !versionOK(version) {
		return fmt.Errorf("%w, found %s", ErrMPVTooOld, version)
	}
	if mediaByMPV && !atLeast(version, 0, 39) {
		return fmt.Errorf("%w; found %s", ErrNoMediaControls, version)
	}
	return nil
}

// mediaByMPV: on macOS and Windows, mpv provides the OS's media controls.
var mediaByMPV = runtime.GOOS == "darwin" || runtime.GOOS == "windows"

// ensure returns the running mpv, starting it if needed.
func (m *MPV) ensure(ctx context.Context) (*mpv.Process, error) {
	if p := m.process(); p != nil {
		return p, nil
	}
	select {
	case <-m.closing:
		return nil, errors.New("player: closed")
	default:
	}
	p, err := mpv.Start(ctx, mpv.Options{Path: m.opts.Path, Args: m.opts.Args})
	if err != nil {
		return nil, err
	}
	var version string
	if err := p.Get(ctx, "mpv-version", &version); err != nil {
		_ = p.Close()
		return nil, err
	}
	if !versionOK(version) {
		_ = p.Close()
		return nil, fmt.Errorf("%w, found %s", ErrMPVTooOld, version)
	}
	m.mu.Lock()
	want := m.wantVolume
	m.mu.Unlock()
	if want >= 0 {
		if err := p.Set(ctx, "volume", want); err != nil {
			_ = p.Close()
			return nil, err
		}
	}
	for id, name := range observed {
		if err := p.Observe(ctx, id, name); err != nil {
			_ = p.Close()
			return nil, err
		}
	}
	m.mu.Lock()
	m.proc = p
	m.mu.Unlock()
	m.loops.Add(1)
	go m.loop(p)
	return p, nil
}

var versionRE = regexp.MustCompile(`(\d+)\.(\d+)`)

// versionOK reports whether an mpv-version such as "mpv v0.41.0" or
// "mpv 0.38.0-45-g1234abc" is at least 0.38. Unknown formats pass.
func versionOK(v string) bool { return atLeast(v, 0, 38) }

// atLeast reports whether an mpv version string is at least major.minor.
// Unknown formats pass.
func atLeast(v string, major, minor int) bool {
	m := versionRE.FindStringSubmatch(v)
	if m == nil {
		return true
	}
	ma, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	return ma > major || ma == major && mi >= minor
}

// load appends e's stream to mpv's playlist with the given loadfile flags
// and returns its mpv ID.
func (m *MPV) load(
	ctx context.Context,
	p *mpv.Process,
	e Entry,
	flags string,
	index any,
) (int64, error) {
	return m.loadAt(ctx, p, e, flags, index, 0)
}

// loadAt is load starting at a position. Each file also gets its title
// (force-media-title) for mpv's Now Playing on macOS and SMTC on Windows.
func (m *MPV) loadAt(
	ctx context.Context,
	p *mpv.Process,
	e Entry,
	flags string,
	index any,
	start time.Duration,
) (int64, error) {
	args := map[string]any{"url": m.opts.StreamURL(e.Track), "flags": flags}
	if index != nil {
		args["index"] = index
	}
	var opts []string
	if start > 0 {
		opts = append(opts, fmt.Sprintf("start=%.3f", start.Seconds()))
	}
	if e.Title != "" {
		// %N% gives the value's length in bytes, so commas in it are safe.
		opts = append(opts, fmt.Sprintf("force-media-title=%%%d%%%s", len(e.Title), e.Title))
	}
	if len(opts) > 0 {
		if index == nil {
			args["index"] = -1 // options come after the index
		}
		args["options"] = strings.Join(opts, ",")
	}
	data, err := p.Named(ctx, "loadfile", args)
	if err != nil {
		return 0, fmt.Errorf("player: load %s: %w", e.Track, err)
	}
	var r struct {
		ID int64 `json:"playlist_entry_id"`
	}
	if err := json.Unmarshal(data, &r); err != nil || r.ID == 0 {
		return 0, fmt.Errorf("player: load %s: no playlist entry ID in %s", e.Track, data)
	}
	m.mu.Lock()
	m.entries[r.ID] = e
	m.live[r.ID] = true
	m.mu.Unlock()
	return r.ID, nil
}

// reconcile makes mpv's playlist the span around the playing entry.
// prev and next are the wanted neighbours (a zero Entry for none); nil
// keeps whatever neighbour is there. Entries outside the span are always
// removed. A neighbour that is already the wanted entry stays, so its
// prefetched stream isn't thrown away.
func (m *MPV) reconcile(ctx context.Context, p *mpv.Process, prev, next *Entry) error {
	var list []struct {
		ID      int64 `json:"id"`
		Current bool  `json:"current"`
	}
	if err := p.Get(ctx, "playlist", &list); err != nil {
		return err
	}
	m.mu.Lock()
	cur := m.current
	entries := m.entries
	pos := -1
	for i, e := range list {
		if (cur != 0 && e.ID == cur) || (cur == 0 && e.Current) {
			pos = i
		}
	}
	m.mu.Unlock()
	if pos < 0 {
		return nil
	}

	wanted := func(i int, want *Entry) bool {
		if want == nil {
			return true
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		return want.ID != 0 && entries[list[i].ID].ID == want.ID
	}
	keepPrev := pos > 0 && wanted(pos-1, prev)
	keepNext := pos+1 < len(list) && wanted(pos+1, next)
	for i := len(list) - 1; i >= 0; i-- {
		keep := i == pos || (i == pos-1 && keepPrev) || (i == pos+1 && keepNext)
		if keep {
			continue
		}
		if _, err := p.Command(ctx, "playlist-remove", i); err != nil {
			return err
		}
		m.mu.Lock()
		delete(m.live, list[i].ID)
		m.mu.Unlock()
	}
	if next != nil && next.ID != 0 && !keepNext {
		if _, err := m.load(ctx, p, *next, "append", nil); err != nil {
			return err
		}
	}
	if prev != nil && prev.ID != 0 && !keepPrev {
		if _, err := m.load(ctx, p, *prev, "insert-at", 0); err != nil {
			return err
		}
	}
	return nil
}

// loop turns one mpv process's events into Player events until it ends.
func (m *MPV) loop(p *mpv.Process) {
	defer m.loops.Done()
	for ev := range p.Events() {
		m.handle(p, ev)
	}
	select {
	case <-m.closing:
		return
	default:
	}
	_ = p.Close()
	// A new mpv numbers its entries from 1 again.
	m.mu.Lock()
	m.proc = nil
	m.current = 0
	m.entries = map[int64]Entry{}
	m.live = map[int64]bool{}
	m.paused, m.coreIdle, m.buffering, m.volume = false, false, false, -1
	m.mu.Unlock()
	err := p.Err()
	if errors.Is(err, mpv.ErrClosed) {
		err = nil
	}
	m.emit(Died{Err: fmt.Errorf("mpv stopped: %v", err)})
}

func (m *MPV) handle(p *mpv.Process, ev mpv.Event) {
	switch ev.Name {
	case "start-file":
		m.settle()
		m.mu.Lock()
		e, ok := m.entries[ev.PlaylistEntryID]
		ok = ok && m.live[ev.PlaylistEntryID]
		if ok {
			// duration stays: mpv reports it only when it changes, so
			// the next track of the same length sends nothing.
			m.current = ev.PlaylistEntryID
			m.pos = 0
			m.lastSec = -1
		}
		m.mu.Unlock()
		if !ok {
			return
		}
		m.emit(TrackStarted{Entry: e.ID})
		// Drop what fell out of the span; the core sends new
		// neighbours in reply to TrackStarted.
		m.op.Lock()
		_ = m.reconcile(context.Background(), p, nil, nil)
		m.op.Unlock()

	case "end-file":
		m.settle()
		m.mu.Lock()
		e, ok := m.entries[ev.PlaylistEntryID]
		if ev.PlaylistEntryID == m.current {
			m.current = 0
		}
		m.mu.Unlock()
		if !ok {
			return
		}
		ended := TrackEnded{Entry: e.ID, Reason: Stopped}
		switch ev.Reason {
		case "eof":
			ended.Reason = Finished
		case "error":
			ended.Reason, ended.Err = Failed, ev.FileError
		}
		m.emit(ended)

	case "seek", "playback-restart":
		m.mu.Lock()
		m.lastSec = -1
		m.mu.Unlock()

	case "property-change":
		m.property(ev)
	}
}

// settle waits for a playlist change in progress to finish: mpv can send
// start-file before the loadfile reply that tells us the entry's ID.
func (m *MPV) settle() {
	m.op.Lock()
	m.op.Unlock()
}

func (m *MPV) property(ev mpv.Event) {
	var num float64
	var flag bool
	switch ev.ID {
	case obsPause, obsCoreIdle:
		if json.Unmarshal(ev.Data, &flag) != nil {
			return
		}
	default:
		if ev.Data == nil || json.Unmarshal(ev.Data, &num) != nil {
			num = math.NaN()
		}
	}

	var out []Event
	m.mu.Lock()
	switch ev.ID {
	case obsPause:
		m.paused = flag
		out = append(out, Paused{Paused: flag})
	case obsCoreIdle:
		m.coreIdle = flag
	case obsDuration:
		m.duration = 0
		if !math.IsNaN(num) {
			m.duration = seconds(num)
			// Positions sent before the duration was known lack it.
			if e, ok := m.entries[m.current]; ok && m.current != 0 {
				out = append(out, Position{Entry: e.ID, Position: m.pos, Duration: m.duration})
			}
		}
	case obsVolume:
		if v := int(math.Round(num)); !math.IsNaN(num) && v != m.volume {
			m.volume = v
			out = append(out, Volume{Percent: v})
		}
	case obsTimePos:
		if e, ok := m.entries[m.current]; ok && m.current != 0 && !math.IsNaN(num) {
			m.pos = seconds(num)
			if sec := int64(num); sec != m.lastSec {
				m.lastSec = sec
				out = append(out, Position{Entry: e.ID, Position: m.pos, Duration: m.duration})
			}
		}
	}
	if b := m.coreIdle && !m.paused && m.current != 0; b != m.buffering {
		m.buffering = b
		out = append(out, Buffering{Buffering: b})
	}
	m.mu.Unlock()
	for _, e := range out {
		m.emit(e)
	}
}

func (m *MPV) emit(e Event) {
	select {
	case m.events <- e:
	case <-m.closing:
	}
}

func seconds(s float64) time.Duration {
	return time.Duration(s * float64(time.Second))
}
