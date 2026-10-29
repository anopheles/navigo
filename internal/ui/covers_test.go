package ui

import (
	"image/color"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/golden"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/cover"
	"github.com/anopheles/navigo/internal/subsonic"
	"github.com/anopheles/navigo/internal/subsonic/subsonictest"
)

// coverHarness starts navigo with cover art available, as on a terminal
// with colour, and the Albums list showing.
func coverHarness(t *testing.T, w, h int, mode string, kitty bool) *harness {
	t.Helper()
	hs := setupHarness(t, w, h, func(c *config.Config, _ *subsonictest.Server) {
		c.CoverArt.Mode = mode
	}, func(hs *harness) {
		hs.m.deps.CoverArtOK = true
		hs.m.deps.CoverKitty = kitty
		hs.m.deps.CoverCache = cover.OpenCache(t.TempDir(), 1<<20)
	})
	hs.toAlbums()
	return hs
}

// play starts the second track of the first album and goes Home, where
// the queue and its card show beside the library.
func (h *harness) play() {
	h.t.Helper()
	h.keys("3j<CR>j<CR>'h")
}

func TestCardShowsTheCover(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.fake.Tick(42 * time.Second)
	h.pump()
	if n := h.requests("getCoverArt"); n != 1 {
		t.Fatalf("%d cover requests, want 1", n)
	}
	golden.RequireEqual(t, h.screen())
}

func TestCardHasTheBarsFacts(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.fake.Tick(42 * time.Second)
	h.pump()
	h.contains("▶  Low Tide", "Aurora Lane", "Blue Hour", " 2/6 ", "0:42 / 3:05")
}

func TestCardSaysWhatPlaysNext(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.contains("2021 · Dream Pop", "next: Streetlamp — Aurora Lane")
	h.keys("RR") // repeat one: the same track again
	h.lacks("next:")
}

func TestNoCardOnASmallTerminal(t *testing.T) {
	for _, size := range [][2]int{{120, 27}, {99, 40}} {
		h := coverHarness(t, size[0], size[1], config.CoverAuto, false)
		h.play()
		if n := h.requests("getCoverArt"); n != 0 {
			t.Errorf("%dx%d asked for %d covers; nothing shows them", size[0], size[1], n)
		}
		if got, want := h.m.listHeight(queueWindow), size[1]-5-1; got != want {
			t.Errorf("%dx%d: %d body rows, want %d (no card)", size[0], size[1], got, want)
		}
	}
}

func TestCardTakesSevenQueueRows(t *testing.T) {
	plain := homeHarness(t, 120, 40, nil)
	card := coverHarness(t, 120, 40, config.CoverAuto, false)
	if d := plain.m.bodyHeight() - card.m.listHeight(queueWindow); d != 0 {
		t.Errorf("the card takes %d rows with nothing playing, want none", d)
	}
	card.play()
	if d := plain.m.bodyHeight() - card.m.listHeight(queueWindow); d != 7 {
		t.Errorf("the card takes %d queue rows, want 7", d)
	}
	if d := plain.m.bodyHeight() - card.m.listHeight(libraryWindow); d != 0 {
		t.Errorf("the card takes %d library rows, want none", d)
	}
	if got := len(strings.Split(card.m.render(), "\n")); got != 40 {
		t.Errorf("the screen is %d lines, want 40", got)
	}
}

func TestCoverArtOff(t *testing.T) {
	for name, h := range map[string]*harness{
		"configured off": coverHarness(t, 120, 40, config.CoverOff, false),
		"no colour": setupHarness(t, 120, 40, nil, func(hs *harness) {
			hs.m.deps.CoverArtOK = false
		}),
	} {
		h.toAlbums()
		h.play()
		if n := h.requests("getCoverArt"); n != 0 {
			t.Errorf("%s: %d cover requests", name, n)
		}
		if h.m.cardOn() {
			t.Errorf("%s: the card shows", name)
		}
	}
}

