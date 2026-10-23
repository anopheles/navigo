package ui

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/resume"
	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// titles are the queue's track titles, in order.
func (h *harness) titles() []string {
	var out []string
	for _, q := range h.m.state.Queue {
		out = append(out, q.Track.Title)
	}
	return out
}

// serverQueue is the play queue the fake server holds for the user.
func (h *harness) serverQueue() (subsonictest.PlayQueue, bool) {
	return h.srv.PlayQueue(subsonictest.Username)
}

// playsThirdPausedAt2m14 plays the album from its third row, paused 134 s into it.
func (h *harness) playsThirdPausedAt2m14() {
	h.t.Helper()
	h.playAlbum()
	h.keys("jj<CR>")
	h.fake.Tick(134 * time.Second)
	h.pump()
	h.keys(" ")
	h.advance(saveAfter)
}

func TestAnotherDeviceRestoresTheQueuePausedAtThePosition(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playsThirdPausedAt2m14()
	want := a.titles()
	current := a.m.state.Queue[a.m.state.Current].Track

	b := a.otherDevice()
	b.contains("restored 6 tracks", "plays from 2:14")
	if got := b.titles(); !slices.Equal(got, want) {
		t.Errorf("queue = %q, want %q", got, want)
	}
	for _, c := range b.fake.Calls() {
		if len(c) >= 4 && c[:4] == "Play" {
			t.Errorf("starting played something: %v", b.fake.Calls())
		}
	}
	b.keys(" ")
	sp := b.fake.Span()
	if !strings.HasPrefix(sp.Current.Title, current.Title) || sp.Start != 134*time.Second {
		t.Errorf("<Space> played %q from %v, want %q from 2m14s", sp.Current.Title, sp.Start,
			current.Title)
	}
}

func TestRestoringTheServersQueueDoesNotSendItBack(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playsThirdPausedAt2m14()
	saves := a.requests("savePlayQueue")

	b := a.otherDevice()
	b.contains("restored 6 tracks")
	b.advance(saveAfter)
	if got := b.requests("savePlayQueue"); got != saves {
		t.Errorf("savePlayQueue requests went from %d to %d", saves, got)
	}
	if _, err := b.savedFile(); err != nil {
		t.Errorf("the device keeps no file of the restored queue: %v", err)
	}
}

func TestTheServersQueueWinsOverTheLocalFile(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playAlbum()
	a.advance(saveAfter)
	local := a.titles()

	q, _ := a.serverQueue()
	ids := slices.Clone(q.IDs)
	slices.Reverse(ids)
	a.srv.SetPlayQueue(subsonictest.Username, subsonictest.PlayQueue{IDs: ids, Current: ids[0]})

	n := a.restart()
	n.contains("restored 6 tracks")
	want := slices.Clone(local)
	slices.Reverse(want)
	if got := n.titles(); !slices.Equal(got, want) {
		t.Errorf("queue = %q, want the server's %q", got, want)
	}
}

func TestVolumeAndRepeatStayOnTheDevice(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playAlbum()
	a.keys(":volume 40<CR>:repeat<CR>")
	a.advance(saveAfter)
	for _, r := range a.srv.Requests() {
		if r.Endpoint != "savePlayQueue" {
			continue
		}
		for k := range r.Params {
			if !slices.Contains([]string{"id", "current", "position"}, k) {
				t.Errorf("savePlayQueue sent %q, which is not the queue", k)
			}
		}
	}

	b := a.otherDevice()
	b.contains("restored 6 tracks")
	if v, r := b.m.state.Player.Volume, b.m.state.Repeat; v != 100 || r != app.RepeatOff {
		t.Errorf("a new device took volume %d and repeat %v from elsewhere", v, r)
	}
	b.keys(":volume 70<CR>:repeat<CR>:repeat<CR>")
	b.advance(saveAfter)

	// The queue comes from the server and the settings from this device.
	q, _ := a.serverQueue()
	q.IDs = q.IDs[:3]
	q.Current = q.IDs[0]
	a.srv.SetPlayQueue(subsonictest.Username, q)
	c := b.restart()
	c.contains("restored 3 tracks")
	if v, r := c.m.state.Player.Volume, c.m.state.Repeat; v != 70 || r != app.RepeatOne {
		t.Errorf("volume %d, repeat %v, want 70 and repeat one from this device's file", v, r)
	}
}

