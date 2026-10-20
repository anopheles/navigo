package ui

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/keymap"
	"github.com/anopheles/navigo/internal/library"
)

// caps are how many results of each group show before a "… N more" row.
var caps = [3]int{library.ArtistGroup: 10, library.AlbumGroup: 20, library.TrackGroup: 200}

var groupNames = [3]string{"Artists", "Albums", "Tracks"}

// searchView is the Search section's results for one query. Frames share
// it by pointer, so the section's saved path sees the same results.
type searchView struct {
	query    string
	res      library.Results
	expanded [3]bool
	rows     []searchRow
}

// searchRow is one row of the results: a group's heading, a result, or
// the "… N more" row that ends a capped group.
type searchRow struct {
	group   library.Group
	i       int // the result's index in its group; -1 on a heading
	more    int // > 0 on the "… N more" row
	heading bool
}

// run searches lib again, as after typing or a sync.
func (v *searchView) run(lib *library.Snapshot) {
	v.res = library.Results{Best: -1}
	if lib != nil {
		v.res = lib.Search(v.query)
	}
	v.layout()
}

func (v *searchView) layout() {
	v.rows = v.rows[:0]
	for g := library.ArtistGroup; g <= library.TrackGroup; g++ {
		n := v.res.Len(g)
		if n == 0 {
			continue
		}
		v.rows = append(v.rows, searchRow{group: g, i: -1, heading: true})
		shown := n
		if !v.expanded[g] {
			shown = min(n, caps[g])
		}
		for i := range shown {
			v.rows = append(v.rows, searchRow{group: g, i: i})
		}
		if shown < n {
			v.rows = append(v.rows, searchRow{group: g, i: -1, more: n - shown})
		}
	}
}

// best is the row of the best match, or 0.
func (v *searchView) best() int {
	if v.res.Best < 0 {
		return 0
	}
	return slices.IndexFunc(v.rows, func(r searchRow) bool { return r.group == v.res.Best && r.i == 0 })
}

func (v *searchView) row(i int) searchRow {
	if i < 0 || i >= len(v.rows) {
		return searchRow{i: -1, heading: true}
	}
	return v.rows[i]
}

func (v *searchView) artist(i int) (library.Artist, bool) {
	if r := v.row(i); r.group == library.ArtistGroup && r.i >= 0 {
		return v.res.Artists[r.i], true
	}
	return library.Artist{}, false
}

func (v *searchView) album(i int) (library.Album, bool) {
	if r := v.row(i); r.group == library.AlbumGroup && r.i >= 0 {
		return v.res.Albums[r.i], true
	}
	return library.Album{}, false
}

func (v *searchView) track(i int) (library.Track, bool) {
	if r := v.row(i); r.group == library.TrackGroup && r.i >= 0 {
		return v.res.Tracks[r.i], true
	}
	return library.Track{}, false
}

// rowID names what row i shows, for keeping the cursor across a sync.
func (v *searchView) rowID(i int) string {
	r := v.row(i)
	switch {
	case r.heading:
		return ""
	case r.more > 0:
		return fmt.Sprintf("more:%d", r.group)
	}
	if a, ok := v.artist(i); ok {
		return "artist:" + string(a.ID)
	}
	if a, ok := v.album(i); ok {
		return "album:" + string(a.ID)
	}
	t, _ := v.track(i)
	return "track:" + string(t.ID)
}

func searchTitle(q string) string { return "Search “" + q + "”" }

// savedView is the library window as it was before the search prompt
// opened, for <Esc> to put back.
type savedView struct {
	lib   []frame
	paths map[listKind][]frame
	focus window
	query string
}

// searchFrame switches to the Search section's results list and returns
// it.
func (m *Model) searchFrame() *frame {
	m.showSection(searchList)
	f := &m.lib[1]
	if f.search == nil {
		f.search = &searchView{}
	}
	return f
}

// search is 's and :search: with text it searches at once, without it
// opens the prompt on the last query.
func (m *Model) search(c command.Call) error {
	if len(c.Args) > 0 {
		q := strings.Join(c.Args, " ")
		m.liveSearch(q)
		m.then(m.remember(&m.searchHist, q))
		return nil
	}
	m.startSearch()
	return nil
}

