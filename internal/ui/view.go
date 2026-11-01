package ui

import (
	"fmt"
	"image/color"
	"math"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// narrow is the width below which only one window is shown.
const narrow = 100

// Styles use the terminal's 16 colours and text attributes, so navigo
// follows the terminal's theme; colors.go lets the config recolour the
// four roles. Each colour means one thing: magenta playing, cyan a key,
// red an error, blue the selection. With colours off (NO_COLOR)
// the attributes and marks (▶, ★, ──) still tell everything apart.
var (
	cursorStyle = lipgloss.NewStyle().Reverse(true) // the cursor in the focused window
	cursorDim   = lipgloss.NewStyle().
			Underline(true)
	// the cursor in the other one, as a last resort
	cursorMark  = "▎" // marks it where the accent can't tint it
	visualStyle = lipgloss.NewStyle().Background(ansi.Blue).Underline(true)
	dimStyle    = lipgloss.NewStyle().Faint(true)
	keyStyle    = lipgloss.NewStyle().Foreground(ansi.Cyan).Bold(true)
	accentStyle = lipgloss.NewStyle().Foreground(ansi.Magenta).Bold(true) // playing
	errStyle    = lipgloss.NewStyle().Foreground(ansi.Red).Bold(true)
	titleOn     = lipgloss.NewStyle().Reverse(true).Bold(true)
	titleOff    = lipgloss.NewStyle().Bold(true)
	boxStyle    = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ansi.BrightBlack).
			Padding(1, 2)
	popupStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ansi.BrightBlack).
			Padding(0, 1)
	focusedInput = lipgloss.NewStyle().Foreground(ansi.Cyan)
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	v.MouseMode = tea.MouseModeCellMotion
	v.ReportFocus = m.deps.SearchOnFocus
	// A fixed title lets window managers find navigo's window where every
	// terminal window has the same app or class (macOS, Windows).
	v.WindowTitle = "navigo"
	return v
}

// render draws the whole screen as styled text.
func (m *Model) render() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}
	if m.screen == screenLogin {
		return m.loginView()
	}
	h := m.bodyHeight()
	var lines []string
	m.paneAt = [2]paneHit{}
	m.colAt = m.colAt[:0]
	switch {
	case m.inColumns() && m.w < narrow && m.focus == libraryWindow:
		m.paneAt[libraryWindow] = paneHit{0, m.w}
		title := m.columnTitle() + "   " + m.keyHint("window", "queue")
		lines = m.pane(title, m.columnRows(1, m.w-2, h, true), m.w, true)
	case m.inColumns() && m.w < narrow:
		m.paneAt[queueWindow] = paneHit{0, m.w}
		title := m.queueTitle() + "   " + m.keyHint("window", "library")
		rows := append(m.queueCard(m.w-2), m.queueRows(m.w-2, m.listHeight(queueWindow), true)...)
		lines = m.pane(title, rows, m.w, true)
	case m.w < narrow:
		m.paneAt[m.focus] = paneHit{0, m.w}
		if m.focus == libraryWindow {
			title := m.libTitle() + "   " + m.keyHint("window", "queue")
			lines = m.pane(title, m.libraryRows(m.w-2, h, true), m.w, true)
		} else {
			title := m.queueTitle() + "   " + m.keyHint("window", "library")
			lines = m.pane(title, m.queueRows(m.w-2, h, true), m.w, true)
		}
	default:
		lw := m.w * 3 / 5
		rw := m.w - lw
		m.paneAt = [2]paneHit{{0, lw}, {lw, rw}}
		var rows []string
		if m.inColumns() {
			rows = m.columnRows(1, lw-2, h, m.focus == libraryWindow)
		} else {
			rows = m.libraryRows(lw-2, h, m.focus == libraryWindow)
		}
		left := m.pane(
			m.libTitle(),
			rows,
			lw,
			m.focus == libraryWindow,
		)
		right := m.pane(
			m.queueTitle(),
			append(
				m.queueCard(rw-2),
				m.queueRows(rw-2, m.listHeight(queueWindow), m.focus == queueWindow)...,
			),
			rw,
			m.focus == queueWindow,
		)
		for i := range left {
			lines = append(lines, left[i]+right[i])
		}
	}
	if m.hints {
		lines = append(lines, fit(m.hintLine(), m.w))
	}
	m.bar = barHit{}
	m.npRows[0] = len(lines)
	lines = append(lines, m.progressRule(), m.nowPlaying())
	m.npRows[1] = len(lines) - m.npRows[0]
	lines = append(lines, m.status())
	if pm := m.prefixMenu(); len(pm) > 0 {
		at := len(lines) - 1 - len(pm)
		copy(lines[max(at, 0):], pm)
	}
	if ms := m.matchList(); len(ms) > 0 {
		at := len(lines) - 1 - len(ms) // just above the status line
		copy(lines[max(at, 0):], ms)
	}
	if m.popup != nil {
		box := m.renderPopup()
		overlay(lines, box, max((m.w-lipgloss.Width(box))/2, 0), 2)
	}
	return strings.Join(lines, "\n")
}

