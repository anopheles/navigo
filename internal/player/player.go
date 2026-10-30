// Package player plays queue entries. The Player interface hides mpv
// completely: the process, its IPC, its playlist and its entry IDs (ADR
// 0002). MPV is the real adapter and Fake the in-memory one for tests.
//
// The core owns the queue and hands the Player a span of it: the
// previous, current and next entries. mpv holds that span as its
// playlist, so the next track plays gaplessly and mpv's own Previous (OS
// media controls on macOS and Windows) has somewhere to go. After every
// TrackStarted the core sends the new neighbours with SetNeighbours.
package player

import (
	"context"
	"time"

	"github.com/anopheles/navigo/internal/library"
)

// EntryID names one queue entry. The core assigns them; zero means none.
type EntryID uint64

// Entry is a queue entry: a track placed in the queue once.
type Entry struct {
	ID    EntryID
	Track library.TrackID
	// Title is what the OS's media controls show for it, such as
	// "Low Tide — Aurora Lane"; mpv's own Now Playing and SMTC read it.
	Title string
	// CoverID names the cover art the OS's media controls show for it, for
	// the Player to resolve to a file; empty for none.
	CoverID string
}

// Span is the part of the queue the Player holds. A zero Prev or Next
// means there is none.
type Span struct {
	Prev, Current, Next Entry
	// Start is where Current starts playing; zero is its beginning.
	Start time.Duration
}

// Player plays queue entries and reports what happens as Events.
type Player interface {
	// Check reports whether the player can play at all, before the first
	// track: ErrMPVMissing, ErrMPVTooOld, or nil.
	Check(ctx context.Context) error
	// Play starts w.Current at w.Start, unpaused, replacing whatever was
	// playing.
	Play(ctx context.Context, w Span) error
	// SetNeighbours replaces the entries around the current one without
	// interrupting it, after the queue changed or a new track started.
	SetNeighbours(ctx context.Context, prev, next Entry) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	// SeekTo jumps to a position in the current track; SeekBy moves by an
	// offset, backwards when negative.
	SeekTo(ctx context.Context, pos time.Duration) error
	SeekBy(ctx context.Context, d time.Duration) error
	// SetVolume sets the volume in percent, 0 to 100.
	SetVolume(ctx context.Context, percent int) error
	// SetReplayGain sets the ReplayGain mode and preamp now, on the
	// current and the queued tracks, and on any player started later.
	SetReplayGain(ctx context.Context, g ReplayGain) error
	// Stop ends playback and empties the span.
	Stop(ctx context.Context) error
	// Events delivers what happened, in order. It is closed by Close.
	Events() <-chan Event
	Close() error
}

// GainMode is how ReplayGain levels the loudness: not at all, per track,
// or per album (which falls back to the track's gain when an album gain
// is missing).
type GainMode string

const (
	GainOff   GainMode = "off"
	GainTrack GainMode = "track"
	GainAlbum GainMode = "album"
)

// ReplayGain is the mode and the preamp in dB that the player applies.
type ReplayGain struct {
	Mode   GainMode
	Preamp float64
}

// Event is one of the event types below.
type Event interface{ isEvent() }

// TrackStarted: the entry began playing.
type TrackStarted struct{ Entry EntryID }

// TrackEnded: the entry stopped playing, for Reason.
type TrackEnded struct {
	Entry  EntryID
	Reason EndReason
	// Err is mpv's reason when Reason is Failed.
	Err string
}

// EndReason is why a track ended.
type EndReason int

const (
	// Finished: it played to the end.
	Finished EndReason = iota
	// Stopped: Play, SetNeighbours or Stop replaced or removed it.
	Stopped
	// Failed: it could not be played, such as a stream error.
	Failed
	// NoAudio: the audio output couldn't be opened, so nothing can play;
	// the file is not to blame.
	NoAudio
)

// Position of the current entry, sent about once a second while playing
// after every seek, and once the duration is known. Duration is zero
// until then.
type Position struct {
	Entry              EntryID
	Position, Duration time.Duration
}

// Paused: playback was paused or resumed.
type Paused struct{ Paused bool }

// Buffering: playback is waiting for data, or no longer is.
type Buffering struct{ Buffering bool }

// Volume changed, in percent.
type Volume struct{ Percent int }

// GainTags are the ReplayGain tags found in the playing entry's file, in
// dB. A tag the file lacks is nil; both nil means no ReplayGain data.
type GainTags struct {
	Entry        EntryID
	Track, Album *float64
}

// Died: the player stopped working, such as mpv exiting. The span is
// gone; the next Play starts over.
type Died struct{ Err error }

func (TrackStarted) isEvent() {}
func (TrackEnded) isEvent()   {}
func (Position) isEvent()     {}
func (Paused) isEvent()       {}
func (Buffering) isEvent()    {}
func (Volume) isEvent()       {}
func (GainTags) isEvent()     {}
func (Died) isEvent()         {}
