//go:build e2e

package e2e

import (
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

var (
	playingRE  = regexp.MustCompile(`▶ (.+?) — `)
	positionRE = regexp.MustCompile(` (\d+):(\d\d) / \d+:\d\d\b`)
	// a track row in the library window ends in its duration
	trackRowRE = regexp.MustCompile(`(?m)\d+:\d\d│`)
)

// The walking skeleton's path, until search exists: browse albums → open
// one → play its first track. The track must show in the now-playing bar
// with its position moving, and the server must list it as playing, which
// proves mpv streamed it and the start was reported.
func TestBrowseOpenPlay(t *testing.T) {
	srv := target(t)
	s := start(t, srv)

	s.waitText("<CR> open", 30*time.Second)
	s.waitFor("albums to load", 30*time.Second, func(scr string) bool {
		return !strings.Contains(scr, "loading…") && !strings.Contains(scr, "connecting")
	})
	s.keys("\r") // open the album under the cursor
	s.waitFor("the album's tracks", 30*time.Second, func(scr string) bool {
		return strings.Contains(scr, "<CR> play") && trackRowRE.MatchString(scr)
	})
	s.keys("\r") // play its first track

	scr := s.waitFor("the now-playing bar", 30*time.Second, func(scr string) bool { return playingRE.MatchString(scr) })
	title := strings.TrimSpace(playingRE.FindStringSubmatch(scr)[1])
	s.waitFor("the position to pass 0:02", 30*time.Second, func(scr string) bool {
		m := positionRE.FindStringSubmatch(scr)
		return m != nil && (m[1] != "0" || m[2] >= "02")
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		playing := nowPlaying(t, srv)
		// The bar may have cut the title short with "…".
		if slices.ContainsFunc(playing, func(p string) bool {
			return strings.HasPrefix(strings.TrimSpace(p), strings.TrimSuffix(title, "…"))
		}) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the server's now playing for %s is %q, want %q", srv.user, playing, title)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
