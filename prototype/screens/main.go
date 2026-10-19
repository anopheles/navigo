// PROTOTYPE: throwaway code for "What screens, layout and navigation model
// does navigo have?" (anopheles/navigo#8). Never merge into main.
//
// Three radically different layouts over the same fake library, switchable
// with F2 (or :variant a|b|c):
//
//	A  Tabs + drill-down stack: one full-width list per tab, h/l walk a stack.
//	B  Miller columns: parent | current | preview, h/l move along one path.
//	C  Vim windows: library window left, queue window always visible right.
//
// Run: cd prototype/screens && go run .
package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// ---------- fake library ----------

type track struct {
	title, artist, album string
	no, secs             int
}

type node struct {
	title  string
	scope  string // keymap scope of this node's *children* list
	kids   []*node
	dyn    func() []*node
	track  *track
	artist *node
	album  *node
}

func (n *node) children() []*node {
	if n.dyn != nil {
		return n.dyn()
	}
	return n.kids
}

var (
	root, homeN, queueN, artistsN, albumsN, playlistsN, searchN, helpN *node
	allTracks                                                          []*track
	albumNode                                                          = map[string]*node{}
)

var words = strings.Fields("amber hollow static lantern tidal velvet copper meridian ghost harbor signal winter paper orbit cinder glass marrow saltwater neon violet quiet thunder rust ember echo ivory feral lunar")

func w(i int) string { s := words[i%len(words)]; return strings.ToUpper(s[:1]) + s[1:] }

func buildLibrary() {
	artistNames := []string{"Kestrel Lane", "The Paper Orbits", "Mira Okonkwo", "Saltwater Choir", "Juno Vale", "Ninefold", "Ash & Ivory", "Low Meridian", "Cinder Theory", "Hal Brenner Trio"}
	artistsN = &node{title: "Artists", scope: "artists"}
	albumsN = &node{title: "Albums", scope: "albums"}
	seed := 3
	for ai, an := range artistNames {
		a := &node{title: an, scope: "albums"}
		for k := 0; k < 2+ai%3; k++ {
			seed += 7
			for albumNode[w(seed)+" "+w(seed*3+1+seed/len(words))] != nil {
				seed++
			}
			alb := &node{title: w(seed) + " " + w(seed*3+1+seed/len(words)), scope: "album-tracks", artist: a}
			alb.album = alb
			for t := 1; t <= 6+(seed%7); t++ {
				tr := &track{title: w(seed+t*5) + " " + w(seed*t+2), artist: an, album: alb.title, no: t, secs: 140 + (seed*t*13)%200}
				allTracks = append(allTracks, tr)
				alb.kids = append(alb.kids, &node{title: tr.title, track: tr, artist: a, album: alb})
			}
			albumNode[alb.title] = alb
			a.kids = append(a.kids, alb)
			albumsN.kids = append(albumsN.kids, alb)
		}
		artistsN.kids = append(artistsN.kids, a)
	}
	playlistsN = &node{title: "Playlists", scope: "playlists"}
	for pi, pn := range []string{"Late night", "Running", "Discover weekly-ish", "Starred"} {
		p := &node{title: pn, scope: "playlist-tracks"}
		for i := 0; i < 12; i++ {
			tr := allTracks[(pi*37+i*11)%len(allTracks)]
			alb := albumNode[tr.album]
			p.kids = append(p.kids, &node{title: tr.title, track: tr, artist: alb.artist, album: alb})
		}
		playlistsN.kids = append(playlistsN.kids, p)
	}
	homeN = &node{title: "Home", scope: "albums"}
	for hi, hn := range []string{"Recently added", "Most played", "Random"} {
		h := &node{title: hn, scope: "albums"}
		for i := 0; i < 6; i++ {
			h.kids = append(h.kids, albumsN.kids[(hi*5+i*3)%len(albumsN.kids)])
		}
		homeN.kids = append(homeN.kids, h)
	}
	queueN = &node{title: "Queue", scope: "queue"}
	queueN.dyn = func() []*node {
		out := make([]*node, len(queue))
		for i, tr := range queue {
			alb := albumNode[tr.album]
			out[i] = &node{title: tr.title, track: tr, artist: alb.artist, album: alb}
		}
		return out
	}
	searchN = &node{title: "Search", scope: "search-results", kids: []*node{{title: "type  :search <text>  to fill this list"}}}
	helpN = &node{title: "Help", scope: "help"}
	for _, l := range []string{
		"j/k  <C-d>/<C-u>  gg/G      move",
		"l / <CR>  go in      h  come out",
		"<CR> on a track     play this list from here",
		"a                   append track to queue",
		"dd (queue)          remove from queue",
		"ga / gr             go to track's album / artist",
		"gp                  now playing in the queue",
		"<Space>             play/pause    > <  next/prev",
		"/text  n N          jump to match",
		":queue :artists :albums :playlists :home :search x :help",
		"A: <Tab>/<S-Tab> gt/gT switch tabs",
		"C: <C-w>w / <C-w>h / <C-w>l switch window",
		"F2 or :variant a|b|c   switch prototype layout",
		":q                  quit",
	} {
		helpN.kids = append(helpN.kids, &node{title: l})
	}
	root = &node{title: "navigo", scope: "sections", kids: []*node{homeN, queueN, artistsN, albumsN, playlistsN, searchN, helpN}}
}

