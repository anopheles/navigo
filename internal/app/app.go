// Package app is navigo's UI-free core (ADR 0001). It holds the session
// state (library lists, queue, undo history, player status) and changes it
// only in Apply, which returns the new state plus effects as plain data.
// A runner executes the effects and feeds their results back in as
// events, so the core never does I/O and is tested by its return values.
package app

import (
	"fmt"
	"reflect"
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

// SyncStatus is the state of a sync with the server.
type SyncStatus struct {
	Running     bool
	Done, Total int  // tracks fetched so far, and how many the server has
	Failed      bool // the last sync ended in an error
}

// State is the session. Treat it as immutable: Apply never changes the
// slices or maps of a State it was given.
type State struct {
	// Library is the snapshot every list reads; nil until the cache is
	// loaded or the first sync is done.
	Library *library.Snapshot
	Sync    SyncStatus
	// ReplayGain is the mode and preamp in use: the configured ones, or
	// what :replaygain set for the session.
	ReplayGain player.ReplayGain
	// Gain holds the ReplayGain tags of the playing file, once the player
	// has read them.
	Gain player.GainTags
	// ResumeOn says the queue is kept between runs (see resume.go).
	ResumeOn bool
	// SyncOn says the saved queue is also kept on the server.
	SyncOn bool
	// recs are the user's Recommendations (see recommendations.go).
	recs recsState
	// Home is Home's album lists by kind, as the server last sent them.
	Home map[string]AlbumList

	cacheChecked bool // the cache file has been tried

	// mixWait is a Mix asked for during the first sync; it runs once the
	// library is there (see mix.go).
	mixWait *mixWait
	// mixLookup is a Mix around a seed waiting for the server's similar
	// artists; mixSeq numbers the lookups, so a stale answer is dropped.
	mixLookup *mixWait
	mixSeq    int

	// Resume: the saved queue is asked for once the login has answered
	// (savedAsked) and waits in pending for the library; saving starts once
	// it is restored or known to be none, so the file isn't overwritten
	// before it is read.
	savedAsked, savedLoaded, restored, saving, queueUnsynced bool
	// heldBack is the queue a restore left after dropping tracks; the
	// server keeps its fuller one until the queue changes.
	heldBack []library.TrackID
	pending  *SavedLoaded
	loggedIn bool

	// Offline: the server can't be reached (see offline.go). Probing is
	// set while a check of the server is running.
	Offline  bool
	Probing  bool
	retry    int // backoff step
	retrySeq int // the scheduled retry; older ones are ignored

	// After a stream failure: player events are ignored until the next
	// Play, and broken files skipped in a row are counted.
	halted                 bool
	skips                  int
	failedTitle, failedErr string
	// stale are entries mpv may still report on after a failure: the one
	// that failed and the one mpv ran on to. Their events are ignored.
	stale []player.EntryID

	Queue []QueueEntry
	// Current is the queue row that is playing, or last played; -1 if none.
	Current int
	Player  PlayerState
	Repeat  Repeat

	// mpv's health: PlayerProblem is why it can't play at all (missing or
	// too old); PlayerDown, that it died and <Space> restarts it;
	// PlayerCapped, that it died too often to restart for now.
	PlayerProblem string
	PlayerDown    bool
	PlayerCapped  bool
	// NoAudio: the audio output couldn't be opened, until a track really
	// plays. PlayerStuck: a player call has gone unanswered for a while.
	NoAudio     bool
	PlayerStuck bool
	installHint string
	resume      *resumePoint
	deaths      []time.Time

	playing      player.EntryID // the entry the player says is playing
	playingTrack library.Track  // its track, even if the queue changed since
	// orphanNext is where the queue goes on after the playing entry was
	// removed from it while it played.
	orphanNext player.EntryID

	nextID     player.EntryID
	undo, redo []snapshot

	// Register is what y and d last took, for p and P to paste.
	Register []library.Track

	// Scrobbling of the playing entry: time listened, not counting seeks.
	listened  time.Duration
	lastPos   time.Duration
	scrobbled bool
}

type snapshot struct {
	queue   []QueueEntry
	current int
}

// New returns the state navigo starts with: nothing loaded, empty queue,
// and mpv's volume (100 with --no-config) until the player says.
func New() State { return State{Current: -1, Player: PlayerState{Volume: 100}} }

// NowPlaying returns the track the player is playing and its queue row, or
// -1 when the queue changed and no longer holds it (it plays on from mpv's
// buffer while offline, or after an undo). ok is false when nothing plays.
func (s State) NowPlaying() (t library.Track, row int, ok bool) {
	if !s.Player.Playing || s.playing == 0 {
		return library.Track{}, -1, false
	}
	return s.playingTrack, s.index(s.playing), true
}

// CanUndo and CanRedo report whether Undo and Redo would change the queue.
func (s State) CanUndo() bool { return len(s.undo) > 0 }
func (s State) CanRedo() bool { return len(s.redo) > 0 }

// Event is an input to Apply.
type Event interface{ isEvent() }

// Events from the UI.
type (
	// Start loads the library cache; sent once when navigo starts.
	Start struct{}
	// LoggedIn: the server accepted the credentials, so syncing can start.
	LoggedIn struct{}
	// SyncNow forces a full sync (:sync).
	SyncNow struct{}
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
	// CacheLoaded carries the cached snapshot, or nil if there is none.
	CacheLoaded  struct{ Library *library.Snapshot }
	SyncProgress struct{ Done, Total int }
	Synced       struct {
		Library *library.Snapshot
		Err     error
	}
	// Scrobbled: the server took a submission scrobble, made at At.
	Scrobbled struct {
		Track library.TrackID
		At    time.Time
	}
	// FromPlayer wraps an event from the Player.
	FromPlayer struct{ Event player.Event }
	// PlayerFailed: a Player call returned an error.
	PlayerFailed struct{ Err error }
	// PlayerUnresponsive: a Player call has gone unanswered for a while
	// (Stuck), or an answer came after all.
	PlayerUnresponsive struct{ Stuck bool }
	// Failed: some other work went wrong in a way navigo can't explain,
	// such as saving the cache.
	Failed struct {
		What string
		Err  error
	}
)

// Effect is an output of Apply, for the runner to execute.
type Effect interface{ isEffect() }

type (
	LoadCache struct{}
	// StartSync syncs onto Old: in full when Full is set or the server has
	// scanned since, else only playlists and stars.
	StartSync struct {
		Old  *library.Snapshot
		Full bool
	}
	SaveCache     struct{ Library *library.Snapshot }
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
		// Quiet keeps it out of the status line: only :messages has it.
		Quiet bool
		// Unexpected marks an error navigo can't explain in plain words;
		// the UI logs it and points to the log.
		Unexpected bool
	}
)

