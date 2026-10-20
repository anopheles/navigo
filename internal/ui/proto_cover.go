// PROTOTYPE: throwaway code for "Where should cover art go, and how is it
// drawn?" (anopheles/navigo#60). Never merge into main.
//
// Real covers from the real server, in the real screen. Keys (prototype only):
//
//	F5  placement   off → P1 card in the bar → P2 panel over the queue →
//	                P3 header over the library list
//	F6  renderer    half-blocks (x/mosaic) ↔ kitty unicode placeholders
//	F7  state       normal → loading → missing → failed (faked)
//	F8  popup       P4: big cover over the screen
//	F10 fake transparency: rounded corners cut out through alpha
//	F9  size gate   enforce 100×28 ↔ show at any size
//
// Fetches are logged to $NAVIGO_PROTO_LOG (bytes, ms, content type) and the
// bytes cached in $NAVIGO_PROTO_COVERS (default: the temp dir).

package ui

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "image/jpeg"

	_ "golang.org/x/image/webp"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/mosaic"

	"github.com/anopheles/navigo/internal/library"
)

type protoCover struct {
	place    int // 0 off, 1 card, 2 panel, 3 header
	kitty    bool
	mode     int // 0 x/mosaic, 1 alpha-aware half-blocks, 2 kitty
	mask     bool
	bg       color.Color
	bgAsked  bool
	force    int // 0 normal, 1 loading, 2 missing, 3 failed
	popup    bool
	noGate   bool
	covers   map[string]*coverEntry
	inflight map[string]bool
	rendered map[string]string
	kittyID  map[string]int
	sent     map[string]bool
	browse   string // album under the cursor
	browseOK string // ... once the cursor rested on it
	seq      int
}

type coverEntry struct {
	img    image.Image
	masked image.Image
	err    error
	bytes  int
	ms     int64
	disk   bool
}

type (
	protoCoverMsg struct {
		key string
		e   *coverEntry
	}
	protoRestMsg struct{ seq int }
)

// shown is the image with the prototype's rounded-corner mask if it is on.
func (e *coverEntry) shown(mask bool) image.Image {
	if !mask {
		return e.img
	}
	if e.masked == nil {
		e.masked = roundCorners(e.img, 0.16)
	}
	return e.masked
}

type protoWant struct {
	id   string
	size int
	w, h int
}

func (w protoWant) key() string { return fmt.Sprintf("%s|%d", w.id, w.size) }

func (p *protoCover) init() {
	if p.covers == nil {
		p.covers = map[string]*coverEntry{}
		p.inflight = map[string]bool{}
		p.rendered = map[string]string{}
		p.kittyID = map[string]int{}
		p.sent = map[string]bool{}
	}
}

// active reports whether the placement shows at this terminal size.
func (m *Model) protoActive() bool {
	p := &m.proto
	return p.place > 0 && m.screen != screenLogin &&
		(p.noGate || (m.w >= narrow && m.h >= 28)) && m.w >= narrow
}

// protoRows is how many body rows the placement takes from the windows.
func (m *Model) protoRows() int {
	if m.protoActive() && m.proto.place == 1 {
		return 4
	}
	return 0
}

func (m *Model) protoPlayingID() string {
	t, _, ok := m.state.NowPlaying()
	if !ok {
		return ""
	}
	return string(t.ID)
}

// protoBrowseAlbum is the album the library cursor is on, or in.
func (m *Model) protoBrowseAlbum() (library.Album, bool) {
	f := m.top()
	lib := m.state.Library
	if lib == nil {
		return library.Album{}, false
	}
	switch f.kind {
	case albumList, homeAlbums:
		as := m.albumsOf(f)
		if f.cur < len(as) {
			return as[f.cur], true
		}
	case artistPage:
		if a, ok := m.artistRows(f.artist).album(f.cur); ok {
			return a, true
		}
	case sectionList, artistList, homeList, playlistList, playlistTracks, searchList, helpList:
	default:
		for _, a := range lib.Albums {
			if a.ID == f.album {
				return a, true
			}
		}
	}
	return library.Album{}, false
}

