// Package app is navigo's UI-free core (ADR 0001). It holds the session
// state (library lists, queue, undo history, player status) and changes it
// only in Apply, which returns the new state plus effects as plain data.
// A runner executes the effects and feeds their results back in as
// events, so the core never does I/O and is tested by its return values.
package app

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
)

// QueueEntry is one row of the queue.
type QueueEntry struct {
	ID    player.EntryID
	Track library.Track
}

// PlayerState is what the player last reported.
type PlayerState struct {
	Playing   bool // an entry is loaded, paused or not
	Paused    bool
	Buffering bool
	Position  time.Duration
	Duration  time.Duration
	Volume    int
}

// State is the session. Treat it as immutable: Apply never changes the
// slices or maps of a State it was given.
type State struct {
	Albums        []library.Album
	AlbumsLoaded  bool
	AlbumsLoading bool
	// Tracks holds the tracks of albums opened so far.
	Tracks  map[library.AlbumID][]library.Track
	Loading map[library.AlbumID]bool

	Queue []QueueEntry
	// Current is the queue row that is playing, or last played; -1 if none.
	Current int
	Player  PlayerState

	playing    player.EntryID // the entry the player says is playing
	nextID     player.EntryID
	undo, redo []snapshot

	// Scrobbling of the playing entry: time listened, not counting seeks.
	listened  time.Duration
	lastPos   time.Duration
	scrobbled bool
}

type snapshot struct {
	queue   []QueueEntry
	current int
}

// New returns the state navigo starts with: nothing loaded, empty queue.
func New() State {
	return State{Current: -1, Tracks: map[library.AlbumID][]library.Track{}, Loading: map[library.AlbumID]bool{}}
}

// CanUndo and CanRedo report whether Undo and Redo would change the queue.
func (s State) CanUndo() bool { return len(s.undo) > 0 }
func (s State) CanRedo() bool { return len(s.redo) > 0 }

// Event is an input to Apply.
type Event interface{ isEvent() }

// Events from the UI.
type (
	// LoadAlbums asks for the album list, unless it is loaded or loading.
	LoadAlbums struct{}
	// LoadAlbum asks for an album's tracks, unless they are loaded.
	LoadAlbum struct{ ID library.AlbumID }
	// PlayNow replaces the queue with Tracks and plays Tracks[Start].
	PlayNow struct {
		Tracks []library.Track
		Start  int
	}
	// AddEnd adds tracks to the end of the queue.
	AddEnd struct{ Tracks []library.Track }
	// AddNext adds tracks after the current one.
	AddNext struct{ Tracks []library.Track }
	// PlayRow plays a row of the queue.
	PlayRow      struct{ Index int }
	PlayPause    struct{}
	NextTrack    struct{}
	PrevTrack    struct{}
	SeekBy       struct{ D time.Duration }
	StopPlayback struct{}
	Undo         struct{}
	Redo         struct{}
)

// Events from the runner.
type (
	AlbumsLoaded struct {
		Albums []library.Album
		Err    error
	}
	AlbumLoaded struct {
		ID     library.AlbumID
		Tracks []library.Track
		Err    error
	}
	// FromPlayer wraps an event from the Player.
	FromPlayer struct{ Event player.Event }
	// PlayerFailed: a Player call returned an error.
	PlayerFailed struct{ Err error }
)

// Effect is an output of Apply, for the runner to execute.
type Effect interface{ isEffect() }

type (
	FetchAlbums   struct{}
	FetchAlbum    struct{ ID library.AlbumID }
	Play          struct{ Span player.Span }
	SetNeighbours struct{ Prev, Next player.Entry }
	Pause         struct{}
	Resume        struct{}
	Seek          struct{ By time.Duration }
	SeekTo        struct{ Pos time.Duration }
	Stop          struct{}
	Scrobble      struct {
		Track      library.TrackID
		Submission bool
	}
	// Notice is a message for the user. When Command is set, the UI adds
	// its key and Hint, such as "· u brings the old queue back", so the
	// message follows rebinding.
	Notice struct {
		Text    string
		Command string
		Hint    string
		Err     bool
	}
)