// Scrobble rule: a play counts after half the track or 4 minutes.
const scrobbleAfter = 4 * time.Minute

// Apply returns the state after e and the effects it calls for. When what
// the OS's media controls show changed, it adds PublishNowPlaying.
func Apply(s State, e Event) (State, []Effect) {
	n, out := apply(s, e)
	n, out = withSaving(s, n, out)
	n = withRecs(s, n)
	if np := n.nowPlaying(); !reflect.DeepEqual(np, s.nowPlaying()) {
		out = append(out, PublishNowPlaying{np})
	}
	return n, out
}

func apply(s State, e Event) (State, []Effect) {
	switch e := e.(type) {
	case Start:
		out := []Effect{LoadCache{}}
		switch {
		case !s.SyncOn:
			var load []Effect
			s, load = s.loadSaved(false)
			out = append(out, load...)
		case s.ResumeOn:
			out = append(out, ScheduleSavedGrace{After: savedGrace})
		}
		if s.recsOn() {
			out = append(out, LoadRecs{})
		}
		return s, out
	case SavedLoaded, ConfigureResume, SavedGraceElapsed, LoginFailed:
		return s.applyResume(e)
	case LoggedOut:
		s, out := s.applyResume(e)
		s.recs.stored = nil
		return s, append(out, DeleteRecs{})
	case ConfigureListenBrainz, RecsLoaded, RecsFetched:
		return s.applyRecs(e)
	case CacheLoaded:
		s.cacheChecked = true
		if s.Library == nil {
			s.Library = e.Library
		}
		if s.loggedIn {
			return s.startSync(false)
		}
		return s, nil
	case Scrobbled:
		// The cache learns the play now, so a Mix made before the next
		// sync doesn't pick what was just heard.
		if s.Library == nil {
			return s, nil
		}
		s.Library = s.Library.WithPlay(e.Track, e.At)
		return s, []Effect{SaveCache{Library: s.Library}}
	case LoggedIn:
		s.loggedIn, s.Offline = true, false
		s, out := s.loadSaved(true)
		if s.cacheChecked {
			s, sync := s.startSync(false)
			return s, append(out, sync...)
		}
		return s, out
	case SyncNow:
		// :sync always asks ListenBrainz too, which doesn't need the server.
		s, recs := s.fetchRecs(true)
		switch {
		case s.Offline:
			s, out := s.probe(ProbeUser)
			return s, slices.Concat(
				out, recs,
				[]Effect{Notice{Text: "offline · the library syncs once the server is back"}},
			)
		case !s.loggedIn:
			return s, append(recs, Notice{Text: "not connected to the server yet", Err: true})
		case s.Sync.Running:
			return s, append(recs, Notice{Text: "already syncing"})
		}
		s, out := s.startSync(true)
		return s, slices.Concat(out, recs, []Effect{Notice{Text: "syncing the whole library…"}})
	case SyncProgress:
		if s.Sync.Running {
			s.Sync.Done, s.Sync.Total = e.Done, e.Total
		}
		return s, nil
	case Synced:
		s.Sync = SyncStatus{}
		wait := s.mixWait
		s.mixWait = nil
		if e.Err != nil {
			s.Sync.Failed = true
			return s, []Effect{
				Notice{
					Text:       "couldn't sync the library: " + e.Err.Error(),
					Command:    "sync",
					Hint:       "tries again",
					Err:        true,
					Unexpected: true,
				},
			}
		}
		s.Library = e.Library
		out := []Effect{SaveCache{Library: e.Library}}
		if wait != nil && wait.still(s) {
			s, more := s.applyMix(wait.mix)
			return s, append(out, more...)
		}
		return s, out

	case PlayNow:
		if e.Start < 0 || e.Start >= len(e.Tracks) {
			return s, nil
		}
		had := len(s.Queue) > 0
		s = s.edit(func(q []QueueEntry) ([]QueueEntry, int) { return nil, -1 })
		s.Queue, s.nextID = entries(s.nextID, e.Tracks)
		s.Current, s.skips = e.Start, 0
		if s.Offline || s.cantPlay() != nil {
			return s.play(s.Current) // explains why nothing plays
		}
		n := Notice{Text: "playing · " + count(len(e.Tracks)) + " in the queue"}
		if had {
			n = Notice{
				Text:    "playing · queue replaced with " + count(len(e.Tracks)),
				Command: "undo",
				Hint:    "brings the old queue back",
			}
		}
		s, out := s.play(s.Current)
		return s, append(out, n)
	case AddEnd:
		if len(e.Tracks) == 0 {
			return s, nil
		}
		var added []QueueEntry
		added, s.nextID = entries(s.nextID, e.Tracks)
		s = s.edit(
			func(q []QueueEntry) ([]QueueEntry, int) { return append(slices.Clone(q), added...), s.Current },
		)
		text := fmt.Sprintf(
			"%s added to the end of the queue (%d in queue)",
			count(len(added)),
			len(s.Queue),
		)
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
		return s, append(
			s.neighbours(),
			Notice{Text: count(len(added)) + " " + verb, Command: "undo", Hint: "undoes"},
		)
	case PlayRow:
		if e.Index < 0 || e.Index >= len(s.Queue) {
			return s, nil
		}
		s.Current, s.skips = e.Index, 0
		return s.play(e.Index)

	case PlayPause:
		switch {
		case s.Player.Playing && s.Player.Paused:
			return s, []Effect{Resume{}}
		case s.Player.Playing:
			return s, []Effect{Pause{}}
		case s.resume != nil && s.index(s.resume.entry) >= 0:
			r := s.resume
			s.Current, s.skips = s.index(r.entry), 0
			return s.playFrom(s.Current, r.pos)
		case len(s.Queue) > 0:
			s.Current, s.skips = max(s.Current, 0), 0
			return s.play(s.Current)
		}
		return s, []Effect{Notice{Text: "the queue is empty"}}
	case NextTrack:
		next := s.Current + 1
		if next >= len(s.Queue) && s.Repeat == RepeatAll && len(s.Queue) > 0 {
			next = 0
		}
		if next >= len(s.Queue) {
			return s, []Effect{Notice{Text: "end of the queue"}}
		}
		s.Current = next
		s.skips = 0
		return s.play(s.Current)
	case PrevTrack:
		first := s.Current <= 0 && s.Repeat != RepeatAll // nothing before it
		if s.Player.Playing && (s.Player.Position > 3*time.Second || first) {
			return s, []Effect{SeekTo{}}
		}
		if s.Current <= 0 && s.Repeat == RepeatAll && len(s.Queue) > 0 {
			s.Current = len(s.Queue) // round to the last row
		}
		if s.Current <= 0 {
			return s, nil
		}
		s.Current--
		s.skips = 0
		return s.play(s.Current)
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
		return s, append(
			s.neighbours(),
			Notice{
				Text:    "undone · " + count(len(s.Queue)) + " in the queue",
				Command: "redo",
				Hint:    "redoes",
			},
		)
	case Redo:
		if len(s.redo) == 0 {
			return s, []Effect{Notice{Text: "nothing to redo"}}
		}
		next := s.redo[len(s.redo)-1]
		s.undo = append(slices.Clone(s.undo), snapshot{s.Queue, s.Current})
		s.redo = s.redo[:len(s.redo)-1]
		s = s.restore(next)
		return s, append(
			s.neighbours(),
			Notice{
				Text:    "redone · " + count(len(s.Queue)) + " in the queue",
				Command: "undo",
				Hint:    "undoes",
			},
		)

	case ChangeVolume, Shuffle, CycleRepeat:
		return s.applyPlayback(e)
	case SetReplayGainMode, ConfigureReplayGain:
		return s.applyReplayGain(e)
	case Mix:
		return s.applyMix(e)
	case SimilarLoaded:
		return s.applySimilar(e)
	case LoadAlbumList, AlbumListLoaded:
		return s.applyHome(e)
	case SetStars, StarsWritten:
		return s.applyStars(e)
	case SetRating, RatingWritten:
		return s.applyRating(e)
	case Yank, Remove, Paste, Move, ClearQueue:
		return s.applyEdit(e)
	case PlaylistRemove,
		PlaylistInsert,
		PlaylistMove,
		PlaylistCreate,
		PlaylistDelete,
		PlaylistRename,
		PlaylistWritten:
		return s.applyPlaylist(e)
	case FromPlayer:
		if d, ok := e.Event.(player.Died); ok {
			return s.died(d.Err, time.Time{})
		}
		return s.fromPlayer(e.Event)
	case PlayerDied:
		return s.died(e.Err, e.At)
	case SeekAt, SetRepeat:
		return s.applyMedia(e)
	case PlayerChecked, Uncap:
		return s.applyMPV(e)
	case StartedOffline:
		s, out := s.applyOffline(e)
		s, more := s.loadSaved(false)
		return s, append(out, more...)
	case Unreachable, RetryNow, Reconnect, BufferingStalled, ProbeResult:
		return s.applyOffline(e)
	case PlayerFailed:
		return s, []Effect{Notice{Text: "player: " + e.Err.Error(), Err: true, Unexpected: true}}
	case PlayerUnresponsive:
		s.PlayerStuck = e.Stuck
		return s, nil
	case Failed:
		return s, []Effect{Notice{Text: e.What + ": " + e.Err.Error(), Err: true, Unexpected: true}}
	}
	return s, nil
}