func TestCoverIsFetchedOncePerAlbum(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.keys(">>")
	if n := h.requests("getCoverArt"); n != 1 {
		t.Errorf("%d requests for three tracks of one album, want 1", n)
	}
	h.keys("'bhj<CR><CR>") // back to the albums, the next one, its first track
	if n := h.requests("getCoverArt"); n != 2 {
		t.Errorf("%d requests after another album, want 2", n)
	}
	var ids []string
	for _, r := range h.srv.Requests() {
		if r.Endpoint == "getCoverArt" {
			ids = append(ids, r.Params.Get("id"))
		}
	}
	if len(ids) == 2 && ids[0] == ids[1] {
		t.Errorf("both requests were for %s", ids[0])
	}
	if _, ok := h.m.deps.CoverCache.Get(ids[0]); !ok {
		t.Error("the cover was not kept on disk")
	}
}

func TestCoverFetchFailsQuietlyAndRetries(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.srv.SetDown(true)
	h.play()
	h.contains("Low Tide")
	id, _ := h.m.playingCover()
	if e := h.m.covers.entries[id]; e == nil || e.state != coverFailed {
		t.Fatalf("entry = %+v, want a failed fetch", e)
	}
	before := h.requests("getCoverArt")
	h.keys("j")
	if h.requests("getCoverArt") != before {
		t.Error("asked again at once")
	}
	h.srv.SetDown(false)
	h.clock.now = h.clock.now.Add(coverRetry + time.Second)
	h.keys("k")
	if e := h.m.covers.entries[id]; e == nil || e.state != coverReady {
		t.Errorf("entry = %+v, want the cover after the retry", e)
	}
	if strings.Contains(h.m.status(), "cover") {
		t.Errorf("a failed cover was reported: %q", h.m.status())
	}
}

func TestAlbumWithoutACoverShowsBlank(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	id, _ := h.m.playingCover()
	h.m.coverArrived(coverMsg{id: id, err: subsonic.ErrNoCover})
	if e := h.m.covers.entries[id]; e.state != coverNone {
		t.Fatalf("state = %v", e.state)
	}
	for _, l := range h.m.coverArt(id, cardCols, cardRows) {
		if strings.TrimSpace(l) != "" {
			t.Errorf("a missing cover drew %q", l)
		}
	}
	h.contains("▶  Low Tide") // the card's text stays
}

func TestCoverPopup(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.keys("gc")
	h.contains("nothing is playing")
	h.play()
	h.keys("gc")
	if h.m.popup == nil || !h.m.popup.cover {
		t.Fatal("gc did not open the cover")
	}
	if !strings.Contains(h.m.popup.title, "Low Tide — Aurora Lane") {
		t.Errorf("title = %q", h.m.popup.title)
	}
	blocks := 0
	for _, l := range h.m.popup.live() {
		blocks += strings.Count(l, "\x1b[")
	}
	if blocks == 0 {
		t.Error("the popup has no image")
	}
	h.keys("q")
	if h.m.popup != nil {
		t.Error("q left the popup open")
	}
}

func TestCoverPopupWorksOnASmallTerminal(t *testing.T) {
	h := coverHarness(t, 80, 24, config.CoverAuto, false)
	h.play()
	h.keys("gc")
	if h.m.popup == nil || h.m.cardOn() {
		t.Fatalf("popup %v, card %v", h.m.popup != nil, h.m.cardOn())
	}
	cols, rows := h.m.coverPopupSize()
	if cols != rows*2 || rows < 4 {
		t.Errorf("popup art is %dx%d", cols, rows)
	}
	lines := strings.Split(h.m.render(), "\n")
	if len(lines) != 24 {
		t.Errorf("%d lines on a 24 line terminal", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w > 80 {
			t.Errorf("line %d is %d cells wide", i, w)
		}
	}
}

func TestKittySendsTheImageOnceAndDrawsPlaceholders(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverKitty, true)
	h.play()
	if len(h.raw) != 1 || !strings.HasPrefix(h.raw[0], "\x1b_Ga=T,U=1,f=100,i=1,c=12,r=6") {
		t.Fatalf("raw = %.80q (%d)", strings.Join(h.raw, "|"), len(h.raw))
	}
	h.keys("j")
	h.keys("k")
	if len(h.raw) != 1 {
		t.Errorf("the image was sent %d times", len(h.raw))
	}
	if n := strings.Count(h.m.render(), "\U0010EEEE"); n != cardCols*cardRows {
		t.Errorf("%d placeholder cells, want %d", n, cardCols*cardRows)
	}
	h.keys("gc")
	if len(h.raw) != 2 || !strings.Contains(h.raw[1], "i=2,") {
		t.Errorf("the popup size was not sent: %d sends", len(h.raw))
	}
}