// protoWants lists the covers the placement needs right now.
func (m *Model) protoWants() []protoWant {
	var out []protoWant
	if !m.protoActive() {
		return nil
	}
	p := &m.proto
	if id := m.protoPlayingID(); id != "" {
		switch p.place {
		case 1:
			out = append(out, protoWant{id, 300, 12, 6})
		case 2:
			out = append(out, protoWant{id, 300, 20, 10})
		}
		if p.popup {
			out = append(out, protoWant{id, 600, 36, 18})
		}
	}
	if a, ok := m.protoBrowseAlbum(); ok && p.place == 3 && string(a.ID) == p.browseOK {
		out = append(out, protoWant{string(a.ID), 300, 20, 10})
	}
	return out
}

// protoPost runs after every update: it starts fetches, sends kitty images,
// and debounces the cursor.
func (m *Model) protoPost() tea.Cmd {
	p := &m.proto
	p.init()
	var cmds []tea.Cmd
	if !p.bgAsked {
		p.bgAsked = true
		cmds = append(cmds, tea.RequestBackgroundColor)
	}
	if a, ok := m.protoBrowseAlbum(); ok && string(a.ID) != p.browse {
		p.browse = string(a.ID)
		p.seq++
		seq := p.seq
		cmds = append(cmds, tea.Tick(150*time.Millisecond, func(time.Time) tea.Msg {
			return protoRestMsg{seq}
		}))
	}
	for _, w := range m.protoWants() {
		if e, ok := p.covers[w.key()]; ok {
			if p.kitty && e.img != nil && !p.sent[m.protoKittyKey(w)] {
				p.sent[m.protoKittyKey(w)] = true
				cmds = append(cmds, tea.Raw(m.protoTransmit(w, e.shown(p.mask))))
			}
			continue
		}
		if p.inflight[w.key()] || m.client == nil {
			continue
		}
		p.inflight[w.key()] = true
		cmds = append(cmds, m.protoFetch(w))
	}
	return tea.Batch(cmds...)
}

func (m *Model) protoFetch(w protoWant) tea.Cmd {
	c := m.client
	if v, err := strconv.Atoi(os.Getenv("NAVIGO_PROTO_SIZE")); err == nil {
		w.size = v // probing what the server does with a size
	}
	return func() tea.Msg {
		dir := os.Getenv("NAVIGO_PROTO_COVERS")
		if dir == "" {
			dir = filepath.Join(os.TempDir(), "navigo-proto-covers")
		}
		_ = os.MkdirAll(dir, 0o700)
		file := filepath.Join(dir, strings.ReplaceAll(w.key(), "|", "_"))
		t0 := time.Now()
		e := &coverEntry{}
		b, err := os.ReadFile(file)
		if err == nil {
			e.disk = true
		} else {
			b, _, err = c.CoverArt(context.Background(), w.id, w.size)
			if err == nil {
				_ = os.WriteFile(file, b, 0o600)
			}
		}
		e.ms, e.bytes = time.Since(t0).Milliseconds(), len(b)
		if err == nil {
			e.img, _, err = image.Decode(bytes.NewReader(b))
		}
		e.err = err
		dim := image.Point{}
		if e.img != nil {
			dim = e.img.Bounds().Size()
		}
		protoLog(fmt.Sprintf("%s id=%s asked=%d got=%v bytes=%d ms=%d disk=%v err=%v",
			time.Now().Format("15:04:05"), w.id, w.size, dim, e.bytes, e.ms, e.disk, err))
		return protoCoverMsg{w.key(), e}
	}
}

func protoLog(s string) {
	path := os.Getenv("NAVIGO_PROTO_LOG")
	if path == "" {
		path = filepath.Join(os.TempDir(), "navigo-proto-cover.log")
	}
	if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600); err == nil {
		fmt.Fprintln(f, s)
		_ = f.Close()
	}
}