// pane frames rows (w-2 wide) in a rounded border with the title in the top
// edge. The focused window's border is plain and its title reversed; the
// other's is faint. With the accent on, the focused border is the accent's
// middle colour and its title fades across.
func (m *Model) pane(title string, rows []string, w int, focused bool) []string {
	name := func(s string) string { return titleOff.Render(s) }
	edge := dimStyle
	if focused {
		edge, name = lipgloss.NewStyle(), func(s string) string { return titleOn.Render(s) }
		if m.accentOn() {
			edge = lipgloss.NewStyle().Foreground(accentStops[1])
			name = m.accent
		}
	}
	inner := max(w-2, 0)
	t := ansi.Truncate(title, max(inner-1, 0), "…") + " "
	t = ansi.Truncate(t, inner, "")
	out := []string{
		edge.Render("╭") + name(t) +
			edge.Render(strings.Repeat("─", max(inner-lipgloss.Width(t), 0))+"╮"),
	}
	for _, r := range rows {
		out = append(out, edge.Render("│")+r+edge.Render("│"))
	}
	return append(out, edge.Render("╰"+strings.Repeat("─", inner)+"╯"))
}

func (m *Model) libTitle() string {
	var parts []string
	for i := range m.lib[1:] {
		f := &m.lib[i+1]
		t := f.title
		if f.kind == homeTracks {
			t = fmt.Sprintf("%s (%d)", f.title, len(m.homeTracksBase(f)))
		}
		if f.kind == playlistTracks && m.state.Library != nil {
			if p, ok := m.state.Library.Playlist(f.playlist); ok {
				t = p.Name // it may have been renamed since
			}
		}
		if d := f.view.describe(); d != "" {
			t += " (" + d + ")"
		}
		parts = append(parts, t)
	}
	if len(parts) == 0 {
		return " " + m.lib[0].title
	}
	return " " + strings.Join(parts, " › ")
}

// queueTitle is " Queue (12) · 48:10": the count, then the total length.
func (m *Model) queueTitle() string {
	t := fmt.Sprintf(" Queue (%d)", len(m.state.Queue))
	if len(m.state.Queue) == 0 {
		return t
	}
	if m.queueView.filtered() {
		t = fmt.Sprintf(
			" Queue (%d of %d) · %s",
			m.queueLen(),
			len(m.state.Queue),
			m.queueView.describe(),
		)
	}
	var total time.Duration
	for _, e := range m.state.Queue {
		total += e.Track.Duration
	}
	return t + " · " + long(total)
}

// keyHint is "<key> label" for a command, or "" if nothing is bound to it.
func (m *Model) keyHint(name, label string) string {
	ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), name)
	if len(ks) == 0 {
		return ""
	}
	return ks[0] + " " + label
}

// libraryRows draws the visible rows of the library window's list. Only
// the rows on screen are rendered, so long lists cost nothing extra.
func (m *Model) libraryRows(w, h int, focused bool) []string {
	f := m.top()
	n := m.rows(f)
	f.offset = scroll(f.cur, f.offset, h, n)
	m.markQueued()
	var out []string
	switch {
	case n > 0:
	case m.state.Library != nil && f.view.filtered():
		out = append(out, dimStyle.Render(fit(m.emptyRows(f), w)))
	case f.kind == homeAlbums:
		out = append(out, dimStyle.Render(fit(m.emptyHome(f.homeKind), w)))
	case f.kind == homeTracks:
		out = append(out, dimStyle.Render(fit(m.emptyHomeTracks(f), w)))
	case f.kind == helpList && f.help != nil:
		out = append(out, dimStyle.Render(fit(m.emptyHelp(f.help), w)))
	case f.kind == searchList && f.search != nil:
		for _, l := range m.emptySearch(f.search) {
			out = append(out, dimStyle.Render(fit(l, w)))
		}
	default:
		out = append(out, dimStyle.Render(fit(m.emptyRows(f), w)))
	}
	for i := f.offset; i < n && len(out) < h; i++ {
		m.dress = i != f.cur && !m.inSelection(libraryWindow, i) && (!focused || !m.highlighting())
		m.marks.on = true
		row := m.libraryRow(f, i, w)
		m.marks.on = false
		m.dress = false
		if focused {
			row = m.highlight(row)
		}
		out = append(
			out,
			m.styleRow(row, i == f.cur, focused, false, m.inSelection(libraryWindow, i)),
		)
	}
	return pad(out, w, h)
}