var queue []*track

// ---------- panes (a stack of frames over the tree) ----------

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
func (p *pane) in() bool {
	r := p.row()
	if r == nil || r.track != nil || len(r.children()) == 0 && r.dyn == nil {
		return false
	}
	p.stack = append(p.stack, frame{n: r})
	return true
}
func (p *pane) out(floor int) {
	if len(p.stack) > floor {
		p.stack = p.stack[:len(p.stack)-1]
	}
}

func idx(list []*node, n *node) int {
	for i, x := range list {
		if x == n {
			return i
		}
	}
	return 0
}

// pathTo builds root → Artists → artist → album [→ cursor on track].
func pathTo(r *node, toArtist bool) []frame {
	a, alb := r.artist, r.album
	fs := []frame{{n: root, cur: idx(root.kids, artistsN)}, {n: artistsN, cur: idx(artistsN.kids, a)}}
	if toArtist {
		fs = append(fs, frame{n: a, cur: idx(a.kids, alb)})
		return fs
	}
	cur := 0
	for i, k := range alb.kids {
		if k.track == r.track {
			cur = i
		}
	}
	return append(fs, frame{n: a, cur: idx(a.kids, alb)}, frame{n: alb, cur: cur})
}

// ---------- model ----------

type tickMsg time.Time

type model struct {
	w, h    int
	variant int

	tabs []pane // A
	tab  int
	b    pane // B
	cl   pane // C left
	cq   pane // C right
	cfoc int  // C focus 0/1

	playing int
	paused  bool
	elapsed int

	mode, buf, pend, last, msg string
}

func newModel() model {
	m := model{playing: -1, mode: "NORMAL"}
	for _, s := range root.kids {
		m.tabs = append(m.tabs, pane{stack: []frame{{n: s}}})
	}
	m.tab = 2
	m.b = pane{stack: []frame{{n: root, cur: 2}, {n: artistsN}}}
	m.cl = pane{stack: []frame{{n: root, cur: 2}, {n: artistsN}}}
	m.cq = pane{stack: []frame{{n: queueN}}}
	queue = append(queue, albumsN.kids[0].kids[0].track, albumsN.kids[0].kids[1].track, albumsN.kids[0].kids[2].track, albumsN.kids[4].kids[0].track)
	m.playing = 0
	return m
}

func (m *model) focused() *pane {
	switch m.variant {
	case 0:
		return &m.tabs[m.tab]
	case 1:
		return &m.b
	}
	if m.cfoc == 1 {
		return &m.cq
	}
	return &m.cl
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
				m.next(1)
			}
		}
		return m, tick()
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

func (m *model) next(d int) {
	if len(queue) == 0 {
		return
	}
	m.playing = max(0, min(len(queue)-1, m.playing+d))
	m.elapsed = 0
}

