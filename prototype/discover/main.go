// PROTOTYPE: throwaway code for "How does a new user discover navigo without
// reading docs?" (anopheles/navigo#13). Never merge into main.
//
// Three discovery approaches on the vim-windows layout chosen in #8,
// switchable with F2 (or :variant a|b|c):
//
//	A  Always-on hint line: the keys that matter here, from the live binding
//	   table; prefix menus pop up at once; empty queue says what to press.
//	B  Quiet until asked: nothing on screen but "? keys"; ? opens the keys for
//	   what's under the cursor; prefix menus after 500 ms (as decided in #6).
//	C  Command palette + getting-started checklist: ":" is a fuzzy palette
//	   that shows each command's key; a 5-step checklist ticks off the
//	   acceptance walkthrough as you do it.
//
// Every variant: arrows/<CR>/<Esc>/<Tab> work, every queue change says what it
// did, "/" that finds nothing offers a library search.
//
// Run: cd prototype/discover && go run .   (best at 120x40 or wider)
package main

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// ---------- fake library ----------

type track struct {
	title, artist, album string
	no, secs             int
}

type node struct {
	title, kind string // kind: section, list, artist, album, playlist, track, text
	kids        []*node
	dyn         func() []*node
	track       *track
	artist      *node
	album       *node
}

func (n *node) children() []*node {
	if n.dyn != nil {
		return n.dyn()
	}
	return n.kids
}

func (n *node) container() bool { return n.kind != "track" && n.kind != "text" }

var (
	root, homeN, artistsN, albumsN, playlistsN, searchN, helpN, queueN *node
	allTracks                                                          []*track
	albumNode                                                          = map[string]*node{}
)

var words = strings.Fields("amber hollow static lantern tidal velvet copper meridian ghost harbor signal winter paper orbit cinder glass marrow saltwater neon violet quiet thunder rust ember echo ivory feral lunar")

func w(i int) string { s := words[i%len(words)]; return strings.ToUpper(s[:1]) + s[1:] }

func trackNode(tr *track) *node {
	alb := albumNode[tr.album]
	return &node{title: tr.title, kind: "track", track: tr, artist: alb.artist, album: alb}
}

func buildLibrary() {
	artistNames := []string{"Kestrel Lane", "The Paper Orbits", "Mira Okonkwo", "Saltwater Choir", "Juno Vale", "Ninefold", "Ash & Ivory", "Low Meridian", "Cinder Theory", "Hal Brenner Trio"}
	artistsN = &node{title: "Artists", kind: "section"}
	albumsN = &node{title: "Albums", kind: "section"}
	seed := 3
	for ai, an := range artistNames {
		a := &node{title: an, kind: "artist"}
		for k := 0; k < 2+ai%3; k++ {
			seed += 7
			for albumNode[w(seed)+" "+w(seed*3+1+seed/len(words))] != nil {
				seed++
			}
			alb := &node{title: w(seed) + " " + w(seed*3+1+seed/len(words)), kind: "album", artist: a}
			alb.album = alb
			albumNode[alb.title] = alb
			for t := 1; t <= 6+(seed%7); t++ {
				tr := &track{title: w(seed*3+t*5) + " " + w(seed+t*11+ai), artist: an, album: alb.title, no: t, secs: 140 + (seed*t*13)%200}
				allTracks = append(allTracks, tr)
				alb.kids = append(alb.kids, trackNode(tr))
			}
			a.kids = append(a.kids, alb)
			albumsN.kids = append(albumsN.kids, alb)
		}
		artistsN.kids = append(artistsN.kids, a)
	}
	playlistsN = &node{title: "Playlists", kind: "section"}
	for pi, pn := range []string{"Late night", "Running", "Discover weekly-ish", "Starred"} {
		p := &node{title: pn, kind: "playlist"}
		for i := 0; i < 12; i++ {
			p.kids = append(p.kids, trackNode(allTracks[(pi*37+i*11)%len(allTracks)]))
		}
		playlistsN.kids = append(playlistsN.kids, p)
	}
	homeN = &node{title: "Home", kind: "section"}
	for hi, hn := range []string{"Recently added", "Most played", "Recently played", "Random", "Starred"} {
		h := &node{title: hn, kind: "list"}
		for i := 0; i < 6; i++ {
			h.kids = append(h.kids, albumsN.kids[(hi*5+i*3)%len(albumsN.kids)])
		}
		homeN.kids = append(homeN.kids, h)
	}
	searchN = &node{title: "Search", kind: "section", kids: []*node{{title: "type 's (or :search <text>) to search the library", kind: "text"}}}
	helpN = &node{title: "Help", kind: "section"}
	queueN = &node{title: "Queue", kind: "queue"}
	queueN.dyn = func() []*node {
		out := make([]*node, len(queue))
		for i, tr := range queue {
			out[i] = trackNode(tr)
		}
		return out
	}
	root = &node{title: "navigo", kind: "sections", kids: []*node{homeN, artistsN, albumsN, playlistsN, searchN, helpN}}
}

// tracksOf expands a row to the tracks it stands for.
func tracksOf(n *node) []*track {
	if n.track != nil {
		return []*track{n.track}
	}
	if n.kind == "text" || n.kind == "section" {
		return nil
	}
	var out []*track
	for _, k := range n.children() {
		out = append(out, tracksOf(k)...)
	}
	return out
}

var queue []*track

// ---------- panes ----------

type frame struct {
	n        *node
	cur, off int
}

type pane struct{ stack []frame }

func (p *pane) top() *frame   { return &p.stack[len(p.stack)-1] }
func (p *pane) rows() []*node { return p.top().n.children() }
func (p *pane) move(d int) {
	f, n := p.top(), len(p.rows())
	f.cur = max(0, min(n-1, f.cur+d))
}
func (p *pane) row() *node {
	r := p.rows()
	if len(r) == 0 {
		return nil
	}
	return r[min(p.top().cur, len(r)-1)]
}

func idx(list []*node, n *node) int {
	for i, x := range list {
		if x == n {
			return i
		}
	}
	return 0
}

// ---------- commands and bindings: the one table that hints, menus, the
// palette and help are all drawn from ----------

type ctx struct {
	win         int // 0 library, 1 queue
	row         *node
	depth, qlen int
	nsel        int
	variant     int
	hasJumps    bool
}

func (c ctx) onTrack() bool { return c.row != nil && c.row.track != nil }
func (c ctx) onItem() bool  { return c.row != nil && len(tracksOf(c.row)) > 0 }
func (c ctx) lib() bool     { return c.win == 0 }

