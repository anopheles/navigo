// Package mediactl connects navigo to the OS's media controls: media
// keys and the now-playing widgets. On Linux it is an MPRIS server on the
// session bus; elsewhere mpv's own integration does the work and this is a
// stub. No cgo anywhere.
package mediactl

import "time"

// Status is what the OS shows: the track, whether it plays, and what the
// controls can do now.
type Status struct {
	TrackID  string // empty: nothing to show
	Title    string
	Artists  []string
	Album    string
	Length   time.Duration
	Playing  bool
	Paused   bool
	Position time.Duration
	CanNext  bool
	CanPrev  bool
	CanPlay  bool // there is something to play
	Volume   int  // percent
	Loop     Loop
}

// Loop is the repeat mode as MPRIS names it.
type Loop string

const (
	LoopNone     Loop = "None"
	LoopPlaylist Loop = "Playlist"
	LoopTrack    Loop = "Track"
)

// Kind is what a Command asks for.
type Kind int

const (
	Play Kind = iota
	Pause
	PlayPause
	Stop
	Next
	Previous
	SeekBy    // Offset
	SeekTo    // Position
	SetVolume // Volume, percent
	SetLoop   // Loop
)

// Command is something the OS's controls asked for.
type Command struct {
	Kind     Kind
	Offset   time.Duration
	Position time.Duration
	Volume   int
	Loop     Loop
}

// Controls is the OS's media controls.
type Controls interface {
	// Update tells the OS what plays now.
	Update(Status)
	// Commands delivers what the user asked for through the OS.
	Commands() <-chan Command
	Close() error
}

// stub is Controls where mpv does the work, or there is nothing to do.
type stub struct{ c chan Command }

func newStub() Controls                 { return stub{make(chan Command)} }
func (stub) Update(Status)              {}
func (s stub) Commands() <-chan Command { return s.c }
func (s stub) Close() error             { close(s.c); return nil }
