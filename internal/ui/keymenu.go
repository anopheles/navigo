package ui

import (
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
			"repeat",
		},
	},
	{
		"Queue",
		[]string{
			"add-end",
			"add-next",
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
			"go-album",
			"go-artist",
			"window",
			"queue",
			"artists",
			"albums",
			"playlists",
			"search",
			"help-filter",
		},
	},
	{"Help", []string{"info", "key-menu", "help", "messages", "cmdline", "quit"}},
}

// openKeyMenu is ?: the keys for what is under the cursor, by group.
// While it is open, keys run in the scopes it was opened in.
func (m *Model) openKeyMenu() {
	under := m.scopes()
	bs := m.engine.Table().Effective(keymap.Normal, under)
	type row struct{ key, label, desc string }
	var groups [][]row
	width := 18 // the label column, widened for a long label up to 32
	for _, g := range menuGroups {
		var rows []row
		for _, name := range g.names {
			for _, b := range bs {
				if b.Name() != name {
					continue
				}
				c, ok := m.registry.Get(name)
				if !ok {
					continue
				}
				label := c.Label
				if _, args, ok := strings.Cut(b.Command, " "); ok {
					label += " " + args
				} else if name == "mix" {
					label = "mix from " + m.mixFrom()
				}
				width = max(width, min(lipgloss.Width(label), 32))
				rows = append(rows, row{keyLabel(b.Seq()), label, c.Desc})
			}
		}
		groups = append(groups, rows)
	}
	var lines []string
	for i, rows := range groups {
		if len(rows) > 0 {
			lines = append(lines, accentStyle.Render(menuGroups[i].title))
		}
		for _, r := range rows {
			lines = append(
				lines,
				"  "+keyStyle.Render(
					fit(r.key, 8),
				)+" "+fit(
					r.label,
					width,
				)+" "+dimStyle.Render(
					r.desc,
				),
			)
		}
	}
	m.openPopup("Keys here", lines)
	m.popup.keyMenu = under
}

// keyMenuKey handles the keys the key menu keeps for itself; it reports
// whether it handled the key.
func (m *Model) keyMenuKey(token string) (tea.Cmd, bool) {
	if m.engine.Pending() != "" {
		return nil, false
	}
	switch token {
	case "q", "<Esc>":
		m.closePopup()
		return nil, true
	case "<F1>":
		m.closePopup()
		return m.exec("help", 0), true
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
