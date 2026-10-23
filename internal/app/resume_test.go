package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/player"
	"github.com/anopheles/navigo/internal/resume"
)

// libWith is a library of the tracks t1…tn.
func libWith(n int) *library.Snapshot {
	return library.Build("https://m.example", "ann", time.Time{}, nil, nil, tracks(n), nil, nil)
}

func resumeState() State {
	s := New()
	s.ResumeOn, s.SyncOn = true, true
	return s
}

func saved(ids ...string) resume.Saved {
	v := resume.Saved{Current: -1, Volume: 100}
	for _, id := range ids {
		v.Tracks = append(v.Tracks, library.TrackID(id))
	}
	return v
}

func TestRestoreLoadsPausedAtThePosition(t *testing.T) {
	s, out := run(resumeState(), Start{}, LoggedIn{})
	if len(only[LoadSaved](out)) != 1 || len(only[LoadCache](out)) != 1 {
		t.Fatalf("startup effects = %#v", out)
	}
	sv := saved("t1", "t2", "t3")
	sv.Current, sv.Position, sv.Volume, sv.Repeat = 1, 134*time.Second, 40, int(RepeatAll)
	// The saved queue arrives before the library: it waits.
	s, out = run(s, SavedLoaded{Saved: sv, Found: true, Settings: true})
	if len(s.Queue) != 0 || len(only[SaveQueue](out)) != 0 {
		t.Fatalf("restored without a library: %d entries, %#v", len(s.Queue), out)
	}
	s, out = run(s, CacheLoaded{Library: libWith(5)})
	if len(s.Queue) != 3 || s.Current != 1 || s.Player.Volume != 40 || s.Repeat != RepeatAll {
		t.Fatalf(
			"queue %d, current %d, volume %d, repeat %v",
			len(s.Queue),
			s.Current,
			s.Player.Volume,
			s.Repeat,
		)
	}
	if len(only[Play](out)) != 0 {
		t.Error("restoring started playback")
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "restored 3 tracks" ||
		n[0].Command != "play-pause" || n[0].Hint != "plays from 2:14" {
		t.Errorf("notice = %#v", n)
	}
	if v := only[SetVolume](out); len(v) != 1 || v[0].Percent != 40 {
		t.Errorf("volume effects = %#v", v)
	}
	if s.CanUndo() {
		t.Error("the restored queue has undo history")
	}

	// <Space> plays the current entry from the saved position.
	_, out = run(s, PlayPause{})
	p := only[Play](out)
	if len(p) != 1 || p[0].Span.Current.Track != "t2" || p[0].Span.Start != 134*time.Second {
		t.Errorf("play = %#v", p)
	}
}

func TestRestoreDropsMissingTracks(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(3)})
	sv := saved("t1", "gone", "t3", "gone2")
	sv.Current, sv.Position = 1, 50*time.Second // the current one is gone
	s, out := run(s, SavedLoaded{Saved: sv, Found: true})
	if len(s.Queue) != 2 {
		t.Fatalf("queue = %d", len(s.Queue))
	}
	if n := only[Notice](out); len(n) != 1 || n[0].Text != "restored 2 of 4 tracks" {
		t.Errorf("notice = %#v", n)
	}
	// It starts from the one that followed the lost track, at its start.
	if s.Current != 1 || s.Queue[s.Current].Track.ID != "t3" {
		t.Errorf("current = %d", s.Current)
	}
	_, out = run(s, PlayPause{})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Start != 0 {
		t.Errorf("play = %#v", p)
	}
}

func TestRestoreWithNothingLeftStartsEmpty(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(2)})
	s, out := run(s, SavedLoaded{Saved: saved("gone"), Found: true})
	if len(s.Queue) != 0 || len(only[Notice](out)) != 0 {
		t.Errorf("queue %d, notices %#v", len(s.Queue), only[Notice](out))
	}
}