// emptyRows says why a list has no rows and what to do about it.
func (m *Model) emptyRows(f *frame) string {
	if m.state.Library == nil {
		return m.emptyList()
	}
	switch {
	case f.view.filtered():
		return " no row matches the filter · :filter alone shows all"
	case f.kind == playlistList && m.state.Offline:
		return " no playlists in the cache · they can be made once the server is back"
	case f.kind == playlistList:
		return " no playlists yet · in a track list, :playlist-new <name> makes one"
	case f.kind == playlistTracks && m.state.Offline:
		return " empty playlist · tracks can be added once the server is back"
	case f.kind == playlistTracks:
		return " empty playlist · :playlist-add <name> adds tracks"
	case f.kind == albumTracks:
		return " this album has no tracks"
	case f.kind == artistList:
		return " no artists in the library · add music to the server, then :sync"
	case f.kind == albumList:
		return " no albums in the library · add music to the server, then :sync"
	}
	return " nothing here"
}

// emptyList says why a list has no rows while there is no library.
func (m *Model) emptyList() string {
	switch sy := m.state.Sync; {
	case m.state.Library != nil:
		return " nothing here"
	case m.state.Offline:
		return " can't reach " + m.deps.Config.Server.URL + " · retrying · :login to change it"
	case sy.Running && sy.Total > 0:
		return fmt.Sprintf(" Syncing library… %d / %d tracks", sy.Done, sy.Total)
	case sy.Running:
		return " Syncing library…"
	case sy.Failed:
		return " the library couldn't sync · :sync tries again"
	}
	return " loading…"
}

func (m *Model) libraryRow(f *frame, i, w int) string {
	switch f.kind {
	case sectionList:
		return fit(" "+sections[i].title, w)
	case artistList:
		return m.artistLine(m.artistsOf(f)[i], w)
	case artistPage:
		rows := m.artistRows(f)
		if a, ok := rows.album(i); ok {
			return m.albumLine(a, w)
		}
		return dimStyle.Render(fit(" ── Appears on ──", w))
	case albumList:
		return m.albumLine(m.albumsOf(f)[i], w)
	case homeList:
		return fit(" "+m.homeRows()[i].title+m.mixRowNote(i), w)
	case homeAlbums:
		return m.albumLine(m.albumsOf(f)[i], w)
	case playlistList:
		p := m.playlistsOf(f)[i]
		n := count(len(p.Tracks))
		return " " + fit(p.Name, w-2-len(n)-2) + "  " + n + " "
	case playlistTracks:
		ts := m.playlistTracksOf(f)
		return m.trackLine("  ", fmt.Sprint(i+1), ts[i], w)
	case homeTracks:
		return m.trackLine("  ", fmt.Sprint(i+1), m.homeTracksOf(f)[i], w)
	case searchList:
		return m.searchRowText(f.search, i, w)
	case helpList:
		return m.helpRowText(f.help, i, w)
	}
	ts := m.tracksOf(f)
	return m.trackLine("  ", trackNumber(ts, i), ts[i], w)
}

// starred marks a starred name with ★.
func (m *Model) starred(id, name string) string {
	if m.state.Library != nil && m.state.Library.IsStarred(id) {
		return "★ " + name
	}
	return name
}

func (m *Model) artistLine(a library.Artist, w int) string {
	if m.dress {
		return m.artistLineDressed(a, w)
	}
	n := artistCount(a.AlbumCount, m.state.Library.AppearsOn(a.ID))
	return " " + fit(m.starred(string(a.ID), a.Name), w-2-len(n)-2) + "  " + n + " "
}

func (m *Model) albumLine(a library.Album, w int) string {
	if m.dress {
		return m.albumLineDressed(a, w)
	}
	year := ""
	if a.Year > 0 {
		year = fmt.Sprint(a.Year)
	}
	rest := w - 1 - 6
	nw := rest * 3 / 5
	return m.albumLead(a) + fit(
		m.starred(string(a.ID), a.Name),
		nw,
	) + fit(
		"  "+a.Artist,
		rest-nw,
	) + fmt.Sprintf(
		"%6s",
		year,
	)
}