func (s State) fromPlayer(e player.Event) (State, []Effect) {
	if s.halted {
		switch e.(type) {
		case player.TrackStarted, player.TrackEnded, player.Position, player.Buffering:
			return s, nil // mpv reporting entries it ran on to after a failure
		}
	}
	if id, ok := entryOf(e); ok && slices.Contains(s.stale, id) {
		return s, nil
	}
	switch e := e.(type) {
	case player.TrackStarted:
		s.playing, s.orphanNext = e.Entry, 0
		s.PlayerDown, s.resume = false, nil
		s.Player.Playing, s.Player.Position = true, 0
		s.listened, s.lastPos, s.scrobbled = 0, 0, false
		i := s.index(e.Entry)
		if i < 0 {
			// Undone out of the queue while it played: let it finish alone.
			return s, []Effect{SetNeighbours{}}
		}
		s.Current = i
		s.playingTrack = s.Queue[i].Track
		s.Player.Duration = s.Queue[i].Track.Duration
		out := s.neighbours()
		if !s.Offline { // scrobbles are dropped while offline
			out = append(out, Scrobble{Track: s.Queue[i].Track.ID})
		}
		return s, out
	case player.TrackEnded:
		// Stopped means Play or Stop replaced it on purpose.
		if e.Entry != s.playing || e.Reason == player.Stopped {
			return s, nil
		}
		if e.Reason == player.Failed || e.Reason == player.NoAudio {
			title := "the track"
			if i := s.index(e.Entry); i >= 0 {
				title = s.Queue[i].Track.Title
			}
			if e.Reason == player.NoAudio {
				return s.noAudio(title)
			}
			return s.streamFailed(title, e.Err)
		}
		// mpv moves on to the next entry by itself; without one, playback
		// is over, unless the entry repeats.
		i := s.index(e.Entry)
		if i >= 0 && s.replays() {
			s.Current = i
			return s.play(i)
		}
		if i < 0 || s.nextRow(i) < 0 {
			s.playing = 0
			s.Player = PlayerState{Volume: s.Player.Volume}
		}
		return s, nil
	case player.Position:
		if e.Entry != s.playing {
			return s, nil
		}
		s.Player.Position = e.Position
		if e.Position > 0 {
			s.skips, s.NoAudio = 0, false // it really plays
		}
		if e.Duration > 0 {
			s.Player.Duration = e.Duration
		}
		if d := e.Position - s.lastPos; d > 0 && d <= 2*time.Second {
			s.listened += d
		}
		s.lastPos = e.Position
		if i := s.index(e.Entry); i >= 0 && !s.scrobbled && !s.Offline && s.Player.Duration > 0 &&
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
	case player.GainTags:
		if e.Entry == s.playing {
			s.Gain = e
		}
	}
	return s, nil
}

