//go:build e2e

package e2e

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The scenarios work on any library, the fixture one or a live one: they
// find their way from what the screen shows, not from known names.

var (
	playingRE  = regexp.MustCompile(`[▶⏸] (.+?) — `)
	positionRE = regexp.MustCompile(` (\d+):(\d\d) / \d+:\d\d\b`)
	queueRE    = regexp.MustCompile(`(\d+)/(\d+) in queue`)
	// a track row in the library window ends in its duration
	trackRowRE = regexp.MustCompile(`(?m)\d+:\d\d│`)
	// the title and artist columns of a track row
	trackColsRE = regexp.MustCompile(`^\s+[\d-]*\s+(.+?)\s{2,}(\S.*?)\s+\d+:\d\d$`)
)

// synced waits for Home, where navigo starts, then goes to Albums and
// waits until the first run's sync has filled it.
func (s *session) synced() {
	s.t.Helper()
	s.waitText("Recently added", 30*time.Second)
	s.keys("'b")
	s.waitText(" Albums ", 5*time.Second)
	s.waitFor("the library to sync", 3*time.Minute, func(scr string) bool {
		return !strings.Contains(scr, "loading…") && !strings.Contains(scr, "connecting") && !strings.Contains(scr, "Syncing library")
	})
}

// openFirstAlbum opens the album under the cursor and waits for its tracks.
func (s *session) openFirstAlbum() {
	s.t.Helper()
	s.keys("\r")
	s.waitFor("the album's tracks", 30*time.Second, func(scr string) bool {
		return strings.Contains(scr, "<CR> play") && trackRowRE.MatchString(scr)
	})
}

// line is one line of the screen, without the queue window.
func (s *session) line(n int) string {
	lines := strings.Split(s.screen(), "\n")
	if n >= len(lines) {
		return ""
	}
	left, _, _ := strings.Cut(lines[n], "│")
	return strings.TrimRight(left, " ")
}

// position is the now-playing bar's elapsed time, or -1.
func (s *session) position() time.Duration {
	m := positionRE.FindStringSubmatch(s.screen())
	if m == nil {
		return -1
	}
	min, _ := strconv.Atoi(m[1])
	sec, _ := strconv.Atoi(m[2])
	return time.Duration(min)*time.Minute + time.Duration(sec)*time.Second
}

// playing waits until the bar shows a track whose position passes 0:02,
// and returns its title.
func (s *session) playing() string {
	s.t.Helper()
	scr := s.waitFor("the now-playing bar", 30*time.Second, func(scr string) bool { return playingRE.MatchString(scr) })
	title := strings.TrimSpace(playingRE.FindStringSubmatch(scr)[1])
	s.waitFor("the position to pass 0:02", 30*time.Second, func(string) bool { return s.position() >= 2*time.Second })
	return title
}

// serverPlays waits until the server lists title as playing for the user,
// which proves mpv streamed it and the start was reported.
func serverPlays(t *testing.T, srv server, title string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		playing := nowPlaying(t, srv)
		// The bar may have cut the title short with "…".
		if slices.ContainsFunc(playing, func(p string) bool {
			return strings.HasPrefix(strings.TrimSpace(p), strings.TrimSuffix(title, "…"))
		}) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server's now playing for %s is %q, want %q", srv.user, playing, title)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// The new user's path: search a track, go to its album, select tracks,
// queue them, play.
func TestNewUserPath(t *testing.T) {
	srv := target(t)
	s := start(t, srv)
	s.synced()

	// Learn a track and its artist from the first album, then go back.
	s.openFirstAlbum()
	m := trackColsRE.FindStringSubmatch(s.line(1))
	if m == nil {
		t.Fatalf("can't read the first track row %q", s.line(1))
	}
	title := strings.TrimSuffix(strings.TrimPrefix(m[1], "★ "), "…")
	artist := strings.Fields(strings.TrimSuffix(m[2], "…"))[0]
	s.keys("h")

	// Its title and artist together match only it, or an album of that
	// name by that artist; ga goes to the album either way.
	s.keys("'s")
	s.waitText("search:", 5*time.Second)
	query := title + " " + artist
	s.keys(query)
	s.waitText("Search “"+query+"”", 10*time.Second)
	s.waitText("── Tracks", 10*time.Second)
	s.keys("\r")
	s.keys("ga")
	s.waitText("Albums › ", 10*time.Second)
	s.waitFor("the album's tracks", 10*time.Second, func(scr string) bool { return trackRowRE.MatchString(scr) })

	s.keys("Vj")
	s.waitText("add ", 5*time.Second) // "A add 2 tracks to queue"
	s.keys("A")
	s.waitText("added to the end of the queue", 10*time.Second)

	s.keys("'q\r")
	serverPlays(t, srv, s.playing())
}

// Transport: next, previous, pause and seek, read off the now-playing bar.
func TestTransport(t *testing.T) {
	srv := target(t)
	s := start(t, srv)
	s.synced()
	s.openFirstAlbum()
	s.keys("\r")
	first := s.playing()
	n, _ := strconv.Atoi(queueRE.FindStringSubmatch(s.screen())[2])

	if n >= 2 {
		s.keys(">")
		s.waitText(fmt.Sprintf("2/%d in queue", n), 15*time.Second)
		s.keys("<") // well within 3 s of its start: the previous track
		s.waitText(fmt.Sprintf("1/%d in queue", n), 15*time.Second)
		if got := s.playing(); got != first {
			t.Errorf("after > and <: %q, want %q", got, first)
		}
	}

	s.keys(" ")
	s.waitText("⏸ ", 5*time.Second)
	time.Sleep(time.Second) // let a last position update land
	p := s.position()
	time.Sleep(1500 * time.Millisecond)
	if q := s.position(); q != p {
		t.Errorf("paused, the position moved from %v to %v", p, q)
	}
	s.keys(" ")
	s.waitFor("playback to resume", 10*time.Second, func(string) bool { return s.position() > p })

	from := s.position()
	s.keys(".")
	s.waitFor("a seek of 10 s", 10*time.Second, func(string) bool { return s.position() >= from+8*time.Second })
}

// Cold start: the second launch shows the library from the cache file at
// once and checks the server without a full sync.
func TestColdStart(t *testing.T) {
	srv := target(t)
	home := t.TempDir()
	s := startIn(t, srv, home)
	s.synced()
	firstAlbum := s.line(1)
	s.stop()

	s = startIn(t, srv, home)
	s.waitText("Recently added", 5*time.Second)
	s.keys("'b")
	s.waitFor("the cached library", 5*time.Second, func(string) bool { return s.line(1) == firstAlbum })
	run := func() string { // this launch's part of the log
		l := s.log()
		return l[strings.LastIndex(l, "# navigo started"):]
	}
	s.waitFor("the sync check", 30*time.Second, func(string) bool { return strings.Contains(run(), "endpoint=getScanStatus") })
	time.Sleep(2 * time.Second) // a full sync would start right after it
	if log := run(); strings.Contains(log, "endpoint=search3") || strings.Contains(log, "endpoint=getAlbumList2") {
		t.Errorf("the second launch synced the whole library:\n%s", log)
	}
	if strings.Contains(s.screen(), "Syncing library") {
		t.Error("the second launch shows a sync")
	}
}