// trackNumber is "3" or, on albums with several discs, "2-03".
func trackNumber(ts []library.Track, i int) string {
	if ts[i].Number == 0 { // untagged
		return ""
	}
	if ts[len(ts)-1].Disc > 1 {
		return fmt.Sprintf("%d-%02d", ts[i].Disc, ts[i].Number)
	}
	return fmt.Sprint(ts[i].Number)
}

func (m *Model) trackLine(mark, num string, t library.Track, w int) string {
	mark = m.trackMark(mark, t)
	title := m.starred(string(t.ID), t.Title)
	if w < fullTrackRow {
		return fit(mark+title, w)
	}
	if m.dress {
		return m.trackLineDressed(mark, num, t, w)
	}
	numW := max(len(num)+1, 3)
	rest := w - lipgloss.Width(mark) - numW - 6
	tw := rest * 5 / 9
	tag := m.ratingTag(t, tw)
	return mark + fmt.Sprintf(
		"%*s ",
		numW-1,
		num,
	) + fit(
		title,
		tw-lipgloss.Width(tag),
	) + tag + fit(
		"  "+t.Artist,
		rest-tw,
	) + fmt.Sprintf(
		"%6s",
		clock(t.Duration),
	)
}

func (m *Model) queueRows(w, h int, focused bool) []string {
	q := m.state.Queue
	if len(q) == 0 {
		return pad(m.emptyQueue(w), w, h)
	}
	n := m.queueLen()
	if n == 0 {
		return pad(
			[]string{dimStyle.Render(fit(" nothing matches · :filter alone shows all", w))},
			w,
			h,
		)
	}
	m.queueOff = scroll(m.queueCur, m.queueOff, h, n)
	var out []string
	_, row, ok := m.state.NowPlaying()
	for r := m.queueOff; r < n && len(out) < h; r++ {
		i := m.queueIndex(r)
		mark := "  "
		if q[i].Radio {
			mark = "~ " // the radio added it
		}
		current := ok && i == row
		// The playing row shows while filtered out, dimmed.
		unmatched := current && m.queueView.filtered() &&
			len(apply(m.queueView, []library.Track{q[i].Track}, trackFacts)) == 0
		if current {
			mark = "▶ "
			if m.state.Player.Paused {
				mark = "⏸ "
			}
		}
		quiet := r != m.queueCur && !current && !m.inSelection(queueWindow, r) &&
			(!focused || !m.highlighting())
		played := quiet && ok && i < row || unmatched
		m.dress = quiet && !played
		line := m.trackLine(mark, fmt.Sprint(i+1), q[i].Track, w)
		m.dress = false
		if played {
			line = dimStyle.Render(line)
		}
		if focused {
			line = m.highlight(line)
		}
		out = append(
			out,
			m.styleRow(line, r == m.queueCur, focused, current, m.inSelection(queueWindow, r)),
		)
	}
	return pad(out, w, h)
}

// emptyQueue tells a new user what to press first, with the keys bound now.
func (m *Model) emptyQueue(w int) []string {
	tracks := []string{"album-tracks", "tracks", "global"}
	containers := []string{"albums", "containers", "global"}
	k := func(scopes []string, name, label string) string {
		ks := m.engine.Table().Keys(keymap.Normal, scopes, name)
		if len(ks) == 0 {
			return ""
		}
		if label == "" {
			return keyStyle.Render(ks[0])
		}
		return keyStyle.Render(ks[0]) + " " + dimStyle.Render(label)
	}
	// pair joins "j" and "k move" into "j/k move".
	pair := func(a, b string) string {
		if a == "" || b == "" {
			return a + b
		}
		return a + dimStyle.Render("/") + b
	}
	join := func(parts ...string) string {
		var out []string
		for _, p := range parts {
			if p != "" {
				out = append(out, p)
			}
		}
		return "   " + strings.Join(out, "   ")
	}
	lines := []string{
		"",
		" " + accentStyle.Render("The queue is empty."),
		"",
		dimStyle.Render(" Find something to play:"),
		join(
			k(containers, "albums", "albums"),
			pair(k(containers, "down", ""), k(containers, "up", "move")),
		),
		join(k(containers, "open", "open"), k(containers, "back", "back")),
		"",
		dimStyle.Render(" On a track:"),
		join(k(tracks, "play-now", "play it"), k(tracks, "add-end", "add to queue")),
		"",
	}
	if mix := k([]string{"global"}, "mix", "mix from the whole library"); mix != "" {
		lines = append(lines, dimStyle.Render(" Or let navigo pick:"), join(mix), "")
	}
	lines = append(lines, join(keyStyle.Render(":q")+" "+dimStyle.Render("quit")))
	for i := range lines {
		lines[i] = fit(lines[i], w)
	}
	return lines
}