func TestRestoreLeavesAQueueTheUserBuiltFirst(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(5)})
	s, _ = run(s, AddEnd{Tracks: tracks(1)})
	s, out := run(s, SavedLoaded{Saved: saved("t4", "t5"), Found: true})
	if len(s.Queue) != 1 || len(only[Notice](out)) != 0 {
		t.Errorf("queue %d, notices %#v", len(s.Queue), only[Notice](out))
	}
}

func TestNothingIsSavedBeforeTheFileIsRead(t *testing.T) {
	s, out := run(resumeState(), Start{}, CacheLoaded{Library: libWith(3)})
	s, out2 := run(s, ChangeVolume{Set: true, To: 20}, AddEnd{Tracks: tracks(1)})
	if len(only[SaveQueue](append(out, out2...))) != 0 {
		t.Fatalf("saved before the saved queue was read: %#v", out2)
	}
	_, out = run(s, SavedLoaded{Found: false})
	if len(only[SaveQueue](out)) != 1 {
		t.Errorf("effects after SavedLoaded = %#v", out)
	}
}

func TestSavesWhatChanges(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(5)}, SavedLoaded{Found: false})
	s, out := run(s, PlayNow{Tracks: tracks(3), Start: 1})
	sq := only[SaveQueue](out)
	if len(sq) == 0 {
		t.Fatal("playing a queue wasn't saved")
	}
	if v := sq[len(sq)-1].Saved; len(v.Tracks) != 3 || v.Current != 1 || v.Volume != 100 {
		t.Errorf("saved = %+v", v)
	}
	// An event that changes nothing kept saves nothing.
	_, out = run(s, ChangeVolume{Set: true, To: 100})
	if len(only[SaveQueue](out)) != 0 {
		t.Errorf("saved again: %#v", out)
	}
	// Position is kept in steps of 10 s while playing, exactly once paused.
	id := s.Queue[s.Current].ID
	pos := func(d time.Duration) Event {
		return FromPlayer{player.Position{Entry: id, Position: d, Duration: 100 * time.Second}}
	}
	s, _ = run(s, FromPlayer{player.TrackStarted{Entry: id}})
	s, out = run(s, pos(4*time.Second))
	if len(only[SaveQueue](out)) != 0 {
		t.Errorf("saved after 4 s: %#v", out)
	}
	s, out = run(s, pos(12*time.Second))
	if sq := only[SaveQueue](out); len(sq) != 1 || sq[0].Saved.Position != 10*time.Second {
		t.Errorf("after 12 s: %#v", out)
	}
	_, out = run(s, PlayPause{}, FromPlayer{player.Paused{Paused: true}})
	if sq := only[SaveQueue](out); len(sq) != 1 || sq[0].Saved.Position != 12*time.Second {
		t.Errorf("after pausing: %#v", out)
	}
}

func TestEmptyingTheQueueSavesAnEmptyOne(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(2)}, SavedLoaded{Found: false})
	s, _ = run(s, AddEnd{Tracks: tracks(2)})
	_, out := run(s, ClearQueue{})
	sq := only[SaveQueue](out)
	if len(sq) != 1 || len(sq[0].Saved.Tracks) != 0 {
		t.Errorf("effects = %#v", out)
	}
}

func TestSavedQueueIsAskedForOnceTheLoginAnswered(t *testing.T) {
	s, out := run(resumeState(), Start{})
	if len(only[LoadSaved](out)) != 0 {
		t.Errorf("asked before the login answered: %#v", out)
	}
	s, out = run(s, LoggedIn{})
	if l := only[LoadSaved](out); len(l) != 1 || !l[0].Server {
		t.Errorf("after the login = %#v, want one that asks the server", l)
	}
	if _, out = run(s, LoggedIn{}); len(only[LoadSaved](out)) != 0 {
		t.Error("asked again for a session that already has its queue")
	}

	s, out = run(resumeState(), Start{}, StartedOffline{})
	if l := only[LoadSaved](out); len(l) != 1 || l[0].Server {
		t.Errorf("starting offline = %#v, want one that leaves the server alone", l)
	}
	if _, out = run(s, LoggedIn{}); len(only[LoadSaved](out)) != 0 {
		t.Error("the server was asked once it came back")
	}
}

