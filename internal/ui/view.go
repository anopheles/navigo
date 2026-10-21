package ui

import (
	"fmt"
	"math"
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
	cursorStyle = lipgloss.NewStyle().Reverse(true)   // the cursor in the focused window
	cursorDim   = lipgloss.NewStyle().Underline(true) // the cursor in the other one
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
	if m.w < narrow {
		m.paneAt[m.focus] = paneHit{0, m.w}
		if m.focus == libraryWindow {
			title := m.libTitle() + "   " + m.keyHint("window", "queue")
			lines = m.pane(title, m.libraryRows(m.w-2, h, true), m.w, true)
		} else {
			title := m.queueTitle() + "   " + m.keyHint("window", "library")
			lines = m.pane(title, m.queueRows(m.w-2, h, true), m.w, true)
		}
	} else {
		lw := m.w * 3 / 5
		rw := m.w - lw
		m.paneAt = [2]paneHit{{0, lw}, {lw, rw}}
		left := m.pane(
			m.libTitle(),
			m.libraryRows(lw-2, h, m.focus == libraryWindow),
			lw,
			m.focus == libraryWindow,
		)
		right := m.pane(
			m.queueTitle(),
			m.queueRows(rw-2, h, m.focus == queueWindow),
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
	lines = append(
		lines,
		dimStyle.Render(strings.Repeat("─", max(m.w, 0))),
	) // the rule above the bar
	m.bar = barHit{}
	m.npRows[0] = len(lines)
	if m.cardOn() {
		lines = append(lines, m.nowPlayingCard()...)
	} else {
		lines = append(lines, m.nowPlaying()...)
	}
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
	for _, f := range m.lib[1:] {
		t := f.title
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
	var total time.Duration
	for _, e := range m.state.Queue {
		total += e.Track.Duration
	}
	return t + " · " + span(total)
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
	var out []string
	switch {
	case n > 0:
	case f.kind == homeAlbums:
		out = append(out, dimStyle.Render(fit(m.emptyHome(f.homeKind), w)))
	case f.kind == helpList && f.help != nil:
		out = append(out, dimStyle.Render(fit(m.emptyHelp(f.help), w)))
	case f.kind == searchList && f.search != nil:
		for _, l := range m.emptySearch(f.search) {
			out = append(out, dimStyle.Render(fit(l, w)))
		}
	default:
		out = append(out, dimStyle.Render(fit(m.emptyList(), w)))
	}
	for i := f.offset; i < n && len(out) < h; i++ {
		m.dress = i != f.cur && !m.inSelection(libraryWindow, i) && (!focused || !m.highlighting())
		row := m.libraryRow(f, i, w)
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

// emptyList says why a list has no rows.
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
	}
	return " loading…"
}

func (m *Model) libraryRow(f *frame, i, w int) string {
	lib := m.state.Library
	switch f.kind {
	case sectionList:
		return fit(" "+sections[i].title, w)
	case artistList:
		return m.artistLine(m.artistsOf(f)[i], w)
	case artistPage:
		rows := m.artistRows(f.artist)
		if a, ok := rows.album(i); ok {
			return m.albumLine(a, w)
		}
		return dimStyle.Render(fit(" ── Appears on ──", w))
	case albumList:
		return m.albumLine(m.albumsOf(f)[i], w)
	case homeList:
		return fit(" "+homeLists[i].title, w)
	case homeAlbums:
		return m.albumLine(m.albumsOf(f)[i], w)
	case playlistList:
		p := lib.Playlists[i]
		n := count(len(p.Tracks))
		return " " + fit(p.Name, w-2-len(n)-2) + "  " + n + " "
	case playlistTracks:
		ts := lib.PlaylistTracks(f.playlist)
		return m.trackLine("  ", fmt.Sprint(i+1), ts[i], w)
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
	return " " + fit(
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
	title := m.starred(string(t.ID), t.Title)
	if w < 40 {
		return fit(mark+title, w)
	}
	if m.dress {
		return m.trackLineDressed(mark, num, t, w)
	}
	numW := max(len(num)+1, 3)
	rest := w - lipgloss.Width(mark) - numW - 6
	tw := rest * 5 / 9
	return mark + fmt.Sprintf(
		"%*s ",
		numW-1,
		num,
	) + fit(
		title,
		tw,
	) + fit(
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
	m.queueOff = scroll(m.queueCur, m.queueOff, h, len(q))
	var out []string
	for i := m.queueOff; i < len(q) && len(out) < h; i++ {
		mark := "  "
		_, row, ok := m.state.NowPlaying()
		current := ok && i == row
		if current {
			mark = "▶ "
			if m.state.Player.Paused {
				mark = "⏸ "
			}
		}
		quiet := i != m.queueCur && !current && !m.inSelection(queueWindow, i) &&
			(!focused || !m.highlighting())
		played := quiet && ok && i < row
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
			m.styleRow(line, i == m.queueCur, focused, current, m.inSelection(queueWindow, i)),
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
		join(keyStyle.Render(":q") + " " + dimStyle.Render("quit")),
	}
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
	case cursor:
		return cursorDim.Render(line)
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
	if m.promptHelp {
		return " " + hint("<CR>", "keep the filter") + "  " + hint("<Esc>", "put it back")
	}
	if m.mode == keymap.Search {
		return " " + hint(
			"<CR>",
			"go to the results",
		) + "  " + hint(
			"<Esc>",
			"cancel",
		) + "  " + hint(
			"<Up>/<Down>",
			"history",
		)
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

// nowPlaying is the two-line bar: the track and indicators, then progress.
func (m *Model) nowPlaying() []string {
	s := m.state
	t, row, playing := s.NowPlaying()
	left := " ■ nothing playing"
	var right []string
	if playing {
		icon := "▶"
		if s.Player.Paused {
			icon = "⏸"
		}
		left = fmt.Sprintf(" %s %s — %s · %s", icon, t.Title, t.Artist, t.Album)
		if row >= 0 {
			right = append(right, fmt.Sprintf("%d/%d in queue", row+1, len(s.Queue)))
		} else {
			right = append(right, "not in the queue")
		}
	}
	if s.Player.Buffering {
		right = append([]string{"buffering"}, right...)
	}
	// Indicators get what's left after a readable start of the left part.
	room := m.w - min(lipgloss.Width(left), 30) - 2 - lipgloss.Width(strings.Join(right, " · "))
	if ind := fitIndicators(m.indicators(), room-3); ind != "" {
		right = append([]string{ind}, right...)
	}
	r := strings.Join(right, " · ")
	if r != "" {
		r += " "
	}
	l1 := fit(left, m.w-lipgloss.Width(r)) + r
	flags := m.flags()
	if !playing {
		return []string{(l1), (fit("", m.w-lipgloss.Width(flags)) + flags)}
	}

	dur := s.Player.Duration
	times := fmt.Sprintf(" %s / %s ", clock(s.Player.Position), clock(dur)) + flags
	pw := max(m.w-lipgloss.Width(times)-2, 0)
	l2 := " " + progressBar(halvesDone(s.Player.Position, dur, pw), pw) + times + " "
	m.bar = barHit{y: m.npRows[0] + 1, x: 1, w: pw}
	return []string{(l1), (fit(l2, m.w))}
}

// flags are the now-playing bar's modes: repeat, and the volume when it
// isn't full.
func (m *Model) flags() string {
	var out []string
	if r := m.state.Repeat; r != app.RepeatOff {
		out = append(out, "repeat "+r.String())
	}
	if v := m.state.Player.Volume; v != 100 {
		out = append(out, fmt.Sprintf("vol %d%%", v))
	}
	if len(out) == 0 {
		return ""
	}
	return " " + strings.Join(out, " · ") + " "
}

// status is the bottom line: the ":" line, or a message, and pending keys.
func (m *Model) status() string {
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

// span is a length that may pass an hour: 48:10, or 2:03:41.
func span(d time.Duration) string {
	s := int(d.Round(time.Second).Seconds())
	if s < 3600 {
		return clock(d)
	}
	return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
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