type cmd struct {
	name  string
	seqs  []string // bubbletea key strings, space-separated for sequences
	modes string   // n, v or nv
	short string   // hint-line label
	desc  string   // menus, palette, help
	group string
	when  func(c ctx) bool
}

func always(ctx) bool { return true }

var cmds = []cmd{
	{"down", []string{"j", "down"}, "nv", "down", "Move down", "Move", always},
	{"up", []string{"k", "up"}, "nv", "up", "Move up", "Move", always},
	{"first", []string{"g g", "home"}, "nv", "first", "First row", "Move", always},
	{"last", []string{"G", "end"}, "nv", "last", "Last row", "Move", always},
	{"half-down", []string{"ctrl+d", "pgdown"}, "nv", "page down", "Half a page down", "Move", always},
	{"half-up", []string{"ctrl+u", "pgup"}, "nv", "page up", "Half a page up", "Move", always},
	{"open", []string{"enter", "l", "right"}, "n", "open", "Open the row", "Move", func(c ctx) bool { return c.lib() && c.row != nil && c.row.container() && len(c.row.children()) > 0 }},
	{"leave", []string{"h", "left", "backspace"}, "n", "back out", "Back out of this list", "Move", func(c ctx) bool { return c.lib() && c.depth > 1 }},
	{"find", []string{"/"}, "n", "find", "Find in this list", "Move", always},
	{"find-back", []string{"?"}, "n", "find back", "Find backwards in this list", "Move", func(c ctx) bool { return c.variant != 1 }},
	{"find-next", []string{"n"}, "n", "next match", "Next match", "Move", always},
	{"find-prev", []string{"N"}, "n", "prev match", "Previous match", "Move", always},

	{"play-now", []string{"enter"}, "n", "play", "Play from here (replaces the queue, u undoes)", "Play", func(c ctx) bool { return c.lib() && c.onTrack() }},
	{"play-row", []string{"enter"}, "n", "play", "Play this queue entry", "Play", func(c ctx) bool { return !c.lib() && c.qlen > 0 }},
	{"play-pause", []string{"space"}, "n", "play/pause", "Play or pause", "Play", always},
	{"next-track", []string{">"}, "n", "next", "Next track", "Play", always},
	{"prev-track", []string{"<"}, "n", "prev", "Previous track", "Play", always},
	{"seek-fwd", []string{"."}, "n", "+10s", "Seek forward 10 s", "Play", always},
	{"seek-back", []string{","}, "n", "-10s", "Seek back 10 s", "Play", always},

	{"add-next", []string{"a"}, "n", "play next", "Add after the current track", "Queue", func(c ctx) bool { return c.lib() && c.onItem() }},
	{"add-end", []string{"A"}, "n", "add to queue", "Add to the end of the queue", "Queue", func(c ctx) bool { return c.lib() && c.onItem() }},
	{"remove", []string{"d d", "x", "delete"}, "n", "remove", "Remove from the queue", "Queue", func(c ctx) bool { return !c.lib() && c.qlen > 0 }},
	{"move-down", []string{"J"}, "n", "move down", "Move the entry down", "Queue", func(c ctx) bool { return !c.lib() && c.qlen > 1 }},
	{"move-up", []string{"K"}, "n", "move up", "Move the entry up", "Queue", func(c ctx) bool { return !c.lib() && c.qlen > 1 }},
	{"undo", []string{"u"}, "n", "undo", "Undo the last queue change", "Queue", always},
	{"redo", []string{"ctrl+r"}, "n", "redo", "Redo", "Queue", always},

	{"select", []string{"V", "v"}, "n", "select", "Select rows (then j/k to extend)", "Select", func(c ctx) bool { return c.onItem() }},
	{"sel-play", []string{"enter"}, "v", "play these", "Play the selection (replaces the queue)", "Select", always},
	{"sel-next", []string{"a"}, "v", "play next", "Add the selection after the current track", "Select", func(c ctx) bool { return c.lib() }},
	{"sel-end", []string{"A"}, "v", "add to queue", "Add the selection to the end of the queue", "Select", func(c ctx) bool { return c.lib() }},
	{"sel-remove", []string{"d", "x", "delete"}, "v", "remove", "Remove the selection from the queue", "Select", func(c ctx) bool { return !c.lib() }},
	{"sel-cancel", []string{"esc", "v", "V"}, "v", "cancel", "Stop selecting", "Select", always},

	{"go-album", []string{"g a"}, "n", "album", "Go to this track's album", "Go", func(c ctx) bool { return c.onTrack() }},
	{"go-artist", []string{"g r"}, "n", "artist", "Go to this track's artist", "Go", func(c ctx) bool { return c.onTrack() }},
	{"go-playing", []string{"g p"}, "n", "now playing", "Show the playing track in the queue", "Go", always},
	{"jump-back", []string{"ctrl+o"}, "n", "go back", "Go back to where you were", "Go", func(c ctx) bool { return c.hasJumps }},
	{"window", []string{"tab", "ctrl+w w"}, "n", "other window", "Switch between library and queue", "Go", always},
	{"win-left", []string{"ctrl+w h"}, "n", "library", "Library window", "Go", always},
	{"win-right", []string{"ctrl+w l"}, "n", "queue", "Queue window", "Go", always},
	{"search", []string{"' s"}, "n", "search", "Search the whole library", "Go", always},
	{"home", []string{"' h"}, "n", "home", "Home", "Go", always},
	{"artists", []string{"' a"}, "n", "artists", "Artists", "Go", always},
	{"albums", []string{"' b"}, "n", "albums", "Albums", "Go", always},
	{"playlists", []string{"' p"}, "n", "playlists", "Playlists", "Go", always},
	{"queue", []string{"' q"}, "n", "queue", "Queue window", "Go", always},
	{"help", []string{"' ?", "f1"}, "n", "help", "Every key and command", "Help", always},
	{"keys", []string{"?"}, "n", "keys", "Keys for what's under the cursor", "Help", func(c ctx) bool { return c.variant == 1 }},
	{"quit", []string{"Z Z"}, "n", "quit", "Quit navigo", "Help", always},
}

var cmdByName = map[string]*cmd{}

func init() {
	for i := range cmds {
		cmdByName[cmds[i].name] = &cmds[i]
	}
}