// Scrobble rule: a play counts after half the track or 4 minutes.
const scrobbleAfter = 4 * time.Minute

// Apply returns the state after e and the effects it calls for.
func Apply(s State, e Event) (State, []Effect) {
	switch e := e.(type) {
	case LoadAlbums:
		if s.AlbumsLoaded || s.AlbumsLoading {
			return s, nil
		}
		s.AlbumsLoading = true
		return s, []Effect{FetchAlbums{}}
	case AlbumsLoaded:
		s.AlbumsLoading = false
		if e.Err != nil {
			return s, []Effect{Notice{Text: "couldn't load albums: " + e.Err.Error(), Err: true}}
		}
		s.Albums, s.AlbumsLoaded = e.Albums, true
		return s, nil
	case LoadAlbum:
		if _, ok := s.Tracks[e.ID]; ok || s.Loading[e.ID] {
			return s, nil
		}
		s.Loading = with(s.Loading, e.ID, true)
		return s, []Effect{FetchAlbum{ID: e.ID}}
	case AlbumLoaded:
		s.Loading = without(s.Loading, e.ID)
		if e.Err != nil {
			return s, []Effect{Notice{Text: "couldn't load the album: " + e.Err.Error(), Err: true}}
		}
		s.Tracks = with(s.Tracks, e.ID, e.Tracks)
		return s, nil

	case PlayNow:
		if e.Start < 0 || e.Start >= len(e.Tracks) {
			return s, nil
		}
		had := len(s.Queue) > 0
		s = s.edit(func(q []QueueEntry) ([]QueueEntry, int) { return nil, -1 })
		s.Queue, s.nextID = entries(s.nextID, e.Tracks)
		s.Current = e.Start
		n := Notice{Text: "playing · " + count(len(e.Tracks)) + " in the queue"}
		if had {
			n = Notice{Text: "playing · queue replaced with " + count(len(e.Tracks)), Command: "undo", Hint: "brings the old queue back"}
		}
		return s, []Effect{Play{Span: s.span(s.Current)}, n}
	case AddEnd:
		if len(e.Tracks) == 0 {
			return s, nil
		}
		var added []QueueEntry
		added, s.nextID = entries(s.nextID, e.Tracks)
		s = s.edit(func(q []QueueEntry) ([]QueueEntry, int) { return append(slices.Clone(q), added...), s.Current })
		text := fmt.Sprintf("%s added to the end of the queue (%d in queue)", count(len(added)), len(s.Queue))
		return s, append(s.neighbours(), Notice{Text: text, Command: "undo", Hint: "undoes"})
	case AddNext:
		if len(e.Tracks) == 0 {
			return s, nil
		}
		var added []QueueEntry
		added, s.nextID = entries(s.nextID, e.Tracks)
		at := s.Current + 1
		s = s.edit(func(q []QueueEntry) ([]QueueEntry, int) {
			return slices.Insert(slices.Clone(q), at, added...), s.Current
		})
		verb := "will play next"
		if len(added) == 1 {
			verb = "plays next"
		}
		return s, append(s.neighbours(), Notice{Text: count(len(added)) + " " + verb, Command: "undo", Hint: "undoes"})
	case PlayRow:
		if e.Index < 0 || e.Index >= len(s.Queue) {
			return s, nil
		}
		s.Current = e.Index
		return s, []Effect{Play{Span: s.span(e.Index)}}

	case PlayPause:
		switch {
		case s.Player.Playing && s.Player.Paused:
			return s, []Effect{Resume{}}
		case s.Player.Playing:
			return s, []Effect{Pause{}}
		case len(s.Queue) > 0:
			s.Current = max(s.Current, 0)
			return s, []Effect{Play{Span: s.span(s.Current)}}
		}
		return s, []Effect{Notice{Text: "the queue is empty"}}
	case NextTrack:
		if s.Current+1 >= len(s.Queue) {
			return s, []Effect{Notice{Text: "end of the queue"}}
		}
		s.Current++
		return s, []Effect{Play{Span: s.span(s.Current)}}
	case PrevTrack:
		if s.Player.Playing && (s.Player.Position > 3*time.Second || s.Current <= 0) {
			return s, []Effect{SeekTo{}}
		}
		if s.Current <= 0 {
			return s, nil
		}
		s.Current--
		return s, []Effect{Play{Span: s.span(s.Current)}}
	case SeekBy:
		if !s.Player.Playing {
			return s, nil
		}
		return s, []Effect{Seek{By: e.D}}
	case StopPlayback:
		if !s.Player.Playing {
			return s, nil
		}
		s.playing = 0
		s.Player = PlayerState{Volume: s.Player.Volume}
		return s, []Effect{Stop{}}

	case Undo:
		if len(s.undo) == 0 {
			return s, []Effect{Notice{Text: "nothing to undo"}}
		}
		last := s.undo[len(s.undo)-1]
		s.redo = append(slices.Clone(s.redo), snapshot{s.Queue, s.Current})
		s.undo = s.undo[:len(s.undo)-1]
		s = s.restore(last)
		return s, append(s.neighbours(), Notice{Text: "undone · " + count(len(s.Queue)) + " in the queue", Command: "redo", Hint: "redoes"})
	case Redo:
		if len(s.redo) == 0 {
			return s, []Effect{Notice{Text: "nothing to redo"}}
		}
		next := s.redo[len(s.redo)-1]
		s.undo = append(slices.Clone(s.undo), snapshot{s.Queue, s.Current})
		s.redo = s.redo[:len(s.redo)-1]
		s = s.restore(next)
		return s, append(s.neighbours(), Notice{Text: "redone · " + count(len(s.Queue)) + " in the queue", Command: "undo", Hint: "undoes"})

	case FromPlayer:
		return s.fromPlayer(e.Event)
	case PlayerFailed:
		return s, []Effect{Notice{Text: "player: " + e.Err.Error(), Err: true}}
	}
	return s, nil
}