func TestKittyInTmuxGoesThroughPassthrough(t *testing.T) {
	h := setupHarness(t, 120, 40, nil, func(hs *harness) {
		hs.m.deps.CoverArtOK, hs.m.deps.CoverKitty, hs.m.deps.InTmux = true, true, true
	})
	h.toAlbums()
	h.play()
	if len(h.raw) != 1 || !strings.HasPrefix(h.raw[0], "\x1bPtmux;\x1b\x1b_G") {
		t.Errorf("raw = %.40q", strings.Join(h.raw, "|"))
	}
}

func TestKittyFreesTheOldestImage(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverKitty, true)
	h.play()
	id, _ := h.m.playingCover()
	art := h.m.covers.entries[id].art
	var last string
	for i := range keptImages + 1 {
		if cmd := h.m.sendImage(id, art, 2+i, 2); cmd != nil {
			last = cmd().(tea.RawMsg).Msg.(string)
		}
	}
	if !strings.HasPrefix(last, "\x1b_Ga=d,d=I,i=") {
		t.Errorf("the ninth image did not free the oldest: %.40q", last)
	}
	if len(h.m.covers.sent) != keptImages {
		t.Errorf("%d images kept, want %d", len(h.m.covers.sent), keptImages)
	}
}

func TestTheTerminalsBackgroundIsKept(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	id, _ := h.m.playingCover()
	h.m.coverArt(id, cardCols, cardRows)
	if h.m.covers.rendered == nil {
		t.Fatal("nothing was drawn to drop")
	}
	bg := color.RGBA{20, 20, 40, 255}
	h.send(tea.BackgroundColorMsg{Color: bg})
	if h.m.covers.bg != bg || h.m.covers.rendered != nil {
		t.Errorf(
			"bg %v, rendered %v: the drawn covers should be dropped",
			h.m.covers.bg,
			h.m.covers.rendered,
		)
	}
}

// The popup draws on every redraw; each must give the same lines, and the
// rows kept for the next draw must stay as they were.
func TestCoverPopupDrawsTheSameEveryTime(t *testing.T) {
	h := coverHarness(t, 150, 42, config.CoverAuto, false)
	h.play()
	h.keys("gc")
	first := slices.Clone(h.m.popup.live())
	for range 3 {
		if got := h.m.popup.live(); !slices.Equal(got, first) {
			t.Fatalf("the popup changed between draws: %q then %q", first[0], got[0])
		}
	}
	id, _ := h.m.playingCover()
	cols, rows := h.m.coverPopupSize()
	for i, l := range h.m.coverArt(id, cols, rows) {
		if w := ansi.StringWidth(l); w != cols {
			t.Fatalf("the kept row %d is %d cells wide, want %d: the popup padded it", i, w, cols)
		}
	}
}

func TestClickOnTheCardPicksNoRow(t *testing.T) {
	h := coverHarness(t, 120, 40, config.CoverAuto, false)
	h.play()
	h.screen()
	x := h.m.paneAt[queueWindow].x + 3
	h.send(click1(x, 3)) // the cover
	if h.m.focus == queueWindow {
		t.Fatal("a click on the card focused the queue")
	}
	h.send(click1(x, 1+cardRows+1)) // the first row under the card's rule
	if h.m.focus != queueWindow || h.m.queueCur != 0 {
		t.Errorf("focus %v, cursor %d; want the queue's first row", h.m.focus, h.m.queueCur)
	}
}