func (m model) key(k tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := k.String()
	if s == "ctrl+c" {
		return m, tea.Quit
	}
	if m.mode == "COMMAND" || m.mode == "SEARCH" {
		switch s {
		case "esc":
			m.mode = "NORMAL"
		case "enter":
			line, mode := m.buf, m.mode
			m.mode = "NORMAL"
			if mode == "COMMAND" {
				return m.run(line)
			}
			m.last = line
			m.find(1, false)
		case "backspace":
			if m.buf == "" {
				m.mode = "NORMAL"
			} else {
				m.buf = m.buf[:len(m.buf)-1]
			}
		default:
			m.buf += k.Text
		}
		return m, nil
	}

	p := m.focused()
	m.msg = ""
	if m.pend != "" {
		pend := m.pend
		m.pend = ""
		switch pend + s {
		case "gg":
			p.top().cur = 0
		case "gt":
			m.tab = (m.tab + 1) % len(m.tabs)
		case "gT":
			m.tab = (m.tab + len(m.tabs) - 1) % len(m.tabs)
		case "ga", "gr":
			if r := p.row(); r != nil && r.track != nil {
				m.jumpTo(pathTo(r, s == "r"))
			} else {
				m.msg = "not on a track"
			}
		case "gp":
			m.gotoPlaying()
		case "dd":
			if p.top().n == queueN && len(queue) > 0 {
				i := p.top().cur
				queue = append(queue[:i], queue[i+1:]...)
				if i < m.playing {
					m.playing--
				}
				p.move(0)
			}
		case "ctrl+ww":
			m.cfoc = 1 - m.cfoc
		case "ctrl+wh":
			m.cfoc = 0
		case "ctrl+wl":
			m.cfoc = 1
		default:
			m.msg = "no binding: " + pend + " " + s
		}
		return m, nil
	}

	switch s {
	case "f2":
		m.variant = (m.variant + 1) % 3
	case ":":
		m.mode, m.buf = "COMMAND", ""
	case "/":
		m.mode, m.buf = "SEARCH", ""
	case "n":
		m.find(1, true)
	case "N":
		m.find(-1, true)
	case "g", "d", "ctrl+w":
		m.pend = s
	case "j", "down":
		p.move(1)
	case "k", "up":
		p.move(-1)
	case "ctrl+d":
		p.move(m.bodyH() / 2)
	case "ctrl+u":
		p.move(-m.bodyH() / 2)
	case "G":
		p.move(1 << 20)
	case "l", "right":
		p.in()
	case "h", "left", "backspace":
		p.out(1) // every pane keeps its bottom frame
	case "enter":
		r := p.row()
		if r == nil {
			break
		}
		if r.track == nil {
			p.in()
			break
		}
		if p.top().n == queueN {
			m.playing, m.elapsed = p.top().cur, 0
			break
		}
		queue = queue[:0]
		for _, x := range p.rows() {
			if x.track != nil {
				queue = append(queue, x.track)
			}
		}
		m.playing, m.elapsed, m.paused = p.top().cur, 0, false
		m.msg = fmt.Sprintf("queue replaced: %d tracks", len(queue))
	case "a":
		if r := p.row(); r != nil && r.track != nil {
			queue = append(queue, r.track)
			m.msg = "appended to queue"
		}
	case "space":
		m.paused = !m.paused
	case ">":
		m.next(1)
	case "<":
		m.next(-1)
	case "tab":
		if m.variant == 0 {
			m.tab = (m.tab + 1) % len(m.tabs)
		} else if m.variant == 2 {
			m.cfoc = 1 - m.cfoc
		}
	case "shift+tab":
		if m.variant == 0 {
			m.tab = (m.tab + len(m.tabs) - 1) % len(m.tabs)
		}
	}
	return m, nil
}

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
			p.top().cur = j
			return
		}
	}
	m.msg = "pattern not found: " + m.last
}

// jumpTo shows a root-based path in whatever the active layout uses.
func (m *model) jumpTo(fs []frame) {
	switch m.variant {
	case 0:
		m.tab = idx(root.kids, fs[1].n)
		m.tabs[m.tab].stack = append([]frame(nil), fs[1:]...)
	case 1:
		m.b.stack = fs
	case 2:
		m.cl.stack, m.cfoc = fs, 0
	}
}

func (m *model) gotoPlaying() {
	if m.variant == 2 {
		m.cfoc, m.cq.stack[0].cur = 1, max(m.playing, 0)
		return
	}
	m.jumpTo([]frame{{n: root, cur: 1}, {n: queueN, cur: max(m.playing, 0)}})
}

func (m model) run(line string) (tea.Model, tea.Cmd) {
	f := strings.Fields(line)
	if len(f) == 0 {
		return m, nil
	}
	sections := map[string]*node{"home": homeN, "queue": queueN, "artists": artistsN, "albums": albumsN, "playlists": playlistsN, "search": searchN, "help": helpN}
	switch {
	case f[0] == "q" || f[0] == "quit":
		return m, tea.Quit
	case f[0] == "variant" && len(f) == 2:
		m.variant = max(0, min(2, int(f[1][0]-'a')))
	case f[0] == "search" && len(f) > 1:
		q := strings.ToLower(strings.Join(f[1:], " "))
		searchN.kids = nil
		for _, a := range artistsN.kids {
			if strings.Contains(strings.ToLower(a.title), q) {
				searchN.kids = append(searchN.kids, a)
			}
		}
		for _, a := range albumsN.kids {
			if strings.Contains(strings.ToLower(a.title), q) {
				searchN.kids = append(searchN.kids, a)
			}
		}
		for _, t := range allTracks {
			if strings.Contains(strings.ToLower(t.title), q) {
				alb := albumNode[t.album]
				searchN.kids = append(searchN.kids, &node{title: t.title, track: t, artist: alb.artist, album: alb})
			}
		}
		m.jumpTo([]frame{{n: root, cur: 5}, {n: searchN}})
	case sections[f[0]] != nil:
		n := sections[f[0]]
		if m.variant == 2 && n == queueN {
			m.cfoc = 1
			break
		}
		m.jumpTo([]frame{{n: root, cur: idx(root.kids, n)}, {n: n}})
	default:
		m.msg = "unknown command: " + line
	}
	return m, nil
}