func TestServerTroubleFallsBackToTheLocalFile(t *testing.T) {
	old := serverQueueTimeout
	serverQueueTimeout = 100 * time.Millisecond
	t.Cleanup(func() { serverQueueTimeout = old })

	tests := []struct {
		name   string
		damage func(*harness)
	}{
		{"the server doesn't know the call", func(h *harness) {
			h.srv.SetPlayQueueFault(subsonictest.PlayQueueMissing)
		}},
		{"the server errors", func(h *harness) {
			h.srv.SetPlayQueueFault(subsonictest.PlayQueueBroken)
		}},
		{"the server is too slow", func(h *harness) {
			h.srv.SetPlayQueueFault(subsonictest.PlayQueueHangs)
		}},
		{"the server is unreachable", func(h *harness) { h.srv.SetDown(true) }},
		{"the server's queue is empty", func(h *harness) {
			h.srv.SetPlayQueue(subsonictest.Username, subsonictest.PlayQueue{})
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := newHarness(t, 120, 40, nil)
			a.playAlbum()
			a.keys("jj<CR>")
			a.advance(saveAfter)
			local := a.titles()

			// The server holds another queue, which is not reachable.
			q, _ := a.serverQueue()
			ids := slices.Clone(q.IDs)
			slices.Reverse(ids)
			a.srv.SetPlayQueue(
				subsonictest.Username,
				subsonictest.PlayQueue{IDs: ids, Current: ids[0]},
			)
			tt.damage(a)

			n := a.restart()
			n.contains("restored 6 tracks")
			if got := n.titles(); !slices.Equal(got, local) {
				t.Errorf("queue = %q, want the local %q", got, local)
			}
		})
	}
}

func TestResumeOffNeverTouchesTheServersQueue(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		c.Resume.Off = true
		srv.SetPlayQueue(
			subsonictest.Username,
			subsonictest.PlayQueue{IDs: []string{"x", "y"}, Current: "x"},
		)
	})
	h.playAlbum()
	h.advance(saveAfter)
	h.m.FlushSaved()
	h.lacks("restored")
	if n := h.requests("getPlayQueue") + h.requests("savePlayQueue"); n != 0 {
		t.Errorf("%d play queue requests with resume off", n)
	}
	if q, ok := h.serverQueue(); !ok || len(q.IDs) != 2 {
		t.Errorf("the server's queue changed: %+v", q)
	}
}

func TestLogoutLeavesTheServersQueueAlone(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	if _, ok := h.serverQueue(); !ok {
		t.Fatal("the server has no queue to keep")
	}
	h.keys(":logout<CR>")
	h.advance(saveAfter)
	if _, err := h.savedFile(); err == nil {
		t.Error(":logout left this device's file")
	}
	if q, ok := h.serverQueue(); !ok || len(q.IDs) != 6 {
		t.Errorf("after :logout the server holds %+v, want the 6 tracks", q)
	}
}

func TestTurningResumeOffLeavesTheServersQueueAlone(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte("resume: false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.keys(":reload-config<CR>")
	if _, err := h.savedFile(); err == nil {
		t.Error("this device's file stayed")
	}
	if q, ok := h.serverQueue(); !ok || len(q.IDs) != 6 {
		t.Errorf("the server holds %+v, want the 6 tracks", q)
	}
}

func TestClearingTheQueueClearsTheServersToo(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	h.keys(":queue-clear<CR>")
	if q, ok := h.serverQueue(); ok {
		t.Errorf("the server still holds %+v", q)
	}
}

func TestARunningSessionIsNotOverwrittenByAnotherDevice(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playAlbum()
	a.advance(saveAfter)
	b := a.otherDevice()
	b.contains("restored 6 tracks")
	want := b.titles()
	reads := b.requests("getPlayQueue")

	a.keys(":queue-clear<CR>")
	b.advance(10 * time.Minute)
	if got := b.titles(); !slices.Equal(got, want) {
		t.Errorf("the running queue changed to %q", got)
	}
	if got := b.requests("getPlayQueue"); got != reads {
		t.Errorf("getPlayQueue requests went from %d to %d while running", reads, got)
	}
}

func TestAFailedPushIsQuiet(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.srv.SetPlayQueueFault(subsonictest.PlayQueueBroken)
	h.playAlbum()
	h.advance(saveAfter)
	h.lacks("couldn't", "offline")
	if h.m.state.Offline {
		t.Error("a failed push made navigo go offline")
	}
	if s, err := h.savedFile(); err != nil || len(s.Tracks) == 0 {
		t.Errorf("the local file wasn't written: %+v, %v", s, err)
	}
}

func TestAPushThatWasOvertakenDoesNotReplaceANewerOne(t *testing.T) {
	srv := subsonictest.NewServer(t)
	c := subsonic.New(subsonic.Credentials{
		URL: srv.URL, Username: subsonictest.Username, Password: subsonictest.Password,
	})
	saved := func(id string) resume.Saved {
		return resume.Saved{Tracks: []library.TrackID{library.TrackID(id)}, Current: 0}
	}
	ctx := context.Background()
	p := newQueuePush()
	p.allow()
	p.send(ctx, c, 2, saved("newer"), false)
	p.send(ctx, c, 1, saved("older"), false)
	if q, _ := srv.PlayQueue(subsonictest.Username); !slices.Equal(q.IDs, []string{"newer"}) {
		t.Errorf("the server holds %q, want the newer write", q.IDs)
	}
	p.send(ctx, c, 3, saved("newer"), false)
	n := 0
	for _, r := range srv.Requests() {
		if r.Endpoint == "savePlayQueue" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d savePlayQueue requests, want 1: the server already held that queue", n)
	}
}

func TestWithoutSyncQueueTheServerIsNeverAsked(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, srv *subsonictest.Server) {
		c.SyncQueue.On = false
		srv.SetPlayQueue(
			subsonictest.Username,
			subsonictest.PlayQueue{IDs: []string{"x", "y"}, Current: "x"},
		)
	})
	h.playAlbum()
	h.advance(saveAfter)
	h.m.FlushSaved()
	h.lacks("restored")
	if n := h.requests("getPlayQueue") + h.requests("savePlayQueue"); n != 0 {
		t.Errorf("%d play queue requests with sync_queue off", n)
	}
	if _, err := h.savedFile(); err != nil {
		t.Errorf("this device's file wasn't written: %v", err)
	}
	if q, ok := h.serverQueue(); !ok || len(q.IDs) != 2 {
		t.Errorf("the server's queue changed: %+v", q)
	}
}