// vim renders a bubbletea key sequence in vim notation.
func vim(seq string) string {
	var out []string
	for _, k := range strings.Fields(seq) {
		switch {
		case k == "enter":
			k = "<CR>"
		case k == "space":
			k = "<Space>"
		case k == "esc":
			k = "<Esc>"
		case k == "tab":
			k = "<Tab>"
		case k == "backspace":
			k = "<BS>"
		case k == "delete":
			k = "<Del>"
		case k == "f1":
			k = "<F1>"
		case k == "up" || k == "down" || k == "left" || k == "right" || k == "home" || k == "end" || k == "pgup" || k == "pgdown":
			k = "<" + strings.ToUpper(k[:1]) + k[1:] + ">"
		case strings.HasPrefix(k, "ctrl+"):
			k = "<C-" + k[5:] + ">"
		}
		out = append(out, k)
	}
	return strings.Join(out, "")
}

func keyOf(name string) string { return vim(cmdByName[name].seqs[0]) }

// lookup finds the command a full key sequence runs here, and whether seq is
// a prefix of a longer binding that applies here.
func lookup(seq, mode string, c ctx) (name string, prefix bool) {
	for _, cm := range cmds {
		if !strings.Contains(cm.modes, mode) || !cm.when(c) {
			continue
		}
		for _, s := range cm.seqs {
			if s == seq && name == "" {
				name = cm.name
			}
			if strings.HasPrefix(s, seq+" ") {
				prefix = true
			}
		}
	}
	return
}

type follower struct{ key, desc string }

// followers lists what can come after a pending prefix, for prefix menus.
func followers(pend, mode string, c ctx) []follower {
	var out []follower
	for _, cm := range cmds {
		if !strings.Contains(cm.modes, mode) || !cm.when(c) {
			continue
		}
		for _, s := range cm.seqs {
			if strings.HasPrefix(s, pend+" ") {
				out = append(out, follower{vim(strings.TrimPrefix(s, pend+" ")), cm.desc})
			}
		}
	}
	return out
}

// ---------- model ----------

type tickMsg time.Time
type whichMsg int

type snap struct {
	q       []*track
	playing int
}

type jump struct {
	stack []frame
	foc   int
	qcur  int
}

type model struct {
	w, h    int
	variant int

	lib     pane
	qp      pane
	foc     int
	saved   map[*node][]frame
	jumps   []jump
	jpos    int
	undo    []snap
	redo    []snap
	vanchor int

	playing, elapsed int
	paused           bool

	mode, buf, pend, last, msg, offer string
	pendID                            int
	showWhich                         bool
	popup                             string // "", "keys", "palette"
	pal                               string
	palCur                            int
	fromPalette                       bool
	done                              map[string]bool
}

func newModel() model {
	return model{
		playing: -1, mode: "NORMAL",
		lib:   pane{stack: []frame{{n: root}, {n: homeN}}},
		qp:    pane{stack: []frame{{n: queueN}}},
		saved: map[*node][]frame{},
		done:  map[string]bool{},
	}
}

func (m *model) focused() *pane {
	if m.foc == 1 {
		return &m.qp
	}
	return &m.lib
}

func (m *model) ctx() ctx {
	p := m.focused()
	c := ctx{win: m.foc, row: p.row(), depth: len(m.lib.stack) - 1, qlen: len(queue), variant: m.variant, hasJumps: len(m.jumps) > 0}
	if c.row != nil && c.row.kind == "text" {
		c.row = nil
	}
	if m.mode == "VISUAL" {
		c.nsel = len(m.selection())
	}
	return c
}

func (m model) Init() tea.Cmd { return tick() }

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		if m.playing >= 0 && m.playing < len(queue) && !m.paused {
			m.elapsed++
			if m.elapsed >= queue[m.playing].secs {
				m.step(1)
			}
		}
		return m, tick()
	case whichMsg:
		if int(msg) == m.pendID && m.pend != "" {
			m.showWhich = true
		}
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *model) step(d int) {
	if len(queue) == 0 {
		return
	}
	m.playing = max(0, min(len(queue)-1, m.playing+d))
	m.elapsed, m.paused = 0, false
}

func (m model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "f2":
		m.variant = (m.variant + 1) % 3
		m.popup, m.pend, m.showWhich = "", "", false
		m.msg = "variant " + variantNames[m.variant]
		return m, nil
	case "ctrl+c":
		m.mode, m.pend, m.popup, m.showWhich = "NORMAL", "", "", false
		m.msg = "type :q to quit"
		return m, nil
	}

	if m.popup == "keys" {
		m.popup = ""
		if s == "esc" || s == "q" || s == "?" {
			return m, nil
		}
		// any other key falls through and runs, so the menu is also a launcher
	}
	if m.popup == "palette" {
		return m.paletteKey(k)
	}
	if m.mode == "COMMAND" || m.mode == "LIBSEARCH" || m.mode == "FIND" {
		return m.lineKey(k)
	}

	if m.offer != "" {
		q := m.offer
		m.offer = ""
		if s == "enter" {
			m.runSearch(q)
			return m, nil
		}
	}

	m.msg = ""
	if s == ":" && m.pend == "" {
		if m.variant == 2 {
			m.popup, m.pal, m.palCur = "palette", "", 0
		} else {
			m.mode, m.buf = "COMMAND", ""
		}
		return m, nil
	}

	mode := "n"
	if m.mode == "VISUAL" {
		mode = "v"
	}
	seq := s
	if m.pend != "" {
		seq = m.pend + " " + s
	}
	if s == "esc" && m.pend != "" {
		m.pend, m.showWhich = "", false
		return m, nil
	}
	name, prefix := lookup(seq, mode, m.ctx())
	if prefix && name == "" {
		m.pend = seq
		m.pendID++
		if m.variant == 0 {
			m.showWhich = true
			return m, nil
		}
		id := m.pendID
		return m, tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return whichMsg(id) })
	}
	m.pend, m.showWhich = "", false
	if name == "" {
		if seq != s || s == "esc" {
			return m, nil // unknown sequence: dropped, as decided in #6
		}
		return m, nil
	}
	m.fromPalette = false
	return m.exec(name, "")
}

func (m model) lineKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	switch s {
	case "esc":
		m.mode = "NORMAL"
	case "enter":
		line, mode := m.buf, m.mode
		m.mode = "NORMAL"
		switch mode {
		case "COMMAND":
			m.fromPalette = false
			return m.runLine(line)
		case "LIBSEARCH":
			m.runSearch(line)
		case "FIND":
			m.last = line
			m.find(1, false)
		}
	case "tab":
		if mode := m.mode; mode == "COMMAND" {
			if c := completions(m.buf, m.ctx()); len(c) > 0 {
				m.buf = c[0].name + " "
			}
		}
	case "backspace":
		if m.buf == "" {
			m.mode = "NORMAL"
		} else {
			r := []rune(m.buf)
			m.buf = string(r[:len(r)-1])
		}
	case "ctrl+u":
		m.buf = ""
	default:
		m.buf += k.Text
	}
	return m, nil
}