func (m *Model) styleRow(line string, cursor, focused, current, selected bool) string {
	switch {
	case cursor && focused && m.accentOn():
		return lipgloss.NewStyle().Background(m.tint()).Bold(true).Render(line)
	case cursor && focused:
		return cursorStyle.Render(line)
	case cursor && m.accentOn():
		return lipgloss.NewStyle().Background(m.restTint()).Render(line)
	case cursor && strings.HasPrefix(line, " "):
		// A mark in the row's blank first column, in any palette.
		return cursorMark + line[1:]
	case cursor:
		return cursorDim.Render(line)
	case selected && selectionTinted && m.accentOn():
		return lipgloss.NewStyle().Background(m.selectionTint()).Render(line)
	case selected:
		return visualStyle.Render(line)
	case current:
		return m.accent(line)
	}
	return line
}

// hintLine lists the keys that matter for the row under the cursor and the
// mode, from the bindings in effect.
func (m *Model) hintLine() string {
	if m.mode == keymap.Cmdline {
		return " " + hint(
			"<CR>",
			"run",
		) + "  " + hint(
			"<Tab>",
			"complete",
		) + "  " + hint(
			"<Up>/<Down>",
			"history",
		) + "  " + hint(
			"<Esc>",
			"cancel",
		)
	}
	if m.finding.prompting {
		return " " + hint(
			"<CR>",
			"find",
		) + "  " + hint(
			"<Esc>",
			"cancel",
		) + dimStyle.Render(
			"  a capital letter matches case",
		)
	}
	if m.promptHelp || m.promptPick {
		return " " + hint("<CR>", "keep the filter") + "  " + hint("<Esc>", "put it back")
	}
	if m.naming != nil {
		return " " + hint("<CR>", "make it") + "  " + hint("<Esc>", "cancel")
	}
	if m.mode == keymap.Search {
		acts := ""
		for _, a := range []struct{ name, label string }{
			{"add-end", "add"}, {"add-next", "play next"}, {"play-this", "play now"},
		} {
			if ks := m.searchKeys(a.name); len(ks) > 0 {
				acts += hint(ks[0], a.label) + "  "
			}
		}
		down, up := m.searchKeys("down"), m.searchKeys("up")
		if len(down) > 0 && len(up) > 0 {
			acts += hint(down[0]+"/"+up[0], "move") + "  "
		}
		if ks := m.searchKeys("queue"); len(ks) > 0 {
			acts += hint(ks[0], "queue") + "  "
		}
		return " " + acts + hint("<CR>", "go to the results") + "  " + hint("<Esc>", "cancel")
	}
	if m.popup != nil && m.popup.yes != nil {
		return " " + hint("y", "yes") + "  " + hint("n", "no")
	}
	if m.popup != nil && m.popup.keyMenu != nil {
		return " " + dimStyle.Render(
			"press a key to run it",
		) + "  " + hint(
			"j/k",
			"scroll",
		) + "  " + hint(
			"<F1>",
			"help",
		) + "  " + hint(
			"q",
			"close",
		)
	}
	if m.popup != nil && m.popup.pick != nil && m.pick != nil {
		return " " + hint("j/k", "move") + "  " + hint("<CR>", "add") + "  " +
			hint("/", "filter") + "  " + hint("q", "close")
	}
	if m.popup != nil && m.popup.pick != nil {
		return " " + hint("j/k", "move") + "  " + hint("<CR>", "choose") + "  " + hint("q", "close")
	}
	if m.popup != nil {
		return " " + hint("j/k", "scroll") + "  " + hint("q", "close")
	}
	if m.engine.Pending() != "" {
		return dimStyle.Render(" waiting for the next key · ") + hint("<Esc>", "cancel")
	}
	if m.visual != nil {
		return m.visualHints()
	}
	var names []string
	switch {
	case m.focus == queueWindow && len(m.state.Queue) == 0:
		names = []string{"window", "albums", "key-menu"}
	case m.focus == queueWindow:
		names = []string{"play-row", "visual", "play-pause", "undo", "window", "key-menu"}
	case m.onTrack():
		names = []string{"play-now", "add-end", "add-next", "visual", "back", "window", "key-menu"}
	default:
		names = []string{"open", "back", "window", "key-menu"}
	}
	if _, ok := m.rowSeed(); ok { // gm offered only where it has a seed
		i := slices.Index(names, "window")
		names = slices.Insert(names, i, "mix")
	}
	var parts []string
	for _, name := range names {
		if name == "back" && len(m.lib) <= 1 {
			continue
		}
		c, _ := m.registry.Get(name)
		if ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), name); len(ks) > 0 && c != nil {
			label := c.Label
			if name == "open" && m.onMixRow() {
				label = "mix" // <CR> on Home's Mix row runs a mix, not opens a list
			}
			parts = append(parts, hint(ks[0], label))
		}
	}
	return " " + strings.Join(parts, "  ")
}

