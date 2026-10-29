package ui

import (
	"strings"
	"testing"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

func TestRowsHaveAHierarchy(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j") // an album's tracks, the cursor on the second
	line := lineWith(t, h.m.render(), "1 Dusk Signal")
	// The number is faint, the title plain, the artist and duration faint.
	for _, want := range []string{"\x1b[2m   1 \x1b[mDusk Signal", "\x1b[2m  Aurora Lane"} {
		if !strings.Contains(line, want) {
			t.Errorf("row lacks %q:\n%q", want, line)
		}
	}
}

// segment is the text from the first start to the next reset.
func segment(s, start string) string {
	i := strings.Index(s, start)
	if i < 0 {
		return ""
	}
	rest := s[i:]
	if j := strings.Index(rest[len(start):], "\x1b[m"); j >= 0 {
		return rest[:len(start)+j]
	}
	return rest
}

func TestCursorAndPlayingRowsStayWhole(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>j<CR>") // Low Tide plays and the library cursor is on it
	screen := h.m.render()
	if cur := segment(
		lineWith(t, screen, "2 Low Tide"),
		"\x1b[7m",
	); !strings.Contains(
		cur,
		"Aurora Lane",
	) ||
		!strings.Contains(cur, "3:05") {
		t.Errorf("the cursor row is cut by styles of its parts: %q", cur)
	}
	h.keys("<Tab>") // the queue, with the playing row
	screen = h.m.render()
	if pl := segment(lineWith(t, screen, "▶"), "\x1b[1;35m▶"); !strings.Contains(pl, "3:05") {
		t.Errorf("the playing row is cut by styles of its parts: %q", pl)
	}
}

func TestPlayedTracksAreFaint(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>jjj<CR><Tab>G") // Blue Hour, the fourth, plays; the queue cursor leaves row 1
	screen := h.m.render()
	if l := lineWith(t, screen, "1 Dusk Signal"); !strings.Contains(l, "\x1b[2m   1 Dusk Signal") {
		t.Errorf("a played track should be faint all through: %q", l)
	}
	if l := lineWith(t, screen, "5 Wintering"); strings.Contains(l, "\x1b[2m   5 Wintering") {
		t.Errorf("an upcoming track was dimmed whole: %q", l)
	}
}

func TestFindMatchesStillShowOnQuietRows(t *testing.T) {
	h := newHarness(t, 120, 40, nil)
	h.keys("3j<CR>/Aurora<CR>")
	if l := lineWith(t, h.m.render(), "3 Streetlamp"); !strings.Contains(l, ";43;") {
		t.Errorf("a find match has lost its highlight: %q", l)
	}
}

func TestStarIsGoldOnlyOnTrueColour(t *testing.T) {
	for _, truecolor := range []bool{true, false} {
		h := setupHarness(
			t,
			120,
			40,
			func(*config.Config, *subsonictest.Server) {},
			func(hs *harness) {
				hs.m.deps.TrueColor = truecolor
			},
		)
		h.toAlbums()
		h.keys("3j<CR>j*j") // star Low Tide, then move off it
		line := lineWith(t, h.m.render(), "★")
		if got := strings.Contains(line, "38;2;229;192;123"); got != truecolor {
			t.Errorf("true colour %v: gold star = %v in %q", truecolor, got, line)
		}
	}
}