// ---------- rendering ----------

var (
	sel     = lipgloss.NewStyle().Reverse(true)
	selDim  = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	dim     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	accent  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	bar     = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	tabOn   = lipgloss.NewStyle().Reverse(true).Bold(true)
	protoSt = lipgloss.NewStyle().Background(lipgloss.Color("166")).Foreground(lipgloss.Color("0")).Bold(true)
)

func fit(s string, w int) string {
	r := []rune(s)
	if len(r) > w {
		if w <= 1 {
			return string(r[:max(w, 0)])
		}
		return string(r[:w-1]) + "…"
	}
	return s + strings.Repeat(" ", w-len(r))
}

func dur(s int) string { return fmt.Sprintf("%d:%02d", s/60, s%60) }

func (m model) rowText(n *node, parent *node, i, w int) string {
	if n.track == nil {
		c := len(n.children())
		if c == 0 {
			return fit(" "+n.title, w)
		}
		return fit(" "+n.title, w-6) + dim.Render(fmt.Sprintf("%5d ", c))
	}
	t := n.track
	mark := "  "
	if parent == queueN && i == m.playing {
		mark = accent.Render("▶ ")
	}
	if w < 40 {
		return mark + fit(t.title, w-2)
	}
	num := fmt.Sprintf("%2d ", t.no)
	if parent == queueN || parent.scope != "album-tracks" {
		num = fmt.Sprintf("%2d ", i+1)
	}
	rest := w - 2 - 3 - 6
	tw := rest * 5 / 9
	return mark + num + fit(t.title, tw) + dim.Render(fit(" "+t.artist, rest-tw)) + fmt.Sprintf("%6s", dur(t.secs))
}

func (m model) list(f *frame, w, h int, focused bool) []string {
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
	out := make([]string, 0, h)
	for i := f.off; i < len(rows) && len(out) < h; i++ {
		line := m.rowText(rows[i], f.n, i, w)
		if i == f.cur {
			if focused {
				line = sel.Render(fit(stripRow(rows[i], f.n, i, w, m), w))
			} else {
				line = selDim.Render(fit(stripRow(rows[i], f.n, i, w, m), w))
			}
		}
		out = append(out, line)
	}
	for len(out) < h {
		out = append(out, strings.Repeat(" ", w))
	}
	return out
}

// stripRow renders a row without inner styles so the selection colour is clean.
func stripRow(n, parent *node, i, w int, m model) string {
	if n.track == nil {
		c := len(n.children())
		if c == 0 {
			return " " + n.title
		}
		return fit(" "+n.title, w-6) + fmt.Sprintf("%5d ", c)
	}
	t := n.track
	mark := "  "
	if parent == queueN && i == m.playing {
		mark = "▶ "
	}
	if w < 40 {
		return mark + fit(t.title, w-2)
	}
	num := fmt.Sprintf("%2d ", t.no)
	if parent == queueN || parent.scope != "album-tracks" {
		num = fmt.Sprintf("%2d ", i+1)
	}
	rest := w - 2 - 3 - 6
	tw := rest * 5 / 9
	return mark + num + fit(t.title, tw) + fit(" "+t.artist, rest-tw) + fmt.Sprintf("%6s", dur(t.secs))
}

func scopeChain(s string) string {
	switch s {
	case "album-tracks", "playlist-tracks", "search-results":
		return s + " → tracks → global"
	case "artists", "albums", "playlists", "sections":
		return s + " → containers → global"
	case "queue":
		return "queue → global"
	}
	return s + " → global"
}

func (m model) nowPlaying(w int) []string {
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
	fill := pw * m.elapsed / max(t.secs, 1)
	l2 := " " + accent.Render(strings.Repeat("━", fill)) + dim.Render(strings.Repeat("─", pw-fill)) + times + " "
	return []string{bar.Render(l1), bar.Render(l2)}
}