// entryOf returns the queue entry a player event is about, if any.
func entryOf(e player.Event) (player.EntryID, bool) {
	switch e := e.(type) {
	case player.TrackStarted:
		return e.Entry, true
	case player.TrackEnded:
		return e.Entry, true
	case player.Position:
		return e.Entry, true
	}
	return 0, false
}

// startSync syncs the library. The startup sync check, which also runs
// after coming back online, asks ListenBrainz too when its list is due.
func (s State) startSync(full bool) (State, []Effect) {
	if s.Sync.Running {
		return s, nil
	}
	var recs []Effect
	if !full {
		s, recs = s.fetchRecs(false)
	}
	s.Sync = SyncStatus{Running: true}
	return s, append([]Effect{StartSync{Old: s.Library, Full: full}}, recs...)
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
	s.stale = nil // long past by the time anyone undoes
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
		if j := s.index(s.orphanNext); j >= 0 && s.orphanNext != 0 {
			sp := s.span(j)
			return []Effect{SetNeighbours{Prev: sp.Prev, Next: sp.Current}}
		}
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
	if n := s.nextRow(i); n >= 0 {
		sp.Next = s.Queue[n].entry()
	}
	return sp
}

func (s State) index(id player.EntryID) int {
	return slices.IndexFunc(s.Queue, func(q QueueEntry) bool { return q.ID == id })
}