// lineCmds are commands with arguments or no key, typeable at ":" only.
var lineCmds = []cmd{
	{name: "search", desc: "search <text>: search the whole library"},
	{name: "variant", desc: "variant a|b|c: switch prototype variant"},
	{name: "forward", desc: "Go forward again (after <C-o>)"},
	{name: "hints", desc: "hints on|off: the hint line (variant A)"},
	{name: "q", desc: "Quit navigo"},
}

func completions(buf string, c ctx) []cmd {
	word := strings.Fields(buf + "x")[0]
	word = strings.TrimSuffix(word, "x")
	if strings.Contains(buf, " ") {
		return nil
	}
	var out []cmd
	seen := map[string]bool{}
	for _, cm := range append(append([]cmd{}, lineCmds...), cmds...) {
		if seen[cm.name] || !strings.HasPrefix(cm.name, word) || (cm.when != nil && !cm.when(c)) || cm.modes == "v" {
			continue
		}
		seen[cm.name] = true
		out = append(out, cm)
	}
	return out
}

func (m model) runLine(line string) (tea.Model, tea.Cmd) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return m, nil
	}
	arg := strings.TrimSpace(strings.TrimPrefix(line, f[0]))
	switch f[0] {
	case "q", "quit", "qa":
		return m, tea.Quit
	case "variant":
		if arg != "" {
			m.variant = max(0, min(2, int(arg[0]-'a')))
		}
		return m, nil
	case "search":
		if arg == "" {
			m.mode, m.buf = "LIBSEARCH", ""
			return m, nil
		}
		m.runSearch(arg)
		return m, nil
	case "forward":
		m.jumpFwd()
		return m, nil
	case "hints":
		hintsOff = arg == "off"
		return m, nil
	}
	if cmdByName[f[0]] != nil {
		return m.exec(f[0], arg)
	}
	m.msg = "unknown command: " + line + "  (<Tab> completes, :help lists all)"
	return m, nil
}

var hintsOff bool

func (m *model) find(dir int, skip bool) {
	if m.last == "" {
		return
	}
	p := m.focused()
	rows := p.rows()
	start := p.top().cur
	if skip {
		start += dir
	}
	for i := 0; i < len(rows); i++ {
		j := ((start+dir*i)%len(rows) + len(rows)) % len(rows)
		if strings.Contains(strings.ToLower(rows[j].title), strings.ToLower(m.last)) {
			m.pushJump()
			p.top().cur = j
			return
		}
	}
	m.offer = m.last
	m.msg = fmt.Sprintf("%q is not in this list · <CR> searches the whole library for it", m.last)
}

func (m *model) runSearch(q string) {
	ql := strings.ToLower(q)
	searchN.kids = nil
	for _, a := range artistsN.kids {
		if strings.Contains(strings.ToLower(a.title), ql) {
			searchN.kids = append(searchN.kids, a)
		}
	}
	for _, a := range albumsN.kids {
		if strings.Contains(strings.ToLower(a.title), ql) {
			searchN.kids = append(searchN.kids, a)
		}
	}
	for _, t := range allTracks {
		if strings.Contains(strings.ToLower(t.title), ql) {
			searchN.kids = append(searchN.kids, trackNode(t))
		}
	}
	if len(searchN.kids) == 0 {
		searchN.kids = []*node{{title: "nothing matches " + q, kind: "text"}}
	}
	searchN.title = "Search: " + q
	m.gotoSection(searchN)
	m.lib.stack[1].cur = 0
	m.done["search"] = true
	m.msg = fmt.Sprintf("%d results for %q", len(searchN.kids), q)
}

// ---------- jumps and sections ----------

func (m *model) here() jump {
	return jump{stack: append([]frame(nil), m.lib.stack...), foc: m.foc, qcur: m.qp.top().cur}
}

func (m *model) pushJump() {
	m.jumps = append(m.jumps[:min(m.jpos, len(m.jumps))], m.here())
	m.jpos = len(m.jumps)
}

func (m *model) restore(j jump) {
	m.lib.stack = append([]frame(nil), j.stack...)
	m.foc = j.foc
	m.qp.top().cur = j.qcur
}

func (m *model) jumpBack() {
	if m.jpos == len(m.jumps) {
		m.jumps = append(m.jumps, m.here())
	}
	if m.jpos == 0 {
		m.msg = "already at the oldest place"
		return
	}
	m.jpos--
	m.restore(m.jumps[m.jpos])
	m.done["back"] = true
	m.msg = "back to " + crumbs(m.lib.stack) + "  (:forward goes forward again)"
}

func (m *model) jumpFwd() {
	if m.jpos >= len(m.jumps)-1 {
		m.msg = "already at the newest place"
		return
	}
	m.jpos++
	m.restore(m.jumps[m.jpos])
}

func (m *model) gotoSection(n *node) {
	m.pushJump()
	cur := m.lib.stack[1].n
	m.saved[cur] = append([]frame(nil), m.lib.stack...)
	if st, ok := m.saved[n]; ok && n != searchN {
		m.lib.stack = append([]frame(nil), st...)
	} else {
		m.lib.stack = []frame{{n: root, cur: idx(root.kids, n)}, {n: n}}
	}
	m.foc = 0
}

func (m *model) gotoAlbum(r *node, artist bool) {
	m.pushJump()
	a, alb := r.artist, r.album
	st := []frame{{n: root, cur: idx(root.kids, artistsN)}, {n: artistsN, cur: idx(artistsN.kids, a)}, {n: a, cur: idx(a.kids, alb)}}
	if !artist {
		cur := 0
		for i, k := range alb.kids {
			if k.track == r.track {
				cur = i
			}
		}
		st = append(st, frame{n: alb, cur: cur})
	}
	m.lib.stack = st
	m.foc = 0
	m.done["album"] = !artist || m.done["album"]
}

// ---------- queue edits ----------

func (m *model) save() {
	m.undo = append(m.undo, snap{append([]*track(nil), queue...), m.playing})
	m.redo = nil
}

func plural(n int) string {
	if n == 1 {
		return "1 track"
	}
	return fmt.Sprintf("%d tracks", n)
}

func (m *model) addNext(ts []*track) {
	m.save()
	at := len(queue)
	if m.playing >= 0 {
		at = m.playing + 1
	}
	queue = append(queue[:at], append(append([]*track(nil), ts...), queue[at:]...)...)
	m.done["add"] = true
	m.msg = plural(len(ts)) + " will play next · u undoes"
	if m.playing < 0 {
		m.msg = plural(len(ts)) + " added to the queue · <Space> plays · u undoes"
	}
}