func hint(key, label string) string { return keyStyle.Render(key) + " " + dimStyle.Render(label) }

// indicators are the lasting states shown in the now-playing bar, most
// important first: mpv problems, then offline, then syncing, then ReplayGain.
func (m *Model) indicators() []string {
	var out []string
	switch s := m.state; {
	case s.PlayerProblem != "":
		out = append(out, s.PlayerProblem)
	case s.PlayerCapped:
		out = append(out, "mpv keeps crashing · :reconnect")
	case s.PlayerDown:
		if ks := m.engine.Table().
			Keys(keymap.Normal, []string{"global"}, "play-pause"); len(
			ks,
		) > 0 {
			out = append(out, "mpv stopped · "+keyLabel(ks[0])+" restarts it")
		} else {
			out = append(out, "mpv stopped")
		}
	case s.PlayerStuck:
		out = append(out, "mpv isn't responding · no audio output?")
	case s.NoAudio:
		out = append(out, "no audio output")
	}
	if m.state.Offline {
		next := "offline · trying…"
		if !m.state.Probing {
			secs := int(math.Ceil(m.retryAt.Sub(m.clock.Now()).Seconds()))
			next = fmt.Sprintf("offline · next try in %ds", max(secs, 1))
		}
		out = append(out, next)
	}
	if m.state.Sync.Running {
		out = append(out, "⟳ syncing")
	}
	if rg := m.gainIndicator(); rg != "" {
		out = append(out, rg)
	}
	return out
}

// fitIndicators joins items with " · " within width cells, dropping the
// least important (last) first.
func fitIndicators(items []string, width int) string {
	for n := len(items); n > 0; n-- {
		if s := strings.Join(items[:n], " · "); lipgloss.Width(s) <= width {
			return s
		}
	}
	return ""
}

// progressRule is the rule above the now-playing bar, filled as the
// track plays.
func (m *Model) progressRule() string {
	s := m.state
	if _, _, playing := s.NowPlaying(); !playing {
		return dimStyle.Render(strings.Repeat("─", max(m.w, 0)))
	}
	m.bar = barHit{y: m.npRows[0], x: 0, w: m.w}
	return progressBar(halvesDone(s.Player.Position, s.Player.Duration, m.w), m.w)
}

// playIcon is ▶ or ⏸ for the playing track, ■ with none.
func (m *Model) playIcon() string {
	switch _, _, playing := m.state.NowPlaying(); {
	case !playing:
		return "■"
	case m.state.Player.Paused:
		return "⏸"
	}
	return "▶"
}

// barSegment is a run of the now-playing bar on one background.
type barSegment struct {
	text   string
	fg, bg color.Color
	bold   bool
}

func (g barSegment) render() string {
	st := lipgloss.NewStyle().Bold(g.bold)
	if g.fg != nil {
		st = st.Foreground(g.fg)
	}
	if g.bg != nil {
		st = st.Background(g.bg)
	}
	return st.Render(g.text)
}

// Nerd Font arrows between segments, with the powerline setting.
const (
	arrowRight = "\ue0b0"
	arrowLeft  = "\ue0b2"
)