func (m model) status(w int) string {
	names := []string{"A tabs+stack", "B miller columns", "C vim windows"}
	tag := protoSt.Render(fmt.Sprintf(" PROTOTYPE %s · F2 ", names[m.variant]))
	tw := lipgloss.Width(tag)
	var left string
	switch m.mode {
	case "COMMAND":
		left = ":" + m.buf + "█"
	case "SEARCH":
		left = "/" + m.buf + "█"
	default:
		p := m.focused()
		left = fmt.Sprintf("-- %s -- %s  scope: %s", m.mode, m.pend, scopeChain(p.top().n.scope))
		if m.msg != "" {
			left = m.msg
		}
	}
	return fit(left, w-tw) + tag
}

func (m model) bodyH() int { return max(3, m.h-5) }

func crumbs(st []frame, from int) string {
	var parts []string
	for _, f := range st[from:] {
		parts = append(parts, f.n.title)
	}
	return strings.Join(parts, " › ")
}

func (m model) View() tea.View {
	if m.w == 0 {
		return tea.NewView("")
	}
	var lines []string
	W := m.w
	switch m.variant {
	case 0: // tabs + stack
		var tabs []string
		for i, s := range root.kids {
			t := fmt.Sprintf(" %s ", s.title)
			if i == m.tab {
				t = tabOn.Render(t)
			}
			tabs = append(tabs, t)
		}
		lines = append(lines, strings.Join(tabs, dim.Render("│")))
		p := &m.tabs[m.tab]
		lines = append(lines, dim.Render(fit(" "+crumbs(p.stack, 0), W)))
		lines = append(lines, m.list(p.top(), W, m.h-6, true)...)
		lines = append(lines, m.nowPlaying(W)...)
	case 1: // miller columns
		lines = append(lines, m.nowPlaying(W)...)
		st := m.b.stack
		h := m.h - 4
		lines = append(lines, dim.Render(fit(" "+crumbs(st, 0), W)))
		cw := (W - 2) / 4
		var parent []string
		if len(st) >= 2 {
			parent = m.list(&st[len(st)-2], cw, h, false)
		} else {
			parent = blank(cw, h)
		}
		mid := m.list(&st[len(st)-1], cw*2-cw/2, h, true)
		pw := W - 2 - cw - (cw*2 - cw/2)
		var prev []string
		r := m.b.row()
		switch {
		case r == nil:
			prev = blank(pw, h)
		case r.track != nil:
			t := r.track
			prev = []string{accent.Render(fit(" "+t.title, pw)), fit(" "+t.artist, pw), fit(" "+t.album, pw), fit(fmt.Sprintf(" track %d · %s", t.no, dur(t.secs)), pw), "", dim.Render(fit(" <CR> play  a append", pw)), dim.Render(fit(" ga album  gr artist", pw))}
			for len(prev) < h {
				prev = append(prev, strings.Repeat(" ", pw))
			}
		default:
			pf := frame{n: r}
			prev = m.list(&pf, pw, h, false)
			for i := range prev {
				prev[i] = dim.Render(prev[i])
			}
		}
		for i := 0; i < h; i++ {
			lines = append(lines, parent[i]+dim.Render("│")+mid[i]+dim.Render("│")+prev[i])
		}
	case 2: // vim windows
		h := m.h - 4
		lw := W * 3 / 5
		rw := W - lw - 1
		lt := " " + crumbs(m.cl.stack, 1)
		rt := fmt.Sprintf(" Queue (%d)", len(queue))
		ts := func(s string, w int, on bool) string {
			if on {
				return tabOn.Render(fit(s, w))
			}
			return bar.Render(fit(s, w))
		}
		lines = append(lines, ts(lt, lw, m.cfoc == 0)+" "+ts(rt, rw, m.cfoc == 1))
		left := m.list(m.cl.top(), lw, h-1, m.cfoc == 0)
		right := m.list(m.cq.top(), rw, h-1, m.cfoc == 1)
		for i := 0; i < h-1; i++ {
			lines = append(lines, left[i]+dim.Render("│")+right[i])
		}
		lines = append(lines, m.nowPlaying(W)...)
	}
	lines = append(lines, m.status(W))
	v := tea.NewView(strings.Join(lines, "\n"))
	v.AltScreen = true
	return v
}

func blank(w, h int) []string {
	out := make([]string, h)
	for i := range out {
		out[i] = strings.Repeat(" ", w)
	}
	return out
}

func main() {
	buildLibrary()
	if _, err := tea.NewProgram(newModel()).Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