func (q QueueEntry) entry() player.Entry {
	title := q.Track.Title
	if q.Track.Artist != "" {
		title += " — " + q.Track.Artist
	}
	return player.Entry{ID: q.ID, Track: q.Track.ID, Title: title}
}

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

func (Start) isEvent()              {}
func (LoggedIn) isEvent()           {}
func (SyncNow) isEvent()            {}
func (CacheLoaded) isEvent()        {}
func (SyncProgress) isEvent()       {}
func (Synced) isEvent()             {}
func (PlayNow) isEvent()            {}
func (AddEnd) isEvent()             {}
func (AddNext) isEvent()            {}
func (PlayRow) isEvent()            {}
func (PlayPause) isEvent()          {}
func (NextTrack) isEvent()          {}
func (PrevTrack) isEvent()          {}
func (SeekBy) isEvent()             {}
func (StopPlayback) isEvent()       {}
func (Undo) isEvent()               {}
func (Redo) isEvent()               {}
func (Scrobbled) isEvent()          {}
func (FromPlayer) isEvent()         {}
func (PlayerFailed) isEvent()       {}
func (PlayerUnresponsive) isEvent() {}
func (Failed) isEvent()             {}

func (LoadCache) isEffect()     {}
func (StartSync) isEffect()     {}
func (SaveCache) isEffect()     {}
func (Play) isEffect()          {}
func (SetNeighbours) isEffect() {}
func (Pause) isEffect()         {}
func (Resume) isEffect()        {}
func (Seek) isEffect()          {}
func (SeekTo) isEffect()        {}
func (Stop) isEffect()          {}
func (Scrobble) isEffect()      {}
func (Notice) isEffect()        {}
