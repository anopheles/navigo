package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/anopheles/navigo/internal/library"
)

// Columns (#111): Artists, Albums and Playlists show their path side by
// side, as Artists | Albums | Tracks, Albums | Tracks and Playlists |
// Tracks. With two windows they share the library window beside the queue,
// as many as fit, sliding along the path; with one they take the whole
// width and the queue is a <Tab> away. The path is the same frames the
// drill-down had, so the keys are too: the focus is the deepest list opened,
// l goes right and h left. The columns right of the focus are previews of
// what is under its cursor, with no cursor of their own.

// columnsMin is the width below which those sections drill down instead.
const columnsMin = 60

// fullTrackRow is the width from which a track row shows its artist and
// rating: the tracks column beside the queue at 120 cells has this many.
const fullTrackRow = 39

// columnShared is the least width of a column beside the queue.
const columnShared = 28

// column is a list drawn as a column: a frame of the path, or a preview.
type column struct {
	f       *frame
	depth   int // its index in m.lib, previews included
	focused bool
	preview bool
}

// colHit is where a column sits on screen, for the mouse.
type colHit struct{ x, w, depth int }

// colKey names a column of a section, for the view it keeps.
type colKey struct {
	section listKind
	depth   int
}

func columnSection(k listKind) bool {
	return k == artistList || k == albumList || k == playlistList
}

func columnKind(k listKind) bool {
	switch k {
	case artistList, artistPage, albumList, albumTracks, playlistList, playlistTracks:
		return true
	}
	return false
}

// inColumns reports whether the library window draws columns: in one of
// their sections, wide enough, and below the section list.
func (m *Model) inColumns() bool {
	if m.w < columnsMin || len(m.lib) < 2 || !columnSection(m.lib[1].kind) {
		return false
	}
	for i := range m.lib[1:] {
		if !columnKind(m.lib[i+1].kind) {
			return false
		}
	}
	return true
}

// columns are the path from the section's list to the focus, then the
// previews right of it.
func (m *Model) columns() []column {
	var cols []column
	for i := 1; i < len(m.lib); i++ {
		cols = append(cols, column{f: &m.lib[i], depth: i, focused: i == len(m.lib)-1})
	}
	for {
		last := cols[len(cols)-1]
		next, ok := m.preview(last.f, last.depth+1)
		if !ok {
			return cols
		}
		cols = append(cols, column{f: next, depth: last.depth + 1, preview: true})
	}
}

// preview is the list under f's cursor, as it would open at depth.
func (m *Model) preview(f *frame, depth int) (*frame, bool) {
	if m.state.Library == nil || f.cur >= m.rows(f) {
		return nil, false
	}
	var p frame
	switch f.kind {
	case artistList:
		a := m.artistsOf(f)[f.cur]
		p = frame{kind: artistPage, artist: a.ID, title: a.Name}
	case artistPage:
		a, ok := m.artistRows(f).album(f.cur)
		if !ok {
			return nil, false
		}
		p = frame{kind: albumTracks, album: a.ID, title: a.Name}
	case albumList:
		a := m.albumsOf(f)[f.cur]
		p = frame{kind: albumTracks, album: a.ID, title: a.Name}
	case playlistList:
		pl := m.playlistsOf(f)[f.cur]
		p = frame{kind: playlistTracks, playlist: pl.ID, title: pl.Name}
	default:
		return nil, false
	}
	p.view = m.colViews[colKey{m.lib[1].kind, depth}]
	m.settle(&p, 1)
	return &p, true
}

// openPreviews makes the previews up to depth part of the path, as l would.
func (m *Model) openPreviews(depth int) {
	for len(m.lib) <= depth {
		f := m.top()
		p, ok := m.preview(f, len(m.lib))
		if !ok {
			return
		}
		m.lib = append(m.lib, *p)
	}
}

// keepColumnView remembers the focused column's sort and filter, so the
// lists that open in that column later take them too.
func (m *Model) keepColumnView() {
	if !m.inColumns() {
		return
	}
	v := m.top().view
	v.key, v.albums, v.appearsOn, v.artists, v.playlists, v.tracks = "", nil, nil, nil, nil, nil
	m.colViews[colKey{m.lib[1].kind, len(m.lib) - 1}] = v
}