// startSearch opens the search prompt with the last query selected, so
// typing replaces it.
func (m *Model) startSearch() {
	before := &savedView{lib: slices.Clone(m.lib), paths: maps.Clone(m.paths), focus: m.focus}
	for k, p := range before.paths {
		before.paths[k] = slices.Clone(p)
	}
	f := m.searchFrame()
	before.query = f.search.query
	m.beforeSearch = before

	last := m.searchHist.last()
	m.mode = keymap.Search
	m.line.Reset()
	m.line.Prompt = "search: "
	m.fitLine()
	m.line.SetValue(last)
	m.line.CursorEnd()
	m.selected = last != ""
	m.searchHist.begin(last != "")
	m.then(m.line.Focus())
	m.liveSearch(last)
}

// liveSearch shows the results for q, with the cursor on the best match.
func (m *Model) liveSearch(q string) {
	f := m.searchFrame()
	v := f.search
	v.query, v.expanded = q, [3]bool{}
	v.run(m.state.Library)
	f.title = searchTitle(q)
	f.cur, f.offset = v.best(), 0
	m.settle(f, 1)
}

// searchKey handles the keys of the search prompt that differ from the
// ":" line. It reports whether it handled k.
func (m *Model) searchKey(key string, k tea.KeyPressMsg) (tea.Cmd, bool) {
	switch key {
	case "esc", "ctrl+c":
		m.cancelSearch()
		return nil, true
	case "enter":
		q := m.line.Value()
		m.leaveLine()
		if strings.TrimSpace(q) == "" {
			m.cancelSearch()
			return nil, true
		}
		m.beforeSearch = nil
		m.focus = libraryWindow
		return m.remember(&m.searchHist, q), true
	case "backspace":
		if m.line.Value() == "" {
			m.cancelSearch()
			return nil, true
		}
	case "up", "down":
		m.historyStep(key == "up")
		return nil, true
	}
	if m.selected {
		m.selected = false
		switch {
		case k.Text != "", key == "backspace":
			m.line.SetValue("")
		}
	}
	return nil, false
}

// cancelSearch puts back the view from before the prompt opened.
func (m *Model) cancelSearch() {
	m.leaveLine()
	b := m.beforeSearch
	m.beforeSearch = nil
	if b == nil {
		return
	}
	m.lib, m.paths, m.focus = b.lib, b.paths, b.focus
	for _, f := range m.searchFrames() {
		f.search.query = b.query
		f.search.run(m.state.Library)
		f.title = searchTitle(b.query)
	}
	m.clamp()
}

// searchFrames are every results frame, shown or saved.
func (m *Model) searchFrames() []*frame {
	var out []*frame
	add := func(fs []frame) {
		for i := range fs {
			if fs[i].search != nil {
				out = append(out, &fs[i])
			}
		}
	}
	add(m.lib)
	for _, p := range m.paths {
		add(p)
	}
	return out
}

// historyStep shows an older (or newer) search from the history.
func (m *Model) historyStep(older bool) {
	q := m.searchHist.step(older, m.line.Value())
	m.selected = false
	m.line.SetValue(q)
	m.line.CursorEnd()
	m.liveSearch(q)
}

// searchRowText draws row i of the results.
func (m *Model) searchRowText(v *searchView, i, w int) string {
	r := v.row(i)
	switch {
	case r.heading:
		return dimStyle.Render(fit(fmt.Sprintf(" ── %s · %d ──", groupNames[r.group], v.res.Len(r.group)), w))
	case r.more > 0:
		return dimStyle.Render(fit(fmt.Sprintf("   … %d more", r.more), w))
	}
	if a, ok := v.artist(i); ok {
		return m.artistLine(a, w)
	}
	if a, ok := v.album(i); ok {
		return m.albumLine(a, w)
	}
	t, _ := v.track(i)
	t.Artist += " · " + t.Album // what it matched on
	return m.trackLine(" ", "", t, w)
}

// emptySearch says why there are no results.
func (m *Model) emptySearch(v *searchView) []string {
	if strings.TrimSpace(v.query) == "" {
		return []string{" type to search the library"}
	}
	lines := []string{fmt.Sprintf(" nothing in the library matches %q", v.query)}
	if m.state.Sync.Running && m.state.Library == nil {
		lines = append(lines, " still syncing, results may be incomplete")
	}
	return lines
}