// protoUpdate handles the prototype's messages and keys; ok means consumed.
func (m *Model) protoUpdate(msg tea.Msg) (tea.Cmd, bool) {
	p := &m.proto
	p.init()
	switch msg := msg.(type) {
	case protoCoverMsg:
		delete(p.inflight, msg.key)
		p.covers[msg.key] = msg.e
		return m.protoPost(), true
	case tea.BackgroundColorMsg:
		p.bg = msg.Color
		p.rendered = map[string]string{}
		protoLog(fmt.Sprintf("terminal background: %v", msg))
		return nil, true
	case protoRestMsg:
		if msg.seq == p.seq {
			p.browseOK = p.browse
		}
		return m.protoPost(), true
	case tea.KeyPressMsg:
		note := ""
		switch msg.String() {
		case "f5":
			p.place = (p.place + 1) % 4
			note = [...]string{"off", "P1 card in the bar", "P2 panel over the queue", "P3 header over the library"}[p.place]
		case "f6":
			p.mode = (p.mode + 1) % 3
			p.kitty = p.mode == 2
			note = [...]string{"x/mosaic (alpha ignored)", "alpha-aware half-blocks", "kitty placeholders"}[p.mode]
		case "f10":
			p.mask = !p.mask
			p.rendered = map[string]string{}
			note = fmt.Sprint("rounded-corner mask (fake transparency): ", p.mask)
		case "f7":
			p.force = (p.force + 1) % 4
			note = [...]string{"normal", "loading", "missing", "failed"}[p.force]
		case "f8":
			p.popup = !p.popup
			note = fmt.Sprint("P4 popup ", p.popup)
		case "f9":
			p.noGate = !p.noGate
			note = fmt.Sprint("size gate off: ", p.noGate)
		default:
			return nil, false
		}
		if p.place > 0 && !m.protoActive() {
			note += fmt.Sprintf(" — HIDDEN: needs 100×28, this is %d×%d (F9 overrides)", m.w, m.h)
		} else if p.place == 3 {
			note += " — shows on album lists and album pages"
		}
		m.setMsg("proto: "+note, false)
		return m.protoPost(), true
	}
	return nil, false
}

// ---------- drawing ----------

func (m *Model) protoKittyKey(w protoWant) string {
	return fmt.Sprintf("%s|%d|%d|%v|%d", w.key(), w.w, w.h, m.proto.mask, m.proto.mode)
}

func (m *Model) protoKittyID(w protoWant) int {
	p := &m.proto
	k := m.protoKittyKey(w)
	if id, ok := p.kittyID[k]; ok {
		return id
	}
	id := 1000 + len(p.kittyID)
	p.kittyID[k] = id
	return id
}

// protoTransmit is the kitty command that stores the image and a virtual
// placement of w×h cells under an id; inside tmux it goes through the
// passthrough, which needs `set -g allow-passthrough on`.
func (m *Model) protoTransmit(w protoWant, img image.Image) string {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	var sb strings.Builder
	for i := 0; i < len(data); i += 4096 {
		end := min(i+4096, len(data))
		more := 1
		if end == len(data) {
			more = 0
		}
		if i == 0 {
			fmt.Fprintf(&sb, "\x1b_Ga=T,U=1,f=100,i=%d,c=%d,r=%d,q=2,m=%d;%s\x1b\\",
				m.protoKittyID(w), w.w, w.h, more, data[i:end])
		} else {
			fmt.Fprintf(&sb, "\x1b_Gq=2,m=%d;%s\x1b\\", more, data[i:end])
		}
	}
	s := sb.String()
	if os.Getenv("TMUX") != "" {
		s = "\x1bPtmux;" + strings.ReplaceAll(s, "\x1b", "\x1b\x1b") + "\x1b\\"
	}
	return s
}