func TestSavedQueueWithoutSettingsKeepsVolumeAndRepeat(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(3)})
	s, _ = run(s, ChangeVolume{Set: true, To: 55}, CycleRepeat{})
	repeat := s.Repeat
	sv := saved("t1", "t2")
	sv.Volume, sv.Repeat = 10, int(RepeatOne) // not this device's: ignored
	s, out := run(s, SavedLoaded{Saved: sv, Found: true})
	if len(s.Queue) != 2 || s.Player.Volume != 55 || s.Repeat != repeat {
		t.Errorf("queue %d, volume %d, repeat %v", len(s.Queue), s.Player.Volume, s.Repeat)
	}
	if len(only[SetVolume](out)) != 0 {
		t.Errorf("set the volume: %#v", out)
	}
}

func TestResumeOffSavesAndRestoresNothing(t *testing.T) {
	s, out := run(New(), Start{}, LoggedIn{})
	if len(only[LoadSaved](out)) != 0 {
		t.Error("read the saved queue with resume off")
	}
	s, out = run(s, CacheLoaded{Library: libWith(2)}, AddEnd{Tracks: tracks(2)})
	if len(only[SaveQueue](out)) != 0 {
		t.Errorf("saved with resume off: %#v", out)
	}
	// Turning it on saves from then on; turning it off deletes the file.
	s, out = run(s, ConfigureResume{On: true})
	if len(only[SaveQueue](out)) != 1 {
		t.Errorf("turned on: %#v", out)
	}
	s, out = run(s, ConfigureResume{On: false})
	if len(only[DeleteSaved](out)) != 1 {
		t.Errorf("turned off: %#v", out)
	}
	_, out = run(s, AddEnd{Tracks: tracks(1)})
	if len(only[SaveQueue](out)) != 0 {
		t.Errorf("saved after turning it off: %#v", out)
	}
}

func TestLoggedOutForgetsTheQueue(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(2)}, SavedLoaded{Found: false})
	s, _ = run(s, AddEnd{Tracks: tracks(2)})
	s, out := run(s, LoggedOut{})
	if len(only[DeleteSaved](out)) != 1 {
		t.Errorf("effects = %#v", out)
	}
	_, out = run(s, StopPlayback{}, AddEnd{Tracks: tracks(1)})
	if len(only[SaveQueue](out)) != 0 {
		t.Errorf("saved after logging out: %#v", out)
	}
}

func TestOfflineSpaceKeepsTheSavedPosition(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(3)})
	sv := saved("t1", "t2")
	sv.Current, sv.Position = 1, 90*time.Second
	s, _ = run(s, SavedLoaded{Saved: sv, Found: true})
	s, _ = run(s, Unreachable{Err: errors.New("connection refused")})
	s, out := run(s, PlayPause{})
	if len(only[Play](out)) != 0 {
		t.Fatal("played while offline")
	}
	s.Offline = false
	_, out = run(s, PlayPause{})
	if p := only[Play](out); len(p) != 1 || p[0].Span.Start != 90*time.Second {
		t.Errorf("play after reconnecting = %#v", p)
	}
}

func TestSaveNowHasTheExactPosition(t *testing.T) {
	s, _ := run(resumeState(), Start{}, CacheLoaded{Library: libWith(3)}, SavedLoaded{Found: false})
	s, _ = run(s, PlayNow{Tracks: tracks(2), Start: 0})
	id := s.Queue[0].ID
	s, _ = run(
		s,
		FromPlayer{player.TrackStarted{Entry: id}},
		FromPlayer{
			player.Position{Entry: id, Position: 27 * time.Second, Duration: 100 * time.Second},
		},
	)
	v, ok := s.SaveNow()
	if !ok || v.Position != 27*time.Second || v.Current != 0 {
		t.Errorf("SaveNow = %+v, %v", v, ok)
	}
	if _, ok := New().SaveNow(); ok {
		t.Error("SaveNow said yes with resume off")
	}
}