func (s State) fromPlayer(e player.Event) (State, []Effect) {
	switch e := e.(type) {
	case player.TrackStarted:
		s.playing = e.Entry
		s.Player.Playing, s.Player.Position = true, 0
		s.listened, s.lastPos, s.scrobbled = 0, 0, false
		i := s.index(e.Entry)
		if i < 0 {
			// Undone out of the queue while it played: let it finish alone.
			return s, []Effect{SetNeighbours{}}
		}
		s.Current = i
		s.Player.Duration = s.Queue[i].Track.Duration
		return s, append(s.neighbours(), Scrobble{Track: s.Queue[i].Track.ID})
	case player.TrackEnded:
		// Stopped means Play or Stop replaced it on purpose.
		if e.Entry != s.playing || e.Reason == player.Stopped {
			return s, nil
		}
		var out []Effect
		if e.Reason == player.Failed {
			title := "the track"
			if i := s.index(e.Entry); i >= 0 {
				title = s.Queue[i].Track.Title
			}
			out = append(out, Notice{Text: fmt.Sprintf("couldn't play %s: %s", title, e.Err), Err: true})
		}
		// mpv moves on to the next entry by itself; without one, playback
		// is over.
		if i := s.index(e.Entry); i < 0 || i == len(s.Queue)-1 {
			s.playing = 0
			s.Player = PlayerState{Volume: s.Player.Volume}
		}
		return s, out
	case player.Position:
		if e.Entry != s.playing {
			return s, nil
		}
		s.Player.Position = e.Position
		if e.Duration > 0 {
			s.Player.Duration = e.Duration
		}
		if d := e.Position - s.lastPos; d > 0 && d <= 2*time.Second {
			s.listened += d
		}
		s.lastPos = e.Position
		if i := s.index(e.Entry); i >= 0 && !s.scrobbled && s.Player.Duration > 0 &&
			(s.listened >= s.Player.Duration/2 || s.listened >= scrobbleAfter) {
			s.scrobbled = true
			return s, []Effect{Scrobble{Track: s.Queue[i].Track.ID, Submission: true}}
		}
	case player.Paused:
		s.Player.Paused = e.Paused
	case player.Buffering:
		s.Player.Buffering = e.Buffering
	case player.Volume:
		s.Player.Volume = e.Percent
	case player.Died:
		s.playing = 0
		s.Player = PlayerState{Volume: s.Player.Volume}
		return s, []Effect{Notice{Text: fmt.Sprintf("the player stopped (%v)", e.Err), Command: "play-pause", Hint: "starts it again", Err: true}}
	}
	return s, nil
}