// protoArt is the cover as exactly w.h lines of w cells.
func (m *Model) protoArt(w protoWant) []string {
	p := &m.proto
	p.init()
	e := p.covers[w.key()]
	switch {
	case p.force == 1 || (p.force == 0 && e == nil):
		return protoBox(w.w, w.h, "loading…")
	case p.force == 2:
		return protoBox(w.w, w.h, "no cover")
	case p.force == 3 || (e != nil && e.err != nil):
		return protoBox(w.w, w.h, "✕ couldn't load")
	}
	if p.kitty {
		var out []string
		id := m.protoKittyID(w)
		fg := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", id>>16&255, id>>8&255, id&255)
		for r := range w.h {
			var sb strings.Builder
			sb.WriteString(fg)
			for c := range w.w {
				sb.WriteRune(0x10EEEE)
				sb.WriteRune(diacritics[r])
				sb.WriteRune(diacritics[c])
			}
			sb.WriteString("\x1b[39m")
			out = append(out, sb.String())
		}
		return out
	}
	k := m.protoKittyKey(w)
	s, ok := p.rendered[k]
	if !ok {
		if p.mode == 1 {
			s = alphaHalf(e.shown(p.mask), w.w, w.h, p.bg)
		} else {
			mo := mosaic.New().Width(w.w * 2).Height(w.h * 2)
			s = strings.TrimRight(mo.Render(e.shown(p.mask)), "\n")
		}
		p.rendered[k] = s
	}
	return strings.Split(s, "\n")
}

func protoBox(w, h int, msg string) []string {
	out := make([]string, h)
	for i := range out {
		out[i] = dimStyle.Render(strings.Repeat("░", w))
	}
	label := " " + msg + " "
	if lw := lipgloss.Width(label); lw <= w {
		left := (w - lw) / 2
		out[h/2] = dimStyle.Render(
			strings.Repeat("░", left) + label + strings.Repeat("░", w-lw-left),
		)
	}
	return out
}

// protoBeside puts art on the left of text lines, h rows tall, w wide.
func protoBeside(art []string, artW int, text []string, w int) []string {
	out := make([]string, len(art))
	for i := range art {
		t := ""
		if i < len(text) {
			t = text[i]
		}
		out[i] = art[i] + fit("  "+t, w-artW)
	}
	return out
}

func (m *Model) protoPlayingText() []string {
	t, row, ok := m.state.NowPlaying()
	if !ok {
		return []string{dimStyle.Render("nothing playing")}
	}
	n := ""
	if row >= 0 {
		n = fmt.Sprintf("%d/%d in queue", row+1, len(m.state.Queue))
	}
	return []string{
		"", titleOff.Render(t.Title), accentStyle.Render(t.Artist), dimStyle.Render(t.Album),
		"", dimStyle.Render(n),
	}
}

// protoCard is P1: six rows in place of the two-line bar.
func (m *Model) protoCard() []string {
	id := m.protoPlayingID()
	art := m.protoArt(protoWant{id, 300, 12, 6})
	w := m.w
	m.w -= 14
	np := m.nowPlaying()
	m.w = w
	text := m.protoPlayingText()
	text[4], text[5] = strings.TrimRight(np[0], " "), strings.TrimRight(np[1], " ")
	return protoBeside(art, 12, text, w)
}

// protoPanel is P2/P3: ten rows with the art on the left.
func (m *Model) protoPanel(w protoWant, text []string, width int) []string {
	return append(protoBeside(m.protoArt(w), w.w, text, width), dimStyle.Render(strings.Repeat("─", width)))
}

func (m *Model) protoAlbumText(a library.Album) []string {
	return []string{
		"", titleOff.Render(a.Name), accentStyle.Render(a.Artist),
		dimStyle.Render(fmt.Sprint(a.Year)), "", dimStyle.Render("the cover of what you browse"),
	}
}

// protoPopup is P4: the big cover in a box, centred by the caller.
func (m *Model) protoPopup() string {
	id := m.protoPlayingID()
	if id == "" {
		return ""
	}
	art := m.protoArt(protoWant{id, 600, 36, 18})
	return popupStyle.Render(strings.Join(art, "\n"))
}