// adoptColumnViews gives the path's lists their columns' sort and filter,
// keeping each cursor on its row's item.
func (m *Model) adoptColumnViews() {
	if len(m.lib) < 2 || !columnSection(m.lib[1].kind) {
		return
	}
	for i := 2; i < len(m.lib); i++ {
		v, ok := m.colViews[colKey{m.lib[1].kind, i}]
		f := &m.lib[i]
		if !ok || sameView(v, f.view) {
			continue
		}
		m.changeView(f, func(w *listView) { *w = v })
	}
}

// adoptOpened gives a list just opened at depth its column's sort and
// filter. One opened at its top starts at the top of the sorted rows; one
// opened on a row (an Appears on album) stays on it.
func (m *Model) adoptOpened(depth int) {
	if len(m.lib) <= depth {
		return
	}
	top := m.lib[depth].cur == 0
	m.adoptColumnViews()
	if f := &m.lib[depth]; top {
		f.cur, f.offset = 0, 0
		m.settle(f, 1)
	}
}

func sameView(a, b listView) bool {
	return a.sortBy == b.sortBy && a.desc == b.desc && a.genre == b.genre &&
		a.yearFrom == b.yearFrom && a.yearTo == b.yearTo
}

// shownColumns are the columns that fit in w. Beside the queue, those around
// the focus, keeping one preview right of it while there is one.
func (m *Model) shownColumns(cols []column, w int) []column {
	n := max((w+1)/(columnShared+1), 2)
	if m.w < narrow || len(cols) <= n {
		return cols
	}
	focus := 0
	for i, c := range cols {
		if c.focused {
			focus = i
		}
	}
	start := min(max(focus-(n-2), 0), len(cols)-n)
	return cols[start : start+n]
}

// colWeight is a column's share of the width: 3 : 4 : 5 for artists,
// albums and tracks, 4 : 5 for albums or playlists and tracks.
func colWeight(k listKind) int {
	switch k {
	case artistList:
		return 3
	case albumTracks, playlistTracks:
		return 5
	}
	return 4
}

// columnTitle is the window's title in columns: the section, and the
// focused column's sort or filter.
func (m *Model) columnTitle() string {
	t := " " + m.lib[1].title
	if d := m.top().view.describe(); d != "" {
		t += " (" + d + ")"
	}
	return t
}

// columnRows draws the columns side by side, w wide and h tall, and notes
// where each one sits from x.
func (m *Model) columnRows(x, w, h int, focused bool) []string {
	cols := m.shownColumns(m.columns(), w)
	total := 0
	for _, c := range cols {
		total += colWeight(c.f.kind)
	}
	avail := w - (len(cols) - 1)
	widths := make([]int, len(cols))
	used := 0
	for i, c := range cols {
		widths[i] = avail * colWeight(c.f.kind) / total
		used += widths[i]
	}
	widths[len(widths)-1] += avail - used

	m.colAt = m.colAt[:0]
	m.markQueued()
	blocks := make([][]string, len(cols))
	for i, c := range cols {
		m.colAt = append(m.colAt, colHit{x, widths[i], c.depth})
		x += widths[i] + 1
		blocks[i] = m.columnBlock(c, widths[i], h, focused)
	}
	sep := dimStyle.Render("│")
	out := make([]string, h)
	for r := range h {
		parts := make([]string, len(cols))
		for i := range cols {
			parts[i] = blocks[i][r]
		}
		out[r] = strings.Join(parts, sep)
	}
	return out
}

// columnBlock is one column: a header naming it, then its rows.
func (m *Model) columnBlock(c column, w, h int, windowFocused bool) []string {
	f := c.f
	head := f.title
	if d := f.view.describe(); d != "" {
		head += " (" + d + ")"
	}
	style := titleOff
	switch {
	case c.focused && windowFocused:
		style = titleOn
	case c.preview:
		style = dimStyle
	}
	out := []string{style.Render(fit(" "+head, w))}
	h--

	n := m.rows(f)
	f.offset = scroll(f.cur, f.offset, h, n)
	live := c.focused && windowFocused
	if n == 0 {
		msg := " empty"
		if !c.preview {
			msg = m.emptyRows(f)
		}
		out = append(out, dimStyle.Render(fit(msg, w)))
	}
	for i := f.offset; i < n && len(out) < h+1; i++ {
		cursor := i == f.cur && !c.preview
		selected := c.focused && m.inSelection(libraryWindow, i)
		m.dress = !cursor && !selected && (!live || !m.highlighting())
		m.marks.on = true
		row := m.columnRow(f, i, w)
		m.marks.on = false
		m.dress = false
		if live {
			row = m.highlight(row)
		}
		out = append(out, m.styleRow(row, cursor, live, false, selected))
	}
	return pad(out, w, h+1)
}

