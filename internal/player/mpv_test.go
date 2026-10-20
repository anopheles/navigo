package player

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// silence returns a WAV of d of silence (8 kHz, mono, 8-bit).
func silence(d time.Duration) []byte {
	n := int(d.Seconds() * 8000)
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + n))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))    // PCM
	w(uint16(1))    // mono
	w(uint32(8000)) // sample rate
	w(uint32(8000)) // byte rate
	w(uint16(1))    // block align
	w(uint16(8))    // bits per sample
	b.WriteString("data")
	w(uint32(n))
	b.Write(bytes.Repeat([]byte{0x80}, n))
	return b.Bytes()
}

type harness struct {
	t   *testing.T
	m   *MPV
	srv *httptest.Server

	mu   sync.Mutex
	urls int
}

// newMPV starts an MPV whose tracks are served as silent WAVs: track "2s"
// lasts two seconds.
func newMPV(t *testing.T) *harness {
	t.Helper()
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not on PATH")
	}
	h := &harness{t: t}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d, err := time.ParseDuration(strings.TrimPrefix(r.URL.Path, "/"))
		if err != nil {
			http.NotFound(w, r)
			return
		}
		// Range requests work, as with a raw stream from Navidrome, so
		// mpv can seek.
		w.Header().Set("Content-Type", "audio/wav")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(silence(d)))
	}))
	t.Cleanup(h.srv.Close)
	h.m = NewMPV(MPVOptions{
		Args: []string{"--ao=null"},
		StreamURL: func(id library.TrackID) string {
			h.mu.Lock()
			h.urls++
			h.mu.Unlock()
			return fmt.Sprintf("%s/%s?salt=%d", h.srv.URL, id, time.Now().UnixNano())
		},
	})
	t.Cleanup(func() { _ = h.m.Close() })
	return h
}

func entry(id EntryID, d string) Entry { return Entry{ID: id, Track: library.TrackID(d)} }