func TestSyncQueueTurnedOnByReloadStartsPushing(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.SyncQueue.On = false
	})
	h.playAlbum()
	h.advance(saveAfter)
	if n := h.requests("savePlayQueue"); n != 0 {
		t.Fatalf("%d pushes with sync_queue off", n)
	}
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte("sync_queue: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.keys(":reload-config<CR>")
	h.keys(":queue-clear<CR>")
	h.playAlbum()
	h.advance(saveAfter)
	if _, ok := h.serverQueue(); !ok {
		t.Error("the server got no queue after sync_queue was turned on")
	}
}

// pushes is how many times the server was sent a play queue so far.
func (h *harness) pushes() int { return h.requests("savePlayQueue") }

func TestStartingOfflineNeverOverwritesTheServersQueue(t *testing.T) {
	pc := newHarness(t, 120, 40, nil)
	pc.playAlbum()
	pc.advance(saveAfter)
	q, _ := pc.serverQueue()
	ids := slices.Clone(q.IDs)
	slices.Reverse(ids)
	elsewhere := subsonictest.PlayQueue{IDs: ids, Current: ids[0]}

	pc.srv.SetDown(true)
	laptop := pc.restart()
	pc.srv.SetPlayQueue(subsonictest.Username, elsewhere)
	laptop.keys(":queue-clear<CR>")
	laptop.advance(saveAfter)
	pc.srv.SetDown(false)
	laptop.cmd("reconnect")
	laptop.contains("back online")
	before := laptop.pushes()
	laptop.playAlbum()
	laptop.advance(saveAfter)
	if got := laptop.pushes(); got != before {
		t.Errorf("%d pushes from a session that never read the server's queue", got-before)
	}
	if got, _ := laptop.serverQueue(); !slices.Equal(got.IDs, ids) {
		t.Errorf("the server holds %q, want the other device's %q", got.IDs, ids)
	}
}

func TestARestoreThatDroppedTracksLeavesTheServersQueueWhole(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playAlbum()
	a.advance(saveAfter)
	q, _ := a.serverQueue()
	whole := append(slices.Clone(q.IDs), "not-in-this-library", "nor-this-one")
	a.srv.SetPlayQueue(subsonictest.Username, subsonictest.PlayQueue{IDs: whole, Current: whole[0]})

	b := a.otherDevice()
	b.contains("restored 6 of 8 tracks")
	b.advance(saveAfter)
	if got, _ := b.serverQueue(); !slices.Equal(got.IDs, whole) {
		t.Fatalf("restoring cut the server's queue to %q", got.IDs)
	}
	if s, err := b.savedFile(); err != nil || len(s.Tracks) != 6 {
		t.Errorf("this device's file = %+v, %v; want the 6 restored tracks", s, err)
	}

	b.keys(":queue-clear<CR>")
	b.advance(saveAfter)
	if got, ok := b.serverQueue(); ok {
		t.Errorf("an edit after the restore didn't reach the server: %q", got.IDs)
	}
}