func (m *model) addEnd(ts []*track) {
	m.save()
	queue = append(queue, ts...)
	m.done["add"] = true
	m.msg = fmt.Sprintf("%s added to the end of the queue (%d in queue) · u undoes", plural(len(ts)), len(queue))
	if m.playing < 0 {
		m.msg += " · <Space> plays"
	}
}

func (m *model) playNow(ts []*track, at int) {
	m.save()
	queue = append([]*track(nil), ts...)
	m.playing, m.elapsed, m.paused = at, 0, false
	m.done["play"] = true
	m.msg = fmt.Sprintf("playing · queue replaced with %s · u brings the old queue back", plural(len(ts)))
}

func (m *model) selection() []*node {
	p := m.focused()
	rows := p.rows()
	a, b := m.vanchor, p.top().cur
	if a > b {
		a, b = b, a
	}
	if b >= len(rows) {
		return nil
	}
	return rows[a : b+1]
}

func (m *model) selRange() (int, int) {
	a, b := m.vanchor, m.focused().top().cur
	if a > b {
		a, b = b, a
	}
	return a, b
}

func selTracks(rows []*node) []*track {
	var ts []*track
	for _, r := range rows {
		ts = append(ts, tracksOf(r)...)
	}
	return ts
}

// ---------- exec: every key, palette entry and ":" line lands here ----------

func (m model) exec(name, arg string) (tea.Model, tea.Cmd) {
	p := m.focused()
	r := p.row()
	c := m.ctx()
	if cm := cmdByName[name]; cm != nil && cm.when != nil && !cm.when(c) {
		m.msg = name + ": not here"
		return m, nil
	}
	switch name {
	case "down":
		p.move(1)
	case "up":
		p.move(-1)
	case "first":
		m.pushJump()
		p.top().cur = 0
	case "last":
		m.pushJump()
		p.move(1 << 20)
	case "half-down":
		p.move(m.bodyH() / 2)
	case "half-up":
		p.move(-m.bodyH() / 2)
	case "open":
		p.stack = append(p.stack, frame{n: r})
	case "leave":
		p.stack = p.stack[:len(p.stack)-1]
	case "find", "find-back":
		m.mode, m.buf = "FIND", ""
		if name == "find-back" {
			m.msg = "(searching backwards)"
		}
	case "find-next":
		m.find(1, true)
	case "find-prev":
		m.find(-1, true)
	case "play-now":
		var ts []*track
		at := 0
		for _, x := range p.rows() {
			if x.track != nil {
				if x == r {
					at = len(ts)
				}
				ts = append(ts, x.track)
			}
		}
		m.playNow(ts, at)
	case "play-row":
		m.playing, m.elapsed, m.paused = p.top().cur, 0, false
		m.done["play"] = true
	case "play-pause":
		if m.playing < 0 {
			if len(queue) == 0 {
				m.msg = "the queue is empty · find something and press <CR> or A"
				break
			}
			m.playing, m.elapsed = 0, 0
			m.done["play"] = true
		} else {
			m.paused = !m.paused
			m.done["play"] = true
		}
	case "next-track":
		m.step(1)
	case "prev-track":
		m.step(-1)
	case "seek-fwd":
		m.elapsed += 10
	case "seek-back":
		m.elapsed = max(0, m.elapsed-10)
	case "add-next":
		m.addNext(tracksOf(r))
	case "add-end":
		m.addEnd(tracksOf(r))
	case "remove":
		m.save()
		i := p.top().cur
		queue = append(queue[:i], queue[i+1:]...)
		if i < m.playing {
			m.playing--
		} else if i == m.playing {
			m.playing = min(m.playing, len(queue)-1)
		}
		p.move(0)
		m.msg = "removed 1 track · u undoes"
	case "move-down", "move-up":
		i, d := p.top().cur, 1
		if name == "move-up" {
			d = -1
		}
		if j := i + d; j >= 0 && j < len(queue) {
			m.save()
			queue[i], queue[j] = queue[j], queue[i]
			switch m.playing {
			case i:
				m.playing = j
			case j:
				m.playing = i
			}
			p.top().cur = j
		}
	case "undo", "redo":
		from, to := &m.undo, &m.redo
		if name == "redo" {
			from, to = to, from
		}
		if len(*from) == 0 {
			m.msg = "nothing to " + name
			break
		}
		*to = append(*to, snap{append([]*track(nil), queue...), m.playing})
		s := (*from)[len(*from)-1]
		*from = (*from)[:len(*from)-1]
		queue, m.playing = s.q, s.playing
		m.qp.move(0)
		m.msg = fmt.Sprintf("%s: queue has %s · <C-r> redoes", name, plural(len(queue)))
	case "select":
		m.mode, m.vanchor = "VISUAL", p.top().cur
	case "sel-play", "sel-next", "sel-end":
		ts := selTracks(m.selection())
		if len(m.selection()) > 1 {
			m.done["select"] = true
		}
		m.mode = "NORMAL"
		switch name {
		case "sel-play":
			m.playNow(ts, 0)
		case "sel-next":
			m.addNext(ts)
		default:
			m.addEnd(ts)
		}
	case "sel-remove":
		a, b := m.selRange()
		m.save()
		queue = append(queue[:a], queue[b+1:]...)
		if m.playing > b {
			m.playing -= b - a + 1
		} else if m.playing >= a {
			m.playing = min(a, len(queue)-1)
		}
		p.top().cur = a
		p.move(0)
		m.mode = "NORMAL"
		m.msg = "removed " + plural(b-a+1) + " · u undoes"
	case "sel-cancel":
		m.mode = "NORMAL"
	case "go-album", "go-artist":
		m.gotoAlbum(r, name == "go-artist")
	case "go-playing":
		if m.playing < 0 {
			m.msg = "nothing is playing"
			break
		}
		m.pushJump()
		m.foc, m.qp.top().cur = 1, m.playing
	case "jump-back":
		m.jumpBack()
	case "window":
		m.foc = 1 - m.foc
	case "win-left":
		m.foc = 0
	case "win-right", "queue":
		if name == "queue" {
			m.pushJump()
		}
		m.foc = 1
	case "search":
		if arg != "" {
			m.runSearch(arg)
		} else {
			m.mode, m.buf = "LIBSEARCH", ""
		}
	case "home":
		m.gotoSection(homeN)
	case "artists":
		m.gotoSection(artistsN)
	case "albums":
		m.gotoSection(albumsN)
	case "playlists":
		m.gotoSection(playlistsN)
	case "help":
		m.gotoSection(helpN)
	case "keys":
		m.popup = "keys"
	case "quit":
		return m, tea.Quit
	}
	if m.fromPalette && m.msg == "" || m.fromPalette && !strings.Contains(m.msg, "tip") {
		if k := cmdByName[name]; k != nil && k.seqs != nil {
			m.msg = strings.TrimSpace(m.msg + "   tip: next time press " + vim(k.seqs[0]))
		}
	}
	return m, nil
}