// await reads events until match returns true, failing after 10 seconds.
func (h *harness) await(what string, match func(Event) bool) Event {
	h.t.Helper()
	timeout := time.After(10 * time.Second)
	for {
		select {
		case e, ok := <-h.m.Events():
			if !ok {
				h.t.Fatalf("events closed waiting for %s", what)
			}
			if match(e) {
				return e
			}
		case <-timeout:
			h.t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func started(id EntryID) func(Event) bool {
	return func(e Event) bool { s, ok := e.(TrackStarted); return ok && s.Entry == id }
}

func TestMPVPlaysTheNextEntryByItself(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.Play(ctx, Span{Current: entry(1, "1s"), Next: entry(2, "1s")}); err != nil {
		t.Fatal(err)
	}
	h.await("entry 1 to start", started(1))
	h.await("entry 1 to finish", func(e Event) bool {
		x, ok := e.(TrackEnded)
		return ok && x.Entry == 1 && x.Reason == Finished
	})
	h.await("entry 2 to start", started(2))
	pos := h.await("a position with a duration", func(e Event) bool {
		p, ok := e.(Position)
		return ok && p.Entry == 2 && p.Duration > 0
	}).(Position)
	if pos.Duration < 900*time.Millisecond || pos.Duration > 1100*time.Millisecond {
		t.Errorf("duration = %v, want about 1s", pos.Duration)
	}

	// The span slid: entry 1 is now previous; nothing came after 2.
	if err := h.m.SetNeighbours(ctx, entry(1, "1s"), Entry{}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := h.m.process().Get(ctx, "playlist-count", &count); err != nil || count != 2 {
		t.Errorf("playlist-count = %d, %v; want 2 (previous and current)", count, err)
	}
	h.await("entry 2 to finish at the end of the queue", func(e Event) bool {
		x, ok := e.(TrackEnded)
		return ok && x.Entry == 2 && x.Reason == Finished
	})
}

func TestMPVSetNeighboursReplacesNext(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.Play(
		ctx,
		Span{Prev: entry(1, "1s"), Current: entry(2, "1s"), Next: entry(3, "1s")},
	); err != nil {
		t.Fatal(err)
	}
	h.await("entry 2 to start", started(2))
	if err := h.m.SetNeighbours(ctx, entry(1, "1s"), entry(4, "1s")); err != nil {
		t.Fatal(err)
	}
	h.await("entry 4, not 3, to start next", func(e Event) bool {
		s, ok := e.(TrackStarted)
		if ok && s.Entry == 3 {
			t.Fatal("entry 3 started after it was replaced")
		}
		return ok && s.Entry == 4
	})

	// Keeping the same neighbours reloads nothing.
	h.mu.Lock()
	before := h.urls
	h.mu.Unlock()
	if err := h.m.SetNeighbours(ctx, entry(2, "1s"), Entry{}); err != nil {
		t.Fatal(err)
	}
	if err := h.m.SetNeighbours(ctx, entry(2, "1s"), Entry{}); err != nil {
		t.Fatal(err)
	}
	h.mu.Lock()
	after := h.urls
	h.mu.Unlock()
	if after != before {
		t.Errorf("SetNeighbours with unchanged neighbours signed %d new URLs", after-before)
	}
}

func TestMPVPauseAndSeek(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.Play(ctx, Span{Current: entry(1, "30s")}); err != nil {
		t.Fatal(err)
	}
	h.await(
		"entry 1 to play",
		func(e Event) bool { p, ok := e.(Position); return ok && p.Entry == 1 },
	)
	if err := h.m.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	h.await("paused", func(e Event) bool { p, ok := e.(Paused); return ok && p.Paused })
	if err := h.m.SeekTo(ctx, 20*time.Second); err != nil {
		t.Fatal(err)
	}
	h.await("position 20s", func(e Event) bool {
		p, ok := e.(Position)
		return ok && p.Position >= 19*time.Second && p.Position <= 21*time.Second
	})
	if err := h.m.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	h.await("resumed", func(e Event) bool { p, ok := e.(Paused); return ok && !p.Paused })
	if err := h.m.SetVolume(ctx, 40); err != nil {
		t.Fatal(err)
	}
	h.await("volume 40", func(e Event) bool { v, ok := e.(Volume); return ok && v.Percent == 40 })
}

func TestMPVRestartsAfterDying(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.Play(ctx, Span{Current: entry(1, "30s")}); err != nil {
		t.Fatal(err)
	}
	h.await("entry 1 to start", started(1))
	_, _ = h.m.process().Command(ctx, "quit")
	h.await("died", func(e Event) bool { _, ok := e.(Died); return ok })
	if err := h.m.Play(ctx, Span{Current: entry(2, "30s")}); err != nil {
		t.Fatalf("Play after mpv died: %v", err)
	}
	h.await("entry 2 to start in a new mpv", started(2))
}

// A volume chosen before mpv runs applies once it starts.
func TestMPVVolumeBeforeStart(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.SetVolume(ctx, 35); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Play(ctx, Span{Current: entry(1, "30s")}); err != nil {
		t.Fatal(err)
	}
	h.await("volume 35", func(e Event) bool { v, ok := e.(Volume); return ok && v.Percent == 35 })
}

func TestMPVBadArgs(t *testing.T) {
	if _, err := exec.LookPath("mpv"); err != nil {
		t.Skip("mpv not on PATH")
	}
	m := NewMPV(
		MPVOptions{
			Args:      []string{"--no-such-option"},
			StreamURL: func(library.TrackID) string { return "" },
		},
	)
	defer m.Close()
	err := m.Play(context.Background(), Span{Current: entry(1, "1s")})
	if err == nil || !strings.Contains(err.Error(), "no-such-option") {
		t.Fatalf("Play = %v, want an error naming the bad option", err)
	}
}

func TestVersionOK(t *testing.T) {
	for v, want := range map[string]bool{
		"mpv v0.41.0":            true,
		"mpv 0.38.0":             true,
		"mpv v0.38.0-45-g1234ab": true,
		"mpv 0.37.1":             false,
		"mpv v0.35.0":            false,
		"mpv 1.0.0":              true,
		"mpv git-abcdef":         true,
	} {
		if got := versionOK(v); got != want {
			t.Errorf("versionOK(%q) = %v, want %v", v, got, want)
		}
	}
}

// A server that is gone makes mpv run through its playlist on its own.
// After Stop and a new Play, events from the old entries start nothing.
func TestMPVIgnoresReplacedEntries(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	bad := MPVOptions{
		Args:      []string{"--ao=null"},
		StreamURL: func(id library.TrackID) string { return "http://127.0.0.1:1/" + string(id) },
	}
	h.m.opts.StreamURL = bad.StreamURL
	if err := h.m.Play(ctx, Span{Current: entry(1, "x"), Next: entry(2, "x")}); err != nil {
		t.Fatal(err)
	}
	h.await(
		"entry 1 to fail",
		func(e Event) bool { x, ok := e.(TrackEnded); return ok && x.Entry == 1 && x.Reason == Failed },
	)
	if err := h.m.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	// Events mpv sent before the new Play were true when sent; drain them.
	for drained := false; !drained; {
		select {
		case <-h.m.Events():
		case <-time.After(300 * time.Millisecond):
			drained = true
		}
	}
	h.m.opts.StreamURL = func(id library.TrackID) string { return h.srv.URL + "/" + string(id) }
	if err := h.m.Play(ctx, Span{Current: entry(3, "5s")}); err != nil {
		t.Fatal(err)
	}
	h.await("entry 3 to start", func(e Event) bool {
		if s, ok := e.(TrackStarted); ok && s.Entry != 3 {
			t.Fatalf("stale entry %d started after the new Play", s.Entry)
		}
		s, ok := e.(TrackStarted)
		return ok && s.Entry == 3
	})
}

func TestMPVCheck(t *testing.T) {
	ctx := context.Background()
	missing := NewMPV(MPVOptions{Path: filepath.Join(t.TempDir(), "no-mpv")})
	if err := missing.Check(ctx); !errors.Is(err, ErrMPVMissing) {
		t.Errorf("a missing mpv: %v", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("the fake mpv is a shell script")
	}
	old := filepath.Join(t.TempDir(), "mpv")
	if err := os.WriteFile(
		old,
		[]byte(
			"#!/bin/sh\necho 'mpv v0.37.0 Copyright © 2000-2023 mpv/MPlayer/mplayer2 projects'\n",
		),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	err := NewMPV(MPVOptions{Path: old}).Check(ctx)
	if !errors.Is(err, ErrMPVTooOld) || !strings.Contains(err.Error(), "mpv v0.37.0") {
		t.Errorf("an old mpv: %v", err)
	}
	if _, err := exec.LookPath("mpv"); err == nil {
		if err := NewMPV(MPVOptions{}).Check(ctx); err != nil {
			t.Errorf("the mpv on PATH: %v", err)
		}
	}
}

// Play with a start position, as when resuming after mpv died.
func TestMPVPlaysFromAPosition(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	if err := h.m.Play(ctx, Span{Current: entry(1, "60s"), Start: 42 * time.Second}); err != nil {
		t.Fatal(err)
	}
	h.await("a position past 42s", func(e Event) bool {
		p, ok := e.(Position)
		return ok && p.Entry == 1 && p.Position >= 42*time.Second
	})
}

// Each file's title reaches mpv's media-title, which its Now Playing and
// SMTC show, commas and all.
func TestMPVMediaTitle(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	e := Entry{ID: 1, Track: "30s", Title: "Low Tide, Live — Aurora Lane"}
	if err := h.m.Play(ctx, Span{Current: e}); err != nil {
		t.Fatal(err)
	}
	h.await("entry 1 to start", started(1))
	var title string
	if err := h.m.process().Get(ctx, "media-title", &title); err != nil {
		t.Fatal(err)
	}
	if title != e.Title {
		t.Errorf("media-title = %q, want %q", title, e.Title)
	}
}

// Where mpv provides the media controls, 0.38 plays but gets a warning.
func TestMPVCheckMediaControls(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fake mpv is a shell script")
	}
	defer func(was bool) { mediaByMPV = was }(mediaByMPV)
	mediaByMPV = true
	fake := filepath.Join(t.TempDir(), "mpv")
	if err := os.WriteFile(
		fake,
		[]byte(
			"#!/bin/sh\necho 'mpv v0.38.0 Copyright © 2000-2024 mpv/MPlayer/mplayer2 projects'\n",
		),
		0o755,
	); err != nil {
		t.Fatal(err)
	}
	if err := NewMPV(
		MPVOptions{Path: fake},
	).Check(context.Background()); !errors.Is(
		err,
		ErrNoMediaControls,
	) {
		t.Errorf("mpv 0.38 with mpv's media controls: %v", err)
	}
	mediaByMPV = false
	if err := NewMPV(MPVOptions{Path: fake}).Check(context.Background()); err != nil {
		t.Errorf("mpv 0.38 on Linux: %v", err)
	}
}

func TestReplayGainArgs(t *testing.T) {
	for _, tc := range []struct {
		g    ReplayGain
		want string
	}{
		{ReplayGain{}, ""},
		{ReplayGain{Mode: GainOff}, "--replaygain=no --replaygain-preamp=0"},
		{ReplayGain{Mode: GainTrack, Preamp: -3.5}, "--replaygain=track --replaygain-preamp=-3.5"},
		{ReplayGain{Mode: GainAlbum, Preamp: 6}, "--replaygain=album --replaygain-preamp=6"},
	} {
		if got := strings.Join(tc.g.args(), " "); got != tc.want {
			t.Errorf("%+v args = %q, want %q", tc.g, got, tc.want)
		}
	}
}

func TestGainFrom(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	for _, tc := range []struct {
		name         string
		tags         map[string]string
		track, album *float64
	}{
		{"none", map[string]string{"encoder": "x"}, nil, nil},
		{"vorbis", map[string]string{"REPLAYGAIN_TRACK_GAIN": "-6.50 dB", "REPLAYGAIN_ALBUM_GAIN": "-7.1 dB"}, f(-6.5), f(-7.1)},
		{"id3 case", map[string]string{"replaygain_track_gain": "+2.00 dB"}, f(2), nil},
		{"opus r128", map[string]string{"R128_TRACK_GAIN": "-1280", "R128_ALBUM_GAIN": "0"}, f(0), f(5)},
		{"unparsable", map[string]string{"REPLAYGAIN_TRACK_GAIN": "loud"}, nil, nil},
	} {
		track, album := gainFrom(tc.tags)
		same := func(a, b *float64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
		if !same(track, tc.track) || !same(album, tc.album) {
			t.Errorf("%s: got %v, %v", tc.name, track, album)
		}
	}
}

func TestMPVReplayGain(t *testing.T) {
	h := newMPV(t)
	ctx := context.Background()
	prop := func(name string, v any) {
		t.Helper()
		if err := h.m.process().Get(ctx, name, v); err != nil {
			t.Fatal(err)
		}
	}
	// Asked for before mpv runs, it is how mpv starts.
	if err := h.m.SetReplayGain(ctx, ReplayGain{Mode: GainAlbum, Preamp: 3}); err != nil {
		t.Fatal(err)
	}
	if err := h.m.Play(ctx, Span{Current: entry(1, "30s")}); err != nil {
		t.Fatal(err)
	}
	h.await("entry 1 to start", started(1))
	var mode any // mpv answers "no" as false
	var preamp float64
	prop("replaygain", &mode)
	prop("replaygain-preamp", &preamp)
	if mode != "album" || preamp != 3 {
		t.Errorf("started with %q, %v", mode, preamp)
	}
	// Changed while playing, it applies without ending the track.
	if err := h.m.SetReplayGain(ctx, ReplayGain{Mode: GainOff, Preamp: -2}); err != nil {
		t.Fatal(err)
	}
	prop("replaygain", &mode)
	prop("replaygain-preamp", &preamp)
	if mode != false || preamp != -2 {
		t.Errorf("changed to %q, %v", mode, preamp)
	}
	h.await("a tag-less file to report no gain", func(e Event) bool {
		g, ok := e.(GainTags)
		return ok && g.Entry == 1 && g.Track == nil && g.Album == nil
	})
}
