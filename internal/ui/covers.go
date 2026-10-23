package ui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/config"
	"github.com/anopheles/navigo/internal/cover"
	"github.com/anopheles/navigo/internal/library"
	"github.com/anopheles/navigo/internal/subsonic"
)

// The cover art of the playing track: a card in the bar when the terminal
// has room, and a large one in a popup. The image is fetched once per album
// and kept on disk; what is drawn is block characters that keep the
// image's transparency, or kitty graphics where the terminal has them.

const (
	// cardCols and cardRows are the card's art: square in cells, as a cell
	// is twice as tall as wide.
	cardCols, cardRows = 12, 6
	// coverMinW and coverMinH are the smallest terminal the card is shown on.
	coverMinW, coverMinH = narrow, 28
	// coverRetry is how long after a failed fetch the cover is asked for again.
	coverRetry = 30 * time.Second
	// keptImages is how many kitty images are kept in the terminal.
	keptImages = 8
)

type coverState int

const (
	coverLoading coverState = iota
	coverReady
	coverNone   // the server has no cover for it
	coverFailed // the fetch failed; tried again after coverRetry
)

type coverEntry struct {
	state  coverState
	art    *cover.Art
	failed time.Time
	// accent is the cover's dominant colour, if it has one.
	accent    color.Color
	hasAccent bool
}

// sentImage is an image kitty holds: which cover at which size, and its id.
type sentImage struct {
	key  string
	id   uint32
	last uint32 // the use count, to drop the least recently used
}

type covers struct {
	// accentFor is the cover the accent colour comes from; "" for the
	// configured one.
	accentFor string
	entries   map[string]*coverEntry
	bg        color.Color // the terminal's background, once it has said
	rendered  map[string][]string
	sent      []sentImage
	nextID    uint32
	uses      uint32
}

// coverMsg is a fetched cover, or why there is none.
type coverMsg struct {
	id  string
	art *cover.Art
	err error
}

// coverMode is "" when cover art is off, else config.CoverBlocks or
// config.CoverKitty. It is off with the option, or where colour is.
func (m *Model) coverMode() string {
	cfg := m.deps.Config.CoverArt.Mode
	switch {
	case cfg == config.CoverOff || !m.deps.CoverArtOK:
		return ""
	case cfg == config.CoverBlocks:
		return config.CoverBlocks
	case cfg == config.CoverKitty || m.deps.CoverKitty:
		return config.CoverKitty
	}
	return config.CoverBlocks
}

// cardOn reports whether the queue window can head with the card: cover
// art is on, the terminal is big enough, and the main screen shows.
func (m *Model) cardOn() bool {
	return m.coverMode() != "" && m.screen != screenLogin && m.w >= coverMinW &&
		m.h >= coverMinH
}

// cardHeight is how many rows the card takes from the queue window: the
// art and the rule under it, while something plays.
func (m *Model) cardHeight() int {
	if _, _, playing := m.state.NowPlaying(); playing && m.cardOn() {
		return cardRows + 1
	}
	return 0
}

// coverAccentWanted reports whether the accent takes the playing album's
// colour: playing is left to the default or set to cover, and the terminal
// shows 24-bit colour.
func (m *Model) coverAccentWanted() bool {
	p := m.deps.Config.RoleColors().Playing
	return (p == "" || p == config.CoverColor) && m.deps.TrueColor && m.coverMode() != ""
}

// syncAccent puts the playing album's colour on the accent once its cover
// is in, and the configured colour back when there is no cover colour to
// use. While a cover loads, the last accent stays, so a change of album
// doesn't flash the default.
func (m *Model) syncAccent() {
	id, playing := m.playingCover()
	target := ""
	var col color.Color
	if m.coverAccentWanted() && playing {
		e := m.covers.entries[id]
		if e == nil || e.state == coverLoading {
			return
		}
		if e.hasAccent {
			target, col = id, e.accent
		}
	}
	if target == m.covers.accentFor {
		return
	}
	m.covers.accentFor = target
	if target == "" {
		applyColors(m.deps.Config.RoleColors(), m.deps.TrueColor)
		return
	}
	playingColor = col
	accentStyle = lipgloss.NewStyle().Foreground(col).Bold(true)
	accentStops = accentFrom(col)
}

