package ui

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/keymap"
)

// helpRow is one row of Help: a group's heading, or a key with its
// command line and what it does.
type helpRow struct {
	heading         string
	key, line, desc string
}

// helpView is the Help section's rows, built from the bindings in effect
// when it opened, and the filter typed after /.
type helpView struct {
	filter string
	all    []helpRow
	rows   []helpRow
}

// scopeTitles name Help's groups, in the order they show.
var scopeTitles = []struct{ scope, title string }{
	{"global", "Everywhere"},
	{"containers", "On artists, albums and playlists"},
	{"sections", "On the section list"},
	{"artists", "In Artists"},
	{"albums", "In Albums"},
	{"playlists", "In Playlists"},
	{"tracks", "On tracks"},
	{"album-tracks", "On an album's tracks"},
	{"playlist-tracks", "On a playlist's tracks"},
	{"queue", "In the queue"},
	{"search", "In search results"},
	{"help", "In Help"},
	{"popup", "In a popup"},
}

// keyLabel shows a key sequence as typed: "<" rather than its notation.
func keyLabel(seq string) string { return strings.ReplaceAll(seq, "<lt>", "<") }

// buildHelp lists every binding by scope, Visual mode's after them, then
// the commands no key runs.
func (m *Model) buildHelp() *helpView {
	t := m.engine.Table()
	var rows []helpRow
	bound := map[string]bool{}
	group := func(title string, mode keymap.Mode, scope string) {
		bs := t.Bindings(mode, scope)
		if len(bs) == 0 {
			return
		}
		rows = append(rows, helpRow{heading: title})
		for _, b := range bs {
			bound[b.Name()] = true
			desc := ""
			if c, ok := m.registry.Get(b.Name()); ok {
				desc = c.Desc
			}
			rows = append(rows, helpRow{key: keyLabel(b.Seq()), line: b.Command, desc: desc})
		}
	}
	known := map[string]bool{}
	for _, s := range scopeTitles {
		known[s.scope] = true
		group(s.title, keymap.Normal, s.scope)
	}
	for _, s := range t.Scopes(keymap.Normal) {
		if !known[s] {
			group("In "+s, keymap.Normal, s)
		}
	}
	for _, s := range t.Scopes(keymap.Visual) {
		title := "Visual mode"
		if s != "global" {
			title += ", in " + s
		}
		group(title, keymap.Visual, s)
	}
	if bs := t.Bindings(keymap.Cmdline, "global"); len(bs) > 0 {
		rows = append(rows, helpRow{heading: "On the : line and in prompts"})
		for _, b := range bs {
			rows = append(
				rows,
				helpRow{key: keyLabel(b.Seq()), line: b.Command, desc: lineCommands[b.Command]},
			)
		}
	}
	if bs := t.Bindings(keymap.Search, "global"); len(bs) > 0 {
		rows = append(rows, helpRow{heading: "In the library search prompt"})
		for _, b := range bs {
			desc := ""
			if c, ok := m.registry.Get(b.Command); ok {
				desc = c.Desc
			}
			rows = append(rows, helpRow{key: keyLabel(b.Seq()), line: b.Command, desc: desc})
		}
	}
	var unbound []helpRow
	for _, c := range m.registry.All() {
		if !bound[c.Name] {
			unbound = append(unbound, helpRow{key: ":" + c.Name, line: c.Name, desc: c.Desc})
		}
	}
	if len(unbound) > 0 {
		rows = append(
			append(rows, helpRow{heading: "Commands without a key: type them after :"}),
			unbound...)
	}
	v := &helpView{all: rows}
	v.apply("")
	return v
}

// apply filters the rows: every word must be in the key, command or
// description. Headings stay over the rows they still have.
func (v *helpView) apply(filter string) {
	v.filter = filter
	words := strings.Fields(strings.ToLower(filter))
	v.rows = v.rows[:0]
	var heading *helpRow
	for i := range v.all {
		r := &v.all[i]
		if r.heading != "" {
			heading = r
			continue
		}
		text := strings.ToLower(r.key + " " + r.line + " " + r.desc)
		if !slices.ContainsFunc(words, func(w string) bool { return !strings.Contains(text, w) }) {
			if heading != nil {
				v.rows = append(v.rows, *heading)
				heading = nil
			}
			v.rows = append(v.rows, *r)
		}
	}
}

// showHelp opens the Help section, rebuilt from the bindings now.
func (m *Model) showHelp() {
	m.showSection(helpList)
	f := &m.lib[1]
	filter := ""
	if f.help != nil {
		filter = f.help.filter
	}
	f.help = m.buildHelp()
	f.help.apply(filter)
	m.clamp()
	m.settle(f, 1)
}

func (m *Model) helpRowText(v *helpView, i, w int) string {
	r := v.rows[i]
	if r.heading != "" {
		return dimStyle.Render(fit(" ── "+r.heading+" ──", w))
	}
	return fit(
		" "+keyStyle.Render(fit(r.key, 10))+" "+fit(r.line, 18)+" "+dimStyle.Render(r.desc),
		w,
	)
}

func (m *Model) emptyHelp(v *helpView) string {
	return fmt.Sprintf(" nothing in Help matches %q · / changes the filter", v.filter)
}

// startHelpFilter is / in Help: a prompt that filters it as you type.
func (m *Model) startHelpFilter() {
	f := m.top()
	if f.kind != helpList || f.help == nil {
		return
	}
	m.helpBefore = f.help.filter
	m.promptHelp = true
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "filter help: "
	m.fitLine()
	m.line.SetValue(f.help.filter)
	m.line.CursorEnd()
	m.then(m.line.Focus())
}

// helpKey handles the keys of the filter prompt other than editing; it
// reports whether it handled k.
func (m *Model) helpKey(key string) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		before := m.helpBefore
		m.leaveLine()
		m.filterHelp(before)
		return nil, true
	case "enter":
		m.leaveLine()
		return nil, true
	}
	return nil, false
}

// filterHelp shows the Help rows that match filter.
func (m *Model) filterHelp(filter string) {
	f := m.top()
	if f.kind != helpList || f.help == nil {
		return
	}
	f.help.apply(filter)
	f.cur, f.offset = 0, 0
	m.settle(f, 1)
}