// columnRow is row i of a column, cut down to its width: artists by name,
// an artist's albums by name and year, the Albums section's by name and
// artist, playlists with their track count, and tracks as number, title
// and length below fullTrackRow.
func (m *Model) columnRow(f *frame, i, w int) string {
	switch f.kind {
	case artistList:
		a := m.artistsOf(f)[i]
		return " " + m.colName(string(a.ID), a.Name, w-2) + " "
	case artistPage:
		a, ok := m.artistRows(f).album(i)
		if !ok {
			return dimStyle.Render(fit(" ── Appears on ──", w))
		}
		return m.colAlbum(a, false, w)
	case albumList:
		return m.colAlbum(m.albumsOf(f)[i], true, w)
	case playlistList:
		p := m.playlistsOf(f)[i]
		n := count(len(p.Tracks))
		return " " + m.colName(string(p.ID), p.Name, w-2-len(n)-2) + m.faint("  "+n) + " "
	case playlistTracks:
		return m.colTrack(m.playlistTracksOf(f)[i], fmt.Sprint(i+1), w)
	}
	ts := m.tracksOf(f)
	return m.colTrack(ts[i], trackNumber(ts, i), w)
}

// colName is a name fitted to w, its ★ gold when the row is dressed.
func (m *Model) colName(id, name string, w int) string {
	if m.dress {
		return m.name(id, name, w)
	}
	return fit(m.starred(id, name), w)
}

// faint is s faint when the row is dressed.
func (m *Model) faint(s string) string {
	if m.dress {
		return dimStyle.Render(s)
	}
	return s
}

// colAlbum is an album row: name and year, with its artist when withArtist
// (the year then only from 40 cells).
func (m *Model) colAlbum(a library.Album, withArtist bool, w int) string {
	year := ""
	if a.Year > 0 && w >= 20 && (!withArtist || w >= 40) {
		year = fmt.Sprint(a.Year)
	}
	yw := 0
	if year != "" {
		yw = 6
	}
	rest := w - 1 - yw
	if !withArtist {
		return m.albumLead(
			a,
		) + m.colName(
			string(a.ID),
			a.Name,
			rest,
		) + m.faint(
			fmt.Sprintf("%*s", yw, year),
		)
	}
	nw := rest * 3 / 5
	return m.albumLead(a) + m.colName(string(a.ID), a.Name, nw) +
		m.faint(fit("  "+a.Artist, rest-nw)+fmt.Sprintf("%*s", yw, year))
}

// colTrack is a track row; below fullTrackRow it drops the artist and keeps
// the number and length.
func (m *Model) colTrack(t library.Track, num string, w int) string {
	if w >= fullTrackRow {
		return m.trackLine("  ", num, t, w)
	}
	mark := m.trackMark("  ", t)
	numW := max(len(num)+1, 3)
	length := fmt.Sprintf("%6s", clock(t.Duration))
	tw := w - ansi.StringWidth(mark) - numW - len(length)
	if tw < 4 { // too narrow for the number and length
		return fit(mark+m.starred(string(t.ID), t.Title), w)
	}
	return m.faint(mark+fmt.Sprintf("%*s ", numW-1, num)) +
		m.colName(string(t.ID), t.Title, tw) + m.faint(length)
}

// columnAt is the column under screen column x, if any.
func (m *Model) columnAt(x int) (colHit, bool) {
	for _, c := range m.colAt {
		if x >= c.x && x < c.x+c.w {
			return c, true
		}
	}
	return colHit{}, false
}

// focusColumn makes the column at depth the focus: the columns right of
// it close, or the previews up to it open.
func (m *Model) focusColumn(depth int) {
	if depth < len(m.lib) {
		m.lib = m.lib[:depth+1]
	} else {
		m.openPreviews(depth)
	}
	m.checkVisual()
}