// coverID is what the server is asked for: the album of the playing track,
// so one fetch serves all its tracks.
func coverID(t library.Track) string {
	if t.AlbumID != "" {
		return string(t.AlbumID)
	}
	return string(t.ID)
}

// playingCover is the id of the playing track's cover, if a track plays.
func (m *Model) playingCover() (string, bool) {
	t, _, ok := m.state.NowPlaying()
	if !ok {
		return "", false
	}
	return coverID(t), true
}

// coverPost runs after every update: it asks for the cover the screen
// needs, and sends the kitty images that are missing.
func (m *Model) coverPost() tea.Cmd {
	mode := m.coverMode()
	if mode == "" || m.client == nil {
		return nil
	}
	id, ok := m.playingCover()
	if !ok || (!m.cardOn() && m.popup == nil && !m.coverAccentWanted()) {
		return nil
	}
	if m.covers.entries == nil {
		m.covers.entries = map[string]*coverEntry{}
	}
	e := m.covers.entries[id]
	if e != nil && e.state == coverFailed && m.clock.Now().Sub(e.failed) >= coverRetry {
		e = nil
	}
	if e == nil {
		m.covers.entries[id] = &coverEntry{}
		return m.fetchCover(id)
	}
	if e.state != coverReady || mode != config.CoverKitty {
		return nil
	}
	var cmds []tea.Cmd
	for _, g := range m.coverSizes() {
		if cmd := m.sendImage(id, e.art, g.cols, g.rows); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

type geometry struct{ cols, rows int }

// coverSizes are the sizes the screen draws the playing cover at.
func (m *Model) coverSizes() []geometry {
	var out []geometry
	if m.cardOn() {
		out = append(out, geometry{cardCols, cardRows})
	}
	if m.popup != nil && m.popup.cover {
		cols, rows := m.coverPopupSize()
		out = append(out, geometry{cols, rows})
	}
	return out
}

func (m *Model) fetchCover(id string) tea.Cmd {
	c, cache, ctx := m.client, m.deps.CoverCache, m.ctx
	return func() tea.Msg {
		b, ok := cache.Get(id)
		if !ok {
			var err error
			if b, err = c.CoverArt(ctx, id); err != nil {
				return coverMsg{id: id, err: err}
			}
		}
		art, err := cover.Decode(b)
		if err != nil {
			return coverMsg{id: id, err: err}
		}
		if !ok {
			cache.Put(id, b) // only what decoded is kept
		}
		return coverMsg{id: id, art: art}
	}
}

// coverArrived keeps a fetched cover.
func (m *Model) coverArrived(msg coverMsg) {
	e := &coverEntry{}
	switch {
	case msg.err == nil:
		e.state, e.art = coverReady, msg.art
		e.accent, e.hasAccent = msg.art.Accent()
	case errors.Is(msg.err, subsonic.ErrNoCover):
		e.state = coverNone
	case errors.Is(msg.err, context.Canceled):
		delete(m.covers.entries, msg.id)
		return
	default:
		e.state, e.failed = coverFailed, m.clock.Now()
	}
	if m.covers.entries == nil {
		m.covers.entries = map[string]*coverEntry{}
	}
	m.covers.entries[msg.id] = e
	m.covers.rendered = nil
}

// sendImage returns the command that stores a cover in the terminal at a
// size, unless it holds it already; the oldest image is freed past
// keptImages.
func (m *Model) sendImage(id string, art *cover.Art, cols, rows int) tea.Cmd {
	key := fmt.Sprintf("%s|%d|%d", id, cols, rows)
	cv := &m.covers
	cv.uses++
	for i := range cv.sent {
		if cv.sent[i].key == key {
			cv.sent[i].last = cv.uses
			return nil
		}
	}
	cv.nextID++
	img := sentImage{key, cv.nextID, cv.uses}
	var out strings.Builder
	if len(cv.sent) >= keptImages {
		oldest := 0
		for i, s := range cv.sent {
			if s.last < cv.sent[oldest].last {
				oldest = i
			}
		}
		out.WriteString(cover.Delete(cv.sent[oldest].id, m.deps.InTmux))
		cv.sent = append(cv.sent[:oldest], cv.sent[oldest+1:]...)
	}
	cv.sent = append(cv.sent, img)
	px := min(cols*2*cellPixels, cover.Side)
	out.WriteString(cover.Transmit(img.id, cols, rows, art, px, m.deps.InTmux))
	return tea.Raw(out.String())
}

// cellPixels is a guess at a cell's width in pixels, to scale what is sent
// to kitty; the image is scaled to the cells by the terminal anyway.
const cellPixels = 12

// coverArt is the playing cover as rows lines of cols cells, whatever its
// state: the image, a faint placeholder while it loads, or blank cells.
func (m *Model) coverArt(id string, cols, rows int) []string {
	blank := func(s string) []string {
		out := make([]string, rows)
		for i := range out {
			out[i] = strings.Repeat(s, cols)
		}
		return out
	}
	e := m.covers.entries[id]
	switch {
	case e == nil || e.state == coverLoading:
		out := blank("░")
		for i := range out {
			out[i] = dimStyle.Render(out[i])
		}
		return out
	case e.state != coverReady:
		return blank(" ")
	}
	if m.coverMode() == config.CoverKitty {
		for _, s := range m.covers.sent {
			if s.key == fmt.Sprintf("%s|%d|%d", id, cols, rows) {
				if rows := cover.Placeholders(s.id, cols, rows); rows != nil {
					return rows
				}
			}
		}
		return blank(" ") // not in the terminal yet, or too big for kitty
	}
	key := fmt.Sprintf("%s|%d|%d", id, cols, rows)
	if rows, ok := m.covers.rendered[key]; ok {
		return rows
	}
	art := e.art.Blocks(cols, rows, m.covers.bg)
	if m.covers.rendered == nil {
		m.covers.rendered = map[string][]string{}
	}
	m.covers.rendered[key] = art
	return art
}

// queueCard heads the queue window, w cells wide, while something plays:
// the cover on the left, then the title, artist and album; the bar below
// the windows has the rest.
func (m *Model) queueCard(w int) []string {
	if m.cardHeight() == 0 {
		return nil
	}
	t, _, _ := m.state.NowPlaying()
	id, _ := m.playingCover()
	art := m.coverArt(id, cardCols, cardRows)
	text := []string{
		"",
		m.accent(m.playIcon() + "  " + t.Title),
		titleOff.Render(t.Artist),
		dimStyle.Render(t.Album),
		"",
		"",
	}
	out := make([]string, 0, cardRows+1)
	for i := range cardRows {
		out = append(out, art[i]+" "+fit(text[i], w-cardCols-1))
	}
	return append(out, dimStyle.Render(strings.Repeat("─", max(w, 0))))
}

// coverPopupSize is the size of the cover in the popup: as large as the
// screen and kitty allow, square in cells.
func (m *Model) coverPopupSize() (cols, rows int) {
	rows = min(m.popupRows()-1, (m.w-12)/2, 30, cover.MaxCells/2)
	rows = max(rows, 4)
	return rows * 2, rows
}

// openCover shows the playing track's cover large.
func (m *Model) openCover() error {
	id, ok := m.playingCover()
	switch {
	case m.coverMode() == "":
		return errors.New("cover art is off")
	case !ok:
		return errors.New("nothing is playing")
	}
	t, _, _ := m.state.NowPlaying()
	m.openPopup(t.Title+" — "+t.Artist, nil)
	m.popup.cover = true
	m.popup.live = func() []string {
		cols, rows := m.coverPopupSize()
		lines := slices.Clone(m.coverArt(id, cols, rows)) // the rows are kept; pad a copy
		if e := m.covers.entries[id]; e != nil && e.state == coverNone {
			lines = append(
				[]string{dimStyle.Render("the server has no cover for this album")},
				lines...)
		}
		pad := strings.Repeat(" ", max((min(m.w-4, 100)-4-cols)/2, 0))
		for i := range lines {
			lines[i] = pad + lines[i]
		}
		return lines
	}
	return nil
}
