package ui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// narrow is the width below which only one window is shown.
const narrow = 100

var (
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	cursorDim    = lipgloss.NewStyle().Background(lipgloss.Color("238"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	keyStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	accentStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	errStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	barStyle     = lipgloss.NewStyle().Background(lipgloss.Color("236"))
	titleOn      = lipgloss.NewStyle().Reverse(true).Bold(true)
	boxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("81")).Padding(1, 2)
	focusedInput = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
)

func (m *Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
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
	if m.w < narrow {
		if m.focus == libraryWindow {
			lines = append(lines, titleOn.Render(fit(m.libTitle()+"   "+m.keyHint("window", "queue"), m.w)))
			lines = append(lines, m.libraryRows(m.w, h, true)...)
		} else {
			lines = append(lines, titleOn.Render(fit(m.queueTitle()+"   "+m.keyHint("window", "library"), m.w)))
			lines = append(lines, m.queueRows(m.w, h, true)...)
		}
	} else {
		lw := m.w * 3 / 5
		rw := m.w - lw - 1
		lines = append(lines, m.title(m.libTitle(), lw, m.focus == libraryWindow)+" "+m.title(m.queueTitle(), rw, m.focus == queueWindow))
		left := m.libraryRows(lw, h, m.focus == libraryWindow)
		right := m.queueRows(rw, h, m.focus == queueWindow)
		for i := range h {
			lines = append(lines, left[i]+dimStyle.Render("│")+right[i])
		}
	}
	if m.hints {
		lines = append(lines, barStyle.Render(fit(m.hintLine(), m.w)))
	}
	lines = append(lines, m.nowPlaying()...)
	lines = append(lines, m.status())
	return strings.Join(lines, "\n")
}

func (m *Model) title(s string, w int, focused bool) string {
	if focused {
		return titleOn.Render(fit(s, w))
	}
	return barStyle.Render(fit(s, w))
}

func (m *Model) libTitle() string {
	var parts []string
	for _, f := range m.lib[1:] {
		parts = append(parts, f.title)
	}
	if len(parts) == 0 {
		return " " + m.lib[0].title
	}
	return " " + strings.Join(parts, " › ")
}

func (m *Model) queueTitle() string { return fmt.Sprintf(" Queue (%d)", len(m.state.Queue)) }

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
	if n == 0 {
		out = append(out, dimStyle.Render(fit(m.emptyList(), w)))
	}
	for i := f.offset; i < n && len(out) < h; i++ {
		out = append(out, m.styleRow(m.libraryRow(f, i, w), i == f.cur, focused, false))
	}
	return pad(out, w, h)
}

// emptyList says why a list has no rows.
func (m *Model) emptyList() string {
	switch sy := m.state.Sync; {
	case m.state.Library != nil:
		return " nothing here"
	case sy.Running && sy.Total > 0:
		return fmt.Sprintf(" Syncing library… %d / %d tracks", sy.Done, sy.Total)
	case sy.Running:
		return " Syncing library…"
	}
	return " loading…"
}