// ---------- palette (variant C) ----------

type palItem struct {
	name, key, desc string
	here            bool
}

func fuzzy(q, s string) bool {
	q, s = strings.ToLower(q), strings.ToLower(s)
	for _, r := range q {
		i := strings.IndexRune(s, r)
		if i < 0 {
			return false
		}
		s = s[i+1:]
	}
	return true
}

func (m *model) palItems() []palItem {
	c := m.ctx()
	mode := "n"
	if m.mode == "VISUAL" {
		mode = "v"
	}
	q := strings.Fields(m.pal + " ")
	word := ""
	if len(q) > 0 {
		word = q[0]
	}
	var out []palItem
	for _, cm := range cmds {
		if !strings.Contains(cm.modes, mode) || cm.group == "Move" && word == "" {
			continue
		}
		if word != "" && !fuzzy(word, cm.name+" "+cm.desc) {
			continue
		}
		out = append(out, palItem{cm.name, vim(cm.seqs[0]), cm.desc, cm.when(c)})
	}
	for _, cm := range lineCmds {
		if word == "" || fuzzy(word, cm.name+" "+cm.desc) {
			out = append(out, palItem{cm.name, ":" + cm.name, cm.desc, true})
		}
	}
	// usable here first, then whole-word hits before scattered-letter hits
	score := func(it palItem) int {
		s := 0
		if !it.here {
			s += 2
		}
		if word != "" && !strings.Contains(strings.ToLower(it.name+" "+it.desc), strings.ToLower(word)) {
			s++
		}
		return s
	}
	sort.SliceStable(out, func(i, j int) bool { return score(out[i]) < score(out[j]) })
	return out
}

func (m model) paletteKey(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	items := m.palItems()
	switch s := k.String(); s {
	case "esc":
		m.popup = ""
	case "down", "ctrl+n", "tab":
		m.palCur = min(len(items)-1, m.palCur+1)
	case "up", "ctrl+p", "shift+tab":
		m.palCur = max(0, m.palCur-1)
	case "backspace":
		if m.pal == "" {
			m.popup = ""
		} else {
			r := []rune(m.pal)
			m.pal, m.palCur = string(r[:len(r)-1]), 0
		}
	case "enter":
		m.popup = ""
		f := strings.Fields(m.pal)
		// "search foo" typed out: run it as a command line
		if len(f) > 1 {
			return m.runLine(m.pal)
		}
		if len(items) == 0 {
			return m, nil
		}
		it := items[min(m.palCur, len(items)-1)]
		if !it.here {
			m.msg = it.desc + ": not available here"
			return m, nil
		}
		if strings.HasPrefix(it.key, ":") {
			if it.name == "search" {
				m.mode, m.buf = "LIBSEARCH", ""
				return m, nil
			}
			return m.runLine(it.name)
		}
		m.fromPalette = true
		return m.exec(it.name, "")
	default:
		m.pal += k.Text
		m.palCur = 0
	}
	return m, nil
}

// ---------- rendering ----------

var (
	sel     = lipgloss.NewStyle().Reverse(true)
	selDim  = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	vis     = lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("15"))
	dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keySt   = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	okSt    = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	bar     = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	tabOn   = lipgloss.NewStyle().Reverse(true).Bold(true)
	protoSt = lipgloss.NewStyle().Background(lipgloss.Color("166")).Foreground(lipgloss.Color("0")).Bold(true)
	box     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("81")).Padding(0, 1)
)

var variantNames = []string{"A hint line", "B quiet, ? asks", "C palette + checklist"}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func dur(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }

// rowPlain renders a row without styles so selection colours stay clean.
func (m *model) rowPlain(n, parent *node, i, w int) string {
	if n.track == nil {
		label := " " + n.title
		if parent == searchN && n.kind != "text" {
			label = " " + n.kind + ": " + n.title
		}
		if c := len(n.children()); c > 0 && n.kind != "text" {
			return fit(label, w-6) + fmt.Sprintf("%5d ", c)
		}
		return fit(label, w)
	}
	t := n.track
	mark := "  "
	if parent == queueN && i == m.playing {
		mark = "▶ "
	}
	if w < 40 {
		return fit(mark+t.title, w)
	}
	num := fmt.Sprintf("%2d ", t.no)
	if parent.kind != "album" {
		num = fmt.Sprintf("%2d ", i+1)
	}
	rest := w - 2 - 3 - 6
	tw := rest * 5 / 9
	return mark + num + fit(t.title, tw) + fit(" "+t.artist, rest-tw) + fmt.Sprintf("%6s", dur(t.secs))
}

