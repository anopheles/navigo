package ui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/keymap"
)

// keyMenuRows is the most rows the key menu shows before it scrolls.
const keyMenuRows = 24

type prefixMsg struct{ seq int }

// menuGroups sort the key menu's commands. Motions aren't listed: they
// scroll the menu.
var menuGroups = []struct {
	title string
	names []string
}{
	{
		"Play",
		[]string{
			"play-now",
			"play-row",
			"play-pause",
			"next-track",
			"prev-track",
			"seek",
			"stop",
			"volume",
			"shuffle",
			"mix",
			"mix-add-end",
			"mix-add-next",
			"radio",
			"radio-more",
			"radio-less",
			"repeat",
			"cover",
		},
	},
	{
		"Queue",
		[]string{
			"add-end",
			"add-next",
			"play-this",
			"delete",
			"remove",
			"yank",
			"paste-after",
			"paste-before",
			"move-down",
			"move-up",
			"undo",
			"redo",
			"playlist-add",
			"playlist-undo",
			"playlist-delete",
		},
	},
	{"Select", []string{"visual", "star"}},
	{
		"Go",
		[]string{
			"open",
			"back",
			"go-playing",
			"go-album",
			"go-artist",
			"window",
			"queue",
			"artists",
			"albums",
			"playlists",
			"search",
			"find",
			"help-filter",
		},
	},
	{"Help", []string{"info", "key-menu", "help", "messages", "cmdline", "quit"}},
}

// keyMenu is the key menu's rows, by group, and the filter typed after /.
type keyMenu struct {
	rows   []keyMenuRow
	width  int // the label column
	filter string
	before string // the filter <Esc> at the prompt puts back
}

// keyMenuRow is one row of the key menu: a group's title, or a key with
// its label, command line and what it does.
type keyMenuRow struct {
	title                  string
	key, label, line, desc string
}

// openKeyMenu is ?: the keys for what is under the cursor, by group.
// While it is open, keys run in the scopes it was opened in.
func (m *Model) openKeyMenu() {
	under := m.scopes()
	bs := m.engine.Table().Effective(keymap.Normal, under)
	type row struct {
		keyMenuRow
		local bool // bound here, not everywhere
	}
	global := m.engine.Table().Bindings(keymap.Normal, "global")
	isLocal := func(b keymap.Binding) bool {
		return !slices.ContainsFunc(global, func(g keymap.Binding) bool {
			return g.Seq() == b.Seq() && g.Command == b.Command
		})
	}
	var groups [][]row
	width := 18 // the label column, widened for a long label up to 32
	for _, g := range menuGroups {
		var rows []row
		for _, name := range g.names {
			for _, b := range bs {
				if b.Name() != name || b.Seq() == "/" { // / filters the menu
					continue
				}
				c, ok := m.registry.Get(name)
				if !ok {
					continue
				}
				label := c.Label
				_, args, hasArgs := strings.Cut(b.Command, " ")
				switch {
				case name == "mix" || name == "mix-add-end" || name == "mix-add-next":
					label = m.mixLabel(label, name, strings.Fields(args))
				case hasArgs:
					label += " " + args
				}
				width = max(width, min(lipgloss.Width(label), 32))
				r := keyMenuRow{key: keyLabel(b.Seq()), label: label, line: b.Command, desc: c.Desc}
				rows = append(rows, row{r, isLocal(b)})
			}
		}
		groups = append(groups, rows)
	}
	// What only works here comes first, in a group of its own, and not
	// again under its usual heading.
	var here []row
	for i := range groups {
		rest := groups[i][:0]
		for _, r := range groups[i] {
			if r.local {
				here = append(here, r)
			} else {
				rest = append(rest, r)
			}
		}
		groups[i] = rest
	}
	titles := make([]string, 0, len(menuGroups)+1)
	if len(here) > 0 {
		groups = append([][]row{here}, groups...)
		titles = append(titles, "Here")
	}
	for _, g := range menuGroups {
		titles = append(titles, g.title)
	}
	menu := &keyMenu{width: width}
	for i, rows := range groups {
		menu.rows = append(menu.rows, keyMenuRow{title: titles[i]})
		for _, r := range rows {
			menu.rows = append(menu.rows, r.keyMenuRow)
		}
	}
	m.openPopup("Keys here", menu.lines())
	m.popup.keyMenu = under
	m.popup.menu = menu
}