// edit records an undo point and applies f to the queue. f returns the
// new queue and current row.
func (s State) edit(f func([]QueueEntry) ([]QueueEntry, int)) State {
	s.undo = append(slices.Clone(s.undo), snapshot{s.Queue, s.Current})
	s.redo = nil
	s.Queue, s.Current = f(s.Queue)
	return s
}

// restore puts a snapshot back, keeping Current on the playing entry if it
// is in the restored queue.
func (s State) restore(snap snapshot) State {
	s.Queue, s.Current = snap.queue, snap.current
	if s.playing != 0 {
		s.Current = s.index(s.playing)
	}
	return s
}

// neighbours tells the player the entries around the playing one.
func (s State) neighbours() []Effect {
	if s.playing == 0 {
		return nil
	}
	i := s.index(s.playing)
	if i < 0 {
		return []Effect{SetNeighbours{}}
	}
	sp := s.span(i)
	return []Effect{SetNeighbours{Prev: sp.Prev, Next: sp.Next}}
}

func (s State) span(i int) player.Span {
	sp := player.Span{Current: s.Queue[i].entry()}
	if i > 0 {
		sp.Prev = s.Queue[i-1].entry()
	}
	if i+1 < len(s.Queue) {
		sp.Next = s.Queue[i+1].entry()
	}
	return sp
}

func (s State) index(id player.EntryID) int {
	return slices.IndexFunc(s.Queue, func(q QueueEntry) bool { return q.ID == id })
}

func (q QueueEntry) entry() player.Entry { return player.Entry{ID: q.ID, Track: q.Track.ID} }

// entries makes new queue entries for tracks, numbering on from last.
func entries(last player.EntryID, tracks []library.Track) ([]QueueEntry, player.EntryID) {
	out := make([]QueueEntry, len(tracks))
	for i, t := range tracks {
		last++
		out[i] = QueueEntry{ID: last, Track: t}
	}
	return out, last
}

func count(n int) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}

func with[K comparable, V any](m map[K]V, k K, v V) map[K]V {
	m = maps.Clone(m)
	if m == nil {
		m = map[K]V{}
	}
	m[k] = v
	return m
}

func without[K comparable, V any](m map[K]V, k K) map[K]V {
	m = maps.Clone(m)
	delete(m, k)
	return m
}

func (LoadAlbums) isEvent()   {}
func (LoadAlbum) isEvent()    {}
func (PlayNow) isEvent()      {}
func (AddEnd) isEvent()       {}
func (AddNext) isEvent()      {}
func (PlayRow) isEvent()      {}
func (PlayPause) isEvent()    {}
func (NextTrack) isEvent()    {}
func (PrevTrack) isEvent()    {}
func (SeekBy) isEvent()       {}
func (StopPlayback) isEvent() {}
func (Undo) isEvent()         {}
func (Redo) isEvent()         {}
func (AlbumsLoaded) isEvent() {}
func (AlbumLoaded) isEvent()  {}
func (FromPlayer) isEvent()   {}
func (PlayerFailed) isEvent() {}

func (FetchAlbums) isEffect()   {}
func (FetchAlbum) isEffect()    {}
func (Play) isEffect()          {}
func (SetNeighbours) isEffect() {}
func (Pause) isEffect()         {}
func (Resume) isEffect()        {}
func (Seek) isEffect()          {}
func (SeekTo) isEffect()        {}
func (Stop) isEffect()          {}
func (Scrobble) isEffect()      {}
func (Notice) isEffect()        {}