func (m *model) list(p *pane, w, h int, focused bool) []string {
	f := p.top()
	rows := f.n.children()
	if f.cur >= len(rows) {
		f.cur = max(0, len(rows)-1)
	}
	if f.cur < f.off {
		f.off = f.cur
	}
	if f.cur >= f.off+h {
		f.off = f.cur - h + 1
	}
	va, vb := -1, -1
	if focused && m.mode == "VISUAL" {
		va, vb = m.selRange()
	}
	out := make([]string, 0, h)
	for i := f.off; i < len(rows) && len(out) < h; i++ {
		line := m.rowPlain(rows[i], f.n, i, w)
		switch {
		case i == f.cur && focused:
			line = sel.Render(line)
		case i >= va && i <= vb:
			line = vis.Render(line)
		case i == f.cur:
			line = selDim.Render(line)
		case rows[i].kind == "text":
			line = dim.Render(line)
		case f.n == queueN && i == m.playing:
			line = accent.Render(line)
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

func crumbs(st []frame) string {
	var parts []string
	for _, f := range st[1:] {
		parts = append(parts, f.n.title)
	}
	return strings.Join(parts, " › ")
}

func (m *model) nowPlaying(w int) []string {
	if m.playing < 0 || m.playing >= len(queue) {
		return []string{bar.Render(fit(" ■ nothing playing", w)), bar.Render(strings.Repeat(" ", w))}
	}
	t := queue[m.playing]
	icon := "▶"
	if m.paused {
		icon = "⏸"
	}
	l1 := fmt.Sprintf(" %s %s — %s · %s", icon, t.title, t.artist, t.album)
	right := fmt.Sprintf("%d/%d in queue ", m.playing+1, len(queue))
	l1 = fit(l1, w-len(right)) + right
	times := fmt.Sprintf(" %s / %s ", dur(m.elapsed), dur(t.secs))
	pw := w - len(times) - 2
	fill := min(pw, pw*m.elapsed/max(t.secs, 1))
	l2 := " " + accent.Render(strings.Repeat("━", fill)) + dim.Render(strings.Repeat("─", pw-fill)) + times + " "
	return []string{bar.Render(l1), bar.Render(l2)}
}

func hint(key, label string) string { return keySt.Render(key) + " " + dim.Render(label) }

// hintLine is variant A: the keys that matter here, from the binding table.
func (m *model) hintLine(w int) string {
	c := m.ctx()
	var names []string
	switch {
	case m.pend != "":
		return fit(dim.Render(" waiting for the next key · ")+hint("<Esc>", "cancel"), w)
	case m.mode == "VISUAL":
		n := plural(len(selTracks(m.selection())))
		parts := []string{hint("j/k", "extend")}
		if c.lib() {
			parts = append(parts, hint("A", "add "+n+" to queue"), hint("a", "play next"), hint("<CR>", "play "+n))
		} else {
			parts = append(parts, hint("d", "remove "+n), hint("<CR>", "play "+n))
		}
		parts = append(parts, hint("<Esc>", "cancel"))
		return fit(" "+strings.Join(parts, "  "), w)
	case c.lib() && c.onTrack():
		names = []string{"play-now", "add-end", "add-next", "select", "go-album", "jump-back", "find", "search", "window", "help"}
	case c.lib():
		names = []string{"open", "leave", "add-end", "select", "jump-back", "find", "search", "window", "help"}
	case c.qlen == 0:
		names = []string{"window", "search", "albums", "help"}
	default:
		names = []string{"play-row", "play-pause", "remove", "move-down", "move-up", "undo", "select", "jump-back", "window", "help"}
	}
	var parts []string
	for _, n := range names {
		cm := cmdByName[n]
		if cm.when(c) {
			parts = append(parts, hint(vim(cm.seqs[0]), cm.short))
		}
	}
	return fit(" "+strings.Join(parts, "  "), w)
}

func (m *model) status(w int) string {
	tag := protoSt.Render(fmt.Sprintf(" PROTOTYPE %s · F2 ", variantNames[m.variant]))
	right := ""
	if m.pend != "" {
		right = vim(m.pend) + "  "
	}
	if m.variant == 1 && m.mode == "NORMAL" {
		right += hint("?", "keys") + "  "
	}
	if m.variant != 0 && len(m.jumps) == 0 && len(queue) == 0 && m.msg == "" && m.mode == "NORMAL" {
		right = hint("F1", "help") + "  " + right
	}
	var left string
	switch m.mode {
	case "COMMAND":
		left = ":" + m.buf + "█"
	case "LIBSEARCH":
		left = "search library: " + m.buf + "█"
	case "FIND":
		left = "/" + m.buf + "█"
	default:
		if m.mode == "VISUAL" {
			left = "-- VISUAL -- " + plural(len(selTracks(m.selection()))) + " selected  "
		}
		left += m.msg
	}
	rw := lipgloss.Width(right) + lipgloss.Width(tag)
	return fit(left, w-rw) + right + tag
}

func (m *model) bodyH() int { return max(3, m.h-5) }

// overlay draws block over lines at column x, row y.
func overlay(lines []string, block string, x, y int) {
	for i, bl := range strings.Split(block, "\n") {
		if y+i < 0 || y+i >= len(lines) {
			continue
		}
		l := lines[y+i]
		bw := lipgloss.Width(bl)
		lines[y+i] = ansi.Truncate(l, x, "") + bl + ansi.TruncateLeft(l, x+bw, "")
	}
}

func (m *model) emptyQueue(w, h int) []string {
	var ls []string
	switch m.variant {
	case 0:
		ls = []string{
			"",
			" " + accent.Render("The queue is empty."),
			"",
			dim.Render(" Find something to play:"),
			"   " + hint("'s", "search the library"),
			"   " + hint("'a", "artists") + "   " + hint("'b", "albums"),
			"   " + hint("↑↓", "or j/k to move") + "  " + hint("<CR>", "to open"),
			"",
			dim.Render(" On a track or album:"),
			"   " + hint("<CR>", "play it") + "   " + hint("A", "add to queue"),
			"",
			"   " + hint("F1", "every key") + "   " + hint(":q", "quit"),
		}
	case 1:
		ls = []string{"", dim.Render(" queue is empty · ") + hint("?", "keys")}
	case 2:
		ls = []string{"", dim.Render(" queue is empty")}
	}
	out := make([]string, h)
	for i := range out {
		if i < len(ls) {
			out[i] = fit(ls[i], w)
		} else {
			out[i] = strings.Repeat(" ", w)
		}
	}
	return out
}

var steps = []struct{ id, text, keys string }{
	{"search", "Search for a track", "'s  or  :search"},
	{"album", "Jump to its album", "ga"},
	{"select", "Select some tracks", "V then j/k"},
	{"add", "Add them to the queue", "A  (or a: play next)"},
	{"play", "Play", "<Space>  or <CR>"},
	{"back", "Go back to where you were", "<C-o>"},
}

func (m *model) checklist(w int) []string {
	n := 0
	for _, s := range steps {
		if m.done[s.id] {
			n++
		}
	}
	if n == len(steps) || hintsOff {
		return nil
	}
	out := []string{fit(accent.Render(fmt.Sprintf(" Getting started %d/%d", n, len(steps)))+dim.Render("   : finds any command"), w)}
	for _, s := range steps {
		mark := dim.Render(" ○ ")
		txt := s.text + "  " + keySt.Render(s.keys)
		if m.done[s.id] {
			mark, txt = okSt.Render(" ✓ "), dim.Render(s.text)
		}
		out = append(out, fit(mark+txt, w))
	}
	return out
}

func (m *model) whichKey() string {
	mode := "n"
	if m.mode == "VISUAL" {
		mode = "v"
	}
	fs := followers(m.pend, mode, m.ctx())
	var ls []string
	for _, f := range fs {
		ls = append(ls, keySt.Render(fmt.Sprintf("%-3s", f.key))+" "+f.desc)
	}
	ls = append(ls, dim.Render("<Esc> cancel"))
	return box.Render(dim.Render(vim(m.pend)+" …") + "\n" + strings.Join(ls, "\n"))
}

func (m *model) keysPopup() string {
	c := m.ctx()
	mode := "n"
	if m.mode == "VISUAL" {
		mode = "v"
	}
	groups := []string{"Play", "Queue", "Select", "Go", "Help"}
	what := "here"
	if c.row != nil {
		what = c.row.kind + " " + c.row.title
	}
	out := []string{accent.Render("Keys for " + what)}
	for _, g := range groups {
		var ls []string
		for _, cm := range cmds {
			if cm.group != g || !strings.Contains(cm.modes, mode) || !cm.when(c) || cm.name == "keys" {
				continue
			}
			if g == "Go" && strings.HasPrefix(cm.seqs[0], "' ") && cm.name != "search" {
				continue
			}
			var ks []string
			for _, s := range cm.seqs {
				ks = append(ks, vim(s))
			}
			ls = append(ls, keySt.Render(fmt.Sprintf("%-14s", strings.Join(ks, " ")))+cm.desc)
		}
		if len(ls) > 0 {
			out = append(out, "", dim.Render(g))
			out = append(out, ls...)
		}
	}
	out = append(out, "", dim.Render("Move: j/k ↑↓  gg G  <C-d> <C-u>   Sections: 'h 'a 'b 'p 's 'q"), dim.Render("Press any key to run it · <Esc> closes · <F1> all keys"))
	return box.Render(strings.Join(out, "\n"))
}

func (m *model) palette(w int) string {
	items := m.palItems()
	var ls []string
	ls = append(ls, ": "+m.pal+"█")
	start := max(0, m.palCur-11)
	for i := start; i < len(items) && i < start+12; i++ {
		it := items[i]
		line := fit(it.desc, w-16) + " " + fmt.Sprintf("%12s", it.key)
		switch {
		case i == m.palCur:
			line = sel.Render(line)
		case !it.here:
			line = dim.Render(line)
		default:
			line = fit(it.desc, w-16) + " " + keySt.Render(fmt.Sprintf("%12s", it.key))
		}
		ls = append(ls, line)
	}
	ls = append(ls, dim.Render("type to filter · ↑↓ choose · <CR> runs · <Esc> closes · dim = not here"))
	return box.Render(strings.Join(ls, "\n"))
}

func (m *model) wildmenu(w int) string {
	cs := completions(m.buf, m.ctx())
	if len(cs) == 0 {
		return ""
	}
	var ls []string
	for i, c := range cs {
		if i == 8 {
			ls = append(ls, dim.Render(fmt.Sprintf("… %d more", len(cs)-8)))
			break
		}
		key := ""
		if c.seqs != nil {
			key = vim(c.seqs[0])
		}
		ls = append(ls, fit(c.name, 14)+fit(c.desc, w-30)+keySt.Render(fmt.Sprintf("%8s", key)))
	}
	ls = append(ls, dim.Render("<Tab> completes"))
	return box.Render(strings.Join(ls, "\n"))
}

func (m model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("")
	}
	W := m.w
	hint := m.variant == 0 && !hintsOff
	h := m.h - 4 // title row + now-playing (2) + status
	if hint {
		h--
	}
	var body []string
	ts := func(s string, w int, on bool) string {
		if on {
			return tabOn.Render(fit(s, w))
		}
		return bar.Render(fit(s, w))
	}
	queueBody := func(w int) []string {
		if len(queue) == 0 {
			return m.emptyQueue(w, h)
		}
		return m.list(&m.qp, w, h, m.foc == 1)
	}
	lt := " " + crumbs(m.lib.stack)
	rt := fmt.Sprintf(" Queue (%d)", len(queue))
	if W < 100 { // one window at a time, as decided in #8
		if m.foc == 0 {
			body = append(body, ts(lt+"   <Tab> queue", W, true))
			body = append(body, m.list(&m.lib, W, h, true)...)
		} else {
			body = append(body, ts(rt+"   <Tab> library", W, true))
			body = append(body, queueBody(W)...)
		}
	} else {
		lw := W * 3 / 5
		rw := W - lw - 1
		body = append(body, ts(lt, lw, m.foc == 0)+" "+ts(rt, rw, m.foc == 1))
		left := m.list(&m.lib, lw, h, m.foc == 0)
		right := queueBody(rw)
		if m.variant == 2 {
			if cl := m.checklist(rw); cl != nil {
				copy(right[max(0, h-len(cl)):], cl)
			}
		}
		for i := 0; i < h; i++ {
			body = append(body, left[i]+dim.Render("│")+right[i])
		}
	}
	if W < 100 && m.variant == 2 {
		if cl := m.checklist(W); cl != nil && m.foc == 1 {
			copy(body[max(1, len(body)-len(cl)):], cl)
		}
	}
	lines := body
	if hint {
		lines = append(lines, bar.Render(m.hintLine(W)))
	}
	lines = append(lines, m.nowPlaying(W)...)
	lines = append(lines, m.status(W))

	switch {
	case m.popup == "keys":
		b := m.keysPopup()
		overlay(lines, b, max(0, (W-lipgloss.Width(b))/2), 1)
	case m.popup == "palette":
		pw := min(W-4, 76)
		b := m.palette(pw)
		overlay(lines, b, max(0, (W-lipgloss.Width(b))/2), 1)
	case m.pend != "" && m.showWhich:
		b := m.whichKey()
		overlay(lines, b, max(0, W-lipgloss.Width(b)-1), len(lines)-4-lipgloss.Height(b))
	case m.mode == "COMMAND":
		if b := m.wildmenu(min(W-4, 70)); b != "" {
			overlay(lines, b, 0, len(lines)-1-lipgloss.Height(b))
		}
	}
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func main() {
	buildLibrary()
	m := newModel()
	for i, a := range os.Args[1:] {
		if a == "-variant" && i+2 < len(os.Args) {
			m.variant = max(0, min(2, int(os.Args[i+2][0]-'a')))
		}
	}
	helpN.dyn = func() []*node {
		var out []*node
		for _, g := range []string{"Play", "Queue", "Select", "Go", "Move", "Help"} {
			out = append(out, &node{title: "── " + g + " ──", kind: "text"})
			for _, cm := range cmds {
				if cm.group != g {
					continue
				}
				var ks []string
				for _, s := range cm.seqs {
					ks = append(ks, vim(s))
				}
				out = append(out, &node{title: fmt.Sprintf("%-18s %s", strings.Join(ks, " "), cm.desc), kind: "text"})
			}
		}
		return out
	}
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