func TestWithoutSyncTheSavedQueueIsReadAtStartWithoutTheNetwork(t *testing.T) {
	s := resumeState()
	s.SyncOn = false
	s, out := run(s, Start{})
	if l := only[LoadSaved](out); len(l) != 1 || l[0].Server {
		t.Errorf("at start = %#v, want one that leaves the server alone", l)
	}
	if _, out = run(s, LoggedIn{}); len(only[LoadSaved](out)) != 0 {
		t.Error("asked again after the login")
	}
}

func notices(out []Effect) []string {
	var texts []string
	for _, n := range only[Notice](out) {
		texts = append(texts, n.Text)
	}
	return texts
}

func hasNote(out []Effect, part string) bool {
	for _, t := range notices(out) {
		if strings.Contains(t, part) {
			return true
		}
	}
	return false
}

func TestWithSyncTheRestoreWaitsForTheLoginOnlyForTheGrace(t *testing.T) {
	s, out := run(resumeState(), Start{})
	if len(only[LoadSaved](out)) != 0 {
		t.Fatalf("asked at start with sync on: %#v", out)
	}
	if g := only[ScheduleSavedGrace](out); len(g) != 1 || g[0].After != savedGrace {
		t.Fatalf("grace = %#v, want one of %v", g, savedGrace)
	}

	late, out := run(s, SavedGraceElapsed{})
	if l := only[LoadSaved](out); len(l) != 1 || l[0].Server {
		t.Errorf("after the grace = %#v, want this device's file only", l)
	}
	if !hasNote(out, "not synced") {
		t.Errorf("the user isn't told the queue won't sync: %q", notices(out))
	}
	if _, out = run(late, LoggedIn{}); len(only[LoadSaved](out)) != 0 {
		t.Error("asked again when the late login answered")
	}

	answered, _ := run(s, LoggedIn{})
	if _, out = run(answered, SavedGraceElapsed{}); len(only[LoadSaved](out)) != 0 {
		t.Error("the grace asked again after the login answered")
	}
}

func TestALoginThatFailsOtherwiseStillRestoresThisDevicesQueue(t *testing.T) {
	s, _ := run(resumeState(), Start{})
	_, out := run(s, LoginFailed{})
	if l := only[LoadSaved](out); len(l) != 1 || l[0].Server {
		t.Errorf("after a failed login = %#v, want this device's file only", l)
	}
	if !hasNote(out, "not synced") {
		t.Errorf("the user isn't told the queue won't sync: %q", notices(out))
	}
}

func TestStartingOfflineNeedsNoSecondNoteAboutSync(t *testing.T) {
	s, _ := run(resumeState(), Start{})
	_, out := run(s, StartedOffline{})
	if len(only[LoadSaved](out)) != 1 {
		t.Fatalf("offline start = %#v", out)
	}
	if hasNote(out, "not synced") {
		t.Errorf("offline already says why: %q", notices(out))
	}
}

func TestAServerQueueThatCouldNotBeReadIsSaidWhenRestoring(t *testing.T) {
	s, _ := run(resumeState(), Start{}, LoggedIn{})
	s, out := run(
		s,
		SavedLoaded{Saved: saved("t1", "t2"), Found: true, Settings: true, Unsynced: true},
	)
	if !hasNote(out, "not synced") {
		t.Errorf("no note when the read failed: %q", notices(out))
	}
	_, out = run(s, CacheLoaded{Library: libWith(2)})
	if !hasNote(out, "(not synced this session)") {
		t.Errorf("the restore doesn't say so: %q", notices(out))
	}
}