// joinSegments draws segs in a row. Between two, with the powerline
// setting, an arrow points away from the middle: in the left one's colour
// on the right one's background on the left side, mirrored on the right.
func (m *Model) joinSegments(segs []barSegment, left bool) string {
	var b strings.Builder
	for i, g := range segs {
		if !left && m.deps.Config.Powerline && g.bg != nil {
			var prev color.Color
			if i > 0 {
				prev = segs[i-1].bg
			}
			b.WriteString(barSegment{text: arrowLeft, fg: g.bg, bg: prev}.render())
		}
		b.WriteString(g.render())
		if left && m.deps.Config.Powerline && g.bg != nil {
			var next color.Color
			if i+1 < len(segs) {
				next = segs[i+1].bg
			}
			b.WriteString(barSegment{text: arrowRight, fg: g.bg, bg: next}.render())
		}
	}
	return b.String()
}

// mode is the now-playing bar's first barSegment: the mode's name in the role
// that goes with it, the selection's for Visual and the key's for typing.
func (m *Model) modeSegment() barSegment {
	name, bg := "NORMAL", playingColor
	switch {
	case m.mode == keymap.Cmdline:
		name, bg = "COMMAND", keyColor
	case m.mode == keymap.Search:
		name, bg = "SEARCH", keyColor
	case m.visual != nil:
		name, bg = "VISUAL", selectionColor
	}
	return barSegment{text: " " + name + " ", fg: ansi.Black, bg: bg, bold: true}
}

// nowPlaying is the bar under the rule: the mode and ▶/⏸, the track, then
// the indicators and modes, the position in the queue and the time.
func (m *Model) nowPlaying() string {
	s := m.state
	t, row, playing := s.NowPlaying()
	mode := m.modeSegment()
	state := " " + m.playIcon() + " "
	if s.Player.Buffering {
		state += "buffering "
	}
	left := m.joinSegments(
		[]barSegment{mode, {text: state, fg: mode.bg, bg: ansi.BrightBlack}},
		true,
	)

	pos := fmt.Sprintf(" %d in queue ", len(s.Queue))
	switch {
	case playing && row >= 0:
		pos = fmt.Sprintf(" %d/%d ", row+1, len(s.Queue))
	case playing:
		pos = " not in the queue "
	}
	segs := []barSegment{{text: pos, fg: mode.bg, bg: ansi.BrightBlack}}
	if playing {
		times := fmt.Sprintf(" %s / %s ", clock(s.Player.Position), clock(s.Player.Duration))
		segs = append(segs, barSegment{text: times, fg: ansi.Black, bg: mode.bg, bold: true})
	}
	right := m.joinSegments(segs, false)

	track := dimStyle.Render("nothing playing")
	if playing {
		track = m.accent(
			t.Title,
		) + dimStyle.Render(
			" — ",
		) + t.Artist + dimStyle.Render(
			" · "+t.Album,
		)
	}
	// Flags get what's left after a readable start of the track.
	room := m.w - lipgloss.Width(left) - lipgloss.Width(right) - 2
	flags := fitIndicators(
		append(m.indicators(), m.flags()...),
		room-min(lipgloss.Width(track), 30)-2,
	)
	if flags != "" {
		flags += " "
	}
	mid := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	return left + fit(" "+track, mid-lipgloss.Width(flags)) + flags + right
}

// flags are the now-playing bar's modes: repeat, the radio, and the
// volume when it isn't full.
func (m *Model) flags() []string {
	var out []string
	if r := m.state.Repeat; r != app.RepeatOff {
		out = append(out, "repeat "+r.String())
	}
	if m.state.Radio {
		out = append(out, "radio")
	}
	if v := m.state.Player.Volume; v != 100 {
		out = append(out, fmt.Sprintf("vol %d%%", v))
	}
	return out
}

// status is the bottom line: the ":" line, or a message, and pending keys.
func (m *Model) status() string {
	if s, ok := m.promptWithMsg(); ok {
		return s
	}
	if m.mode == keymap.Search && m.selected {
		return fit(m.line.Prompt+cursorStyle.Render(m.line.Value()), m.w)
	}
	if m.mode == keymap.Cmdline || m.mode == keymap.Search {
		return fit(m.line.View(), m.w)
	}
	right := m.engine.Pending()
	if right != "" {
		right += "  "
	}
	left := " " + m.shown.text
	if m.shown.err {
		left = " " + errStyle.Render(m.shown.text)
	}
	return fit(left, m.w-lipgloss.Width(right)) + right
}

