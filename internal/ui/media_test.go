package ui

import (
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/mediactl"
)

// fakeMedia records what navigo publishes.
type fakeMedia struct{ last mediactl.Status }

func (f *fakeMedia) Update(s mediactl.Status)          { f.last = s }
func (f *fakeMedia) Commands() <-chan mediactl.Command { return nil }
func (f *fakeMedia) Close() error                      { return nil }

func TestMediaControls(t *testing.T) {
	media := &fakeMedia{}
	h := setupHarness(t, 120, 40, nil, func(h *harness) { h.m.deps.Media = media })
	h.toAlbums()
	h.keys("3j<CR>j<CR>")
	if s := media.last; s.Title != "Low Tide" || s.Album != "Blue Hour" || len(s.Artists) != 1 ||
		s.Artists[0] != "Aurora Lane" ||
		!s.Playing ||
		s.Paused ||
		!s.CanNext ||
		s.Length != 185*time.Second {
		t.Errorf("published %+v", s)
	}

	// The OS's keys act like navigo's.
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.Pause}))
	if !media.last.Paused {
		t.Error("Pause didn't pause")
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.Pause})) // already paused: nothing
	if !media.last.Paused {
		t.Error("a second Pause resumed")
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.Play}))
	if media.last.Paused {
		t.Error("Play didn't resume")
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.Next}))
	h.contains("▶  Streetlamp")
	if media.last.Title != "Streetlamp" {
		t.Errorf("after Next: %q", media.last.Title)
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.SeekTo, Position: time.Minute}))
	if c := h.lastCall(); c != "SeekTo(1m0s)" {
		t.Errorf("SeekTo: %q", c)
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.SetVolume, Volume: 30}))
	if c := h.lastCall(); c != "SetVolume(30)" || media.last.Volume != 30 {
		t.Errorf("volume: %q, %d", c, media.last.Volume)
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.SetLoop, Loop: mediactl.LoopTrack}))
	h.contains("repeat one")
	if media.last.Loop != mediactl.LoopTrack {
		t.Errorf("loop %q", media.last.Loop)
	}
	h.send(MediaMsg(mediactl.Command{Kind: mediactl.Stop}))
	if media.last.Playing || media.last.TrackID != "" {
		t.Errorf("after Stop: %+v", media.last)
	}
}