func TestPositionAloneIsSentOncePerMinuteWhilePlaying(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.advance(saveAfter)
	base := h.pushes()

	h.fake.Tick(20 * time.Second)
	h.pump()
	h.advance(saveAfter)
	if got := h.pushes(); got != base {
		t.Errorf("%d pushes for 20 s of playing", got-base)
	}
	h.fake.Tick(50 * time.Second)
	h.pump()
	h.advance(saveAfter)
	if got := h.pushes(); got != base+1 {
		t.Errorf("%d pushes after 70 s of playing, want 1", got-base)
	}
	h.fake.Tick(5 * time.Second)
	h.pump()
	h.keys(" ")
	h.advance(saveAfter)
	if got := h.pushes(); got != base+2 {
		t.Errorf("pausing sent %d pushes, want 1 more", got-base-1)
	}
	if q, _ := h.serverQueue(); q.Position != 75*time.Second {
		t.Errorf("the server holds position %v, want 1m15s", q.Position)
	}
}

func TestQuitWaitsForOneDeadlineNotTwo(t *testing.T) {
	old := serverQueueTimeout
	serverQueueTimeout = 400 * time.Millisecond
	t.Cleanup(func() { serverQueueTimeout = old })
	srv := subsonictest.NewServer(t)
	srv.SetPlayQueueFault(subsonictest.PlayQueueHangs)
	c := subsonic.New(subsonic.Credentials{
		URL: srv.URL, Username: subsonictest.Username, Password: subsonictest.Password,
	})
	saved := func(id string) resume.Saved {
		return resume.Saved{Tracks: []library.TrackID{library.TrackID(id)}, Current: 0}
	}
	p := newQueuePush()
	p.allow()
	ctx := context.Background()
	start := time.Now()
	done := make(chan struct{})
	go func() { p.send(ctx, c, 1, saved("a"), false); close(done) }()
	time.Sleep(100 * time.Millisecond)
	p.send(ctx, c, 2, saved("b"), true)
	<-done
	if took := time.Since(start); took > 650*time.Millisecond {
		t.Errorf("two sends to a hung server took %v, want about one timeout", took)
	}
}

func TestARestoreThatFindsNoTrackLeavesTheServersQueueWhole(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	elsewhere := subsonictest.PlayQueue{IDs: []string{"gone-1", "gone-2"}, Current: "gone-1"}
	a.srv.SetPlayQueue(subsonictest.Username, elsewhere)

	b := a.otherDevice()
	b.advance(saveAfter)
	if got, ok := b.serverQueue(); !ok || !slices.Equal(got.IDs, elsewhere.IDs) {
		t.Errorf("a device that restored nothing left the server with %q", got.IDs)
	}
	b.playAlbum()
	b.advance(saveAfter)
	if got, _ := b.serverQueue(); len(got.IDs) != 6 {
		t.Errorf("a queue built after that didn't reach the server: %q", got.IDs)
	}
}

func TestTurningResumeOnStartsPushingWhenSyncWasAlreadyOn(t *testing.T) {
	h := newHarness(t, 120, 40, func(c *config.Config, _ *subsonictest.Server) {
		c.Resume.Off = true
	})
	if err := os.MkdirAll(filepath.Dir(h.path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(h.path, []byte("sync_queue: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	h.keys(":reload-config<CR>")
	h.playAlbum()
	h.advance(saveAfter)
	if _, ok := h.serverQueue(); !ok {
		t.Error("the server got no queue after resume was turned on")
	}
}

func TestResumingAfterAPauseDoesNotPushTheRoundedPosition(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.playAlbum()
	h.fake.Tick(134 * time.Second)
	h.pump()
	h.keys(" ")
	h.advance(saveAfter)
	base := h.pushes()
	h.keys(" ")
	h.advance(saveAfter)
	if got := h.pushes(); got != base {
		t.Errorf("%d pushes for playing on after a pause", got-base)
	}
	if q, _ := h.serverQueue(); q.Position != 134*time.Second {
		t.Errorf("the server holds position %v, want the paused 2m14s", q.Position)
	}
}

func TestAServerQueueThatCannotBeReadIsSaidAtTheRestore(t *testing.T) {
	a := newHarness(t, 120, 40, nil)
	a.playAlbum()
	a.advance(saveAfter)
	a.srv.SetPlayQueueFault(subsonictest.PlayQueueBroken)

	n := a.restart()
	n.contains("restored 6 tracks (not synced this session)")
	before := n.pushes()
	n.keys(":queue-clear<CR>")
	n.advance(saveAfter)
	if got := n.pushes(); got != before {
		t.Errorf("%d pushes from a session that never read the server's queue", got-before)
	}
}