// lines draws the key menu's rows that match its filter, each group's
// title over the rows it still has; the legend shows only unfiltered.
func (k *keyMenu) lines() []string {
	words := filterWords(k.filter)
	var lines []string
	if len(words) == 0 {
		for _, l := range keyLegend {
			lines = append(lines, dimStyle.Render(l))
		}
	}
	title := ""
	for _, r := range k.rows {
		if r.title != "" {
			title = r.title
			continue
		}
		if !matchesWords(words, r.key, r.label, r.line, r.desc) {
			continue
		}
		if title != "" {
			lines = append(lines, accentStyle.Render(title))
			title = ""
		}
		lines = append(
			lines,
			"  "+keyStyle.Render(
				fit(r.key, 8),
			)+" "+fit(
				r.label,
				k.width,
			)+" "+dimStyle.Render(
				r.desc,
			),
		)
	}
	if len(lines) == 0 {
		lines = append(lines, dimStyle.Render(
			fmt.Sprintf("nothing here matches %q · <F1> searches all keys", k.filter),
		))
	}
	return lines
}

// filterKeyMenu shows the key menu's rows that match filter.
func (m *Model) filterKeyMenu(filter string) {
	p := m.popup
	if p == nil || p.menu == nil {
		return
	}
	p.menu.filter = filter
	p.lines = p.menu.lines()
	p.off = 0
	p.title = "Keys here"
	if filter != "" {
		p.title += " /" + filter
	}
}

// startKeyMenuFilter is / in the key menu: a prompt that filters it as
// you type.
func (m *Model) startKeyMenuFilter() {
	k := m.popup.menu
	k.before = k.filter
	m.promptKeys = true
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "filter keys: "
	m.fitLine()
	m.line.SetValue(k.filter)
	m.line.CursorEnd()
	m.then(m.line.Focus())
}

// keyMenuFilterKey handles the filter prompt's keys other than editing:
// <Esc> puts the old filter back, <CR> keeps the new one.
func (m *Model) keyMenuFilterKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		before := ""
		if m.popup != nil && m.popup.menu != nil {
			before = m.popup.menu.before
		}
		m.leaveLine()
		m.filterKeyMenu(before)
		return nil, true
	case "enter":
		m.leaveLine()
		return nil, true
	}
	return nil, false
}

// keyMenuKey handles the keys the key menu keeps for itself; it reports
// whether it handled the key.
func (m *Model) keyMenuKey(token string) (tea.Cmd, bool) {
	if m.engine.Pending() != "" {
		return nil, false
	}
	filter := ""
	if m.popup.menu != nil {
		filter = m.popup.menu.filter
	}
	switch token {
	case "q", "<Esc>":
		if filter != "" { // a filter goes first, then the menu
			m.filterKeyMenu("")
			return nil, true
		}
		m.closePopup()
		return nil, true
	case "/":
		if m.popup.menu != nil {
			m.startKeyMenuFilter()
			return nil, true
		}
	case "<F1>": // Help, filtered as the menu was
		m.closePopup()
		cmd := m.exec("help", 0)
		if filter != "" {
			m.filterHelp(filter)
		}
		return cmd, true
	}
	return nil, false
}

// prefixMenu draws what can follow the keys typed so far, above the
// status line, once a pure prefix has waited keys.hint-delay.
func (m *Model) prefixMenu() []string {
	if !m.prefixShown || m.engine.Pending() == "" {
		return nil
	}
	t := m.engine.Table()
	type entry struct{ keys, name, args string }
	var entries []entry
	if op, opKeys := m.engine.Operator(); op != "" {
		entries = append(entries, entry{strings.Join(opKeys, ""), "", "this row"})
		for _, b := range t.Effective(keymap.Normal, m.scopes()) {
			if c, ok := m.registry.Get(b.Name()); ok && c.Motion {
				entries = append(entries, entry{b.Seq(), b.Name(), ""})
			}
		}
	} else {
		prefix := m.engine.PendingKeys()
		for _, b := range t.Continuations(m.mode, m.scopes(), prefix) {
			_, args, _ := strings.Cut(b.Command, " ")
			entries = append(entries, entry{strings.Join(b.Keys[len(prefix):], ""), b.Name(), args})
		}
	}
	if len(entries) == 0 {
		return nil
	}
	entries = entries[:min(len(entries), max(m.h-6, 1))]
	out := []string{(fit(dimStyle.Render(" after "+m.engine.Pending()+":"), m.w))}
	for _, e := range entries {
		label := e.args
		if c, ok := m.registry.Get(e.name); ok {
			label = strings.TrimSpace(c.Label + " " + e.args)
		}
		row := "   " + keyStyle.Render(fit(keyLabel(e.keys), 6)) + " " + label
		out = append(out, (fit(row, m.w)))
	}
	return out
}

// armPrefix shows the prefix menu if the keys are still pending after
// keys.hint-delay, unless keys.hints is off.
func (m *Model) armPrefix() tea.Cmd {
	if !m.keys.prefixMenus {
		return nil
	}
	seq := m.seq
	return m.clock.After(m.keys.hintDelay, func(time.Time) tea.Msg { return prefixMsg{seq} })
}