func (m *Model) libraryRow(f *frame, i, w int) string {
	switch f.kind {
	case sectionList:
		return fit(" "+sections[i], w)
	case albumList:
		a := m.state.Library.Albums[i]
		year := ""
		if a.Year > 0 {
			year = fmt.Sprint(a.Year)
		}
		rest := w - 1 - 6
		nw := rest * 3 / 5
		return " " + fit(a.Name, nw) + fit("  "+a.Artist, rest-nw) + fmt.Sprintf("%6s", year)
	}
	ts := m.state.Library.AlbumTracks(f.album)
	return trackRow("  ", trackNumber(ts, i), ts[i], w)
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

func trackRow(mark, num string, t library.Track, w int) string {
	if w < 40 {
		return fit(mark+t.Title, w)
	}
	numW := max(len(num)+1, 3)
	rest := w - lipgloss.Width(mark) - numW - 6
	tw := rest * 5 / 9
	return mark + fmt.Sprintf("%*s ", numW-1, num) + fit(t.Title, tw) + fit("  "+t.Artist, rest-tw) + fmt.Sprintf("%6s", clock(t.Duration))
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
		current := i == m.state.Current && m.state.Player.Playing
		if current {
			mark = "▶ "
			if m.state.Player.Paused {
				mark = "⏸ "
			}
		}
		out = append(out, m.styleRow(trackRow(mark, fmt.Sprint(i+1), q[i].Track, w), i == m.queueCur, focused, current))
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
		join(k(containers, "albums", "albums"), pair(k(containers, "down", ""), k(containers, "up", "move"))),
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

func (m *Model) styleRow(line string, cursor, focused, current bool) string {
	switch {
	case cursor && focused:
		return cursorStyle.Render(line)
	case cursor:
		return cursorDim.Render(line)
	case current:
		return accentStyle.Render(line)
	}
	return line
}

// hintLine lists the keys that matter for the row under the cursor and the
// mode, from the bindings in effect.
func (m *Model) hintLine() string {
	if m.mode == keymap.Cmdline {
		return " " + hint("<CR>", "run") + "  " + hint("<Esc>", "cancel")
	}
	if m.engine.Pending() != "" {
		return dimStyle.Render(" waiting for the next key · ") + hint("<Esc>", "cancel")
	}
	var names []string
	switch {
	case m.focus == queueWindow && len(m.state.Queue) == 0:
		names = []string{"window", "albums"}
	case m.focus == queueWindow:
		names = []string{"play-row", "play-pause", "undo", "window"}
	case m.top().kind == albumTracks:
		names = []string{"play-now", "add-end", "add-next", "back", "window"}
	default:
		names = []string{"open", "back", "window"}
	}
	var parts []string
	for _, name := range names {
		if name == "back" && len(m.lib) <= 1 {
			continue
		}
		c, _ := m.registry.Get(name)
		if ks := m.engine.Table().Keys(keymap.Normal, m.scopes(), name); len(ks) > 0 && c != nil {
			parts = append(parts, hint(ks[0], c.Label))
		}
	}
	return " " + strings.Join(parts, "  ")
}

func hint(key, label string) string { return keyStyle.Render(key) + " " + dimStyle.Render(label) }

// nowPlaying is the two-line bar: the track, then its progress.
func (m *Model) nowPlaying() []string {
	s := m.state
	syncing := ""
	if s.Sync.Running {
		syncing = "⟳ syncing · "
	}
	if !s.Player.Playing || s.Current < 0 || s.Current >= len(s.Queue) {
		right := strings.TrimSuffix(syncing, " · ") + " "
		return []string{barStyle.Render(fit(" ■ nothing playing", m.w-lipgloss.Width(right)) + right), barStyle.Render(fit("", m.w))}
	}
	t := s.Queue[s.Current].Track
	icon := "▶"
	if s.Player.Paused {
		icon = "⏸"
	}
	l1 := fmt.Sprintf(" %s %s — %s · %s", icon, t.Title, t.Artist, t.Album)
	right := syncing + fmt.Sprintf("%d/%d in queue ", s.Current+1, len(s.Queue))
	if s.Player.Buffering {
		right = "buffering · " + right
	}
	l1 = fit(l1, m.w-lipgloss.Width(right)) + right

	dur := s.Player.Duration
	times := fmt.Sprintf(" %s / %s ", clock(s.Player.Position), clock(dur))
	pw := max(m.w-lipgloss.Width(times)-2, 0)
	fill := 0
	if dur > 0 {
		fill = min(pw, int(int64(pw)*int64(s.Player.Position)/int64(dur)))
	}
	l2 := " " + accentStyle.Render(strings.Repeat("━", fill)) + dimStyle.Render(strings.Repeat("─", pw-fill)) + times + " "
	return []string{barStyle.Render(l1), barStyle.Render(fit(l2, m.w))}
}

// status is the bottom line: the ":" line, or a message, and pending keys.
func (m *Model) status() string {
	if m.mode == keymap.Cmdline {
		return fit(m.line.View(), m.w)
	}
	right := m.engine.Pending()
	if right != "" {
		right += "  "
	}
	left := " " + m.msg
	if m.msgErr {
		left = " " + errStyle.Render(m.msg)
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
		rows = append(rows, errStyle.Render(f.err), "Save the password in "+m.deps.ConfigPath+" instead? "+hint("y", "yes")+" "+hint("n", "only for this session"))
	case f.busy:
		rows = append(rows, dimStyle.Render("checking with the server…"))
	case f.err != "":
		rows = append(rows, errStyle.Render(f.err))
	default:
		rows = append(rows, "")
	}
	rows = append(rows, "", hint("<Tab>", "next field")+"  "+hint("<CR>", "log in")+"  "+hint("<Esc>", "quit"))
	box := boxStyle.Render(strings.Join(rows, "\n"))
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, box)
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