// promptWithMsg is the library search prompt with a message from since it
// opened at the right end, cut short so the query stays readable. It does not
// apply when there is no message or no room for one.
func (m *Model) promptWithMsg() (string, bool) {
	if !m.libraryPrompt() || m.shown.text == "" || m.shown.id <= m.promptFloor {
		return "", false
	}
	const minRoom, gap = 12, 2
	room := m.w - lipgloss.Width(m.line.Prompt) - lipgloss.Width(m.line.Value()) - gap - 1
	if room < minRoom {
		return "", false
	}
	n := min(lipgloss.Width(m.shown.text), room)
	style := dimStyle
	if m.shown.err {
		style = errStyle
	}
	left := m.line.Prompt + cursorStyle.Render(m.line.Value())
	if !m.selected {
		l := m.line
		l.SetWidth(m.w - lipgloss.Width(m.line.Prompt) - n - gap - 1)
		left = l.View()
	}
	return fit(
		left,
		m.w-n-gap,
	) + strings.Repeat(
		" ",
		gap,
	) + style.Render(
		fit(m.shown.text, n),
	), true
}

func (m *Model) loginView() string {
	f := &m.login
	labels := []string{"Server URL", "Username", "Password"}
	var rows []string
	rows = append(rows, accentStyle.Render("Log in to your Navidrome or Subsonic server"), "")
	for i, l := range labels {
		label := fmt.Sprintf("%-11s", l)
		if i == f.focus {
			label = focusedInput.Render(label)
		} else {
			label = dimStyle.Render(label)
		}
		rows = append(rows, label+" "+f.fields[i].View())
	}
	rows = append(rows, "")
	switch {
	case f.askPlain:
		rows = append(
			rows,
			errStyle.Render(f.err),
			"Save the password in "+m.deps.ConfigPath+" instead? "+hint(
				"y",
				"yes",
			)+" "+hint(
				"n",
				"only for this session",
			),
		)
	case f.busy:
		rows = append(rows, dimStyle.Render("checking with the server…"))
	case f.err != "":
		rows = append(rows, errStyle.Render(f.err))
	default:
		rows = append(rows, "")
	}
	rows = append(
		rows,
		"",
		hint("<Tab>", "next field")+"  "+hint("<CR>", "log in")+"  "+hint("<Esc>", "quit"),
	)
	box := boxStyle.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
}

// overlay draws block over lines with its top-left corner at x, y.
func overlay(lines []string, block string, x, y int) {
	for i, bl := range strings.Split(block, "\n") {
		if y+i < 0 || y+i >= len(lines) {
			continue
		}
		l := lines[y+i]
		lines[y+i] = ansi.Truncate(l, x, "") + bl + ansi.TruncateLeft(l, x+lipgloss.Width(bl), "")
	}
}

// newInput returns a text input with a steady cursor: a blinking one
// would keep a timer running for nothing.
func newInput() textinput.Model {
	t := textinput.New()
	st := textinput.DefaultDarkStyles()
	st.Cursor.Blink = false
	t.SetStyles(st)
	return t
}

// scroll returns the list offset that keeps cur visible in h rows.
func scroll(cur, off, h, n int) int {
	if cur < off {
		off = cur
	}
	if cur >= off+h {
		off = cur - h + 1
	}
	return max(min(off, n-h), 0)
}

// fit pads or truncates s to exactly w cells.
func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func pad(lines []string, w, h int) []string {
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return lines[:h]
}

func clock(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// progressBar is the bar w cells wide, filled to halves half-cells: ━ for a
// whole cell, ╸ for a half, then the faint rest, so the head moves twice as
// finely as a cell. When the playing role is a hex colour (a theme, the
// config's, or the cover's), the fill fades up to it from a darker shade of
// the same hue; the terminal's own palette can't be blended, so it stays flat.
func progressBar(halves, w int) string {
	full := min(halves/2, w)
	half := 0
	if full < w {
		half = halves % 2
	}
	var b strings.Builder
	if _, basic := playingColor.(ansi.BasicColor); basic || full < 2 {
		b.WriteString(accentStyle.Render(strings.Repeat("━", full)))
	} else {
		for _, c := range lipgloss.Blend1D(full, lipgloss.Darken(playingColor, 0.5), playingColor) {
			b.WriteString(lipgloss.NewStyle().Foreground(c).Bold(true).Render("━"))
		}
	}
	if half == 1 {
		b.WriteString(accentStyle.Render("╸"))
	}
	b.WriteString(dimStyle.Render(strings.Repeat("─", w-full-half)))
	return b.String()
}

// halvesDone is how many half-cells of a w cell bar the position has covered.
func halvesDone(pos, dur time.Duration, w int) int {
	if dur <= 0 {
		return 0
	}
	return min(2*w, int(int64(2*w)*int64(max(pos, 0))/int64(dur)))
}
