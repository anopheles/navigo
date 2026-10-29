package ui

// PROTOTYPE (throwaway, never merge): Miller-column browsing for #111.
// F2 cycles the layouts live; NAVIGO_COLUMNS=A..E picks the first one.
// The current layout's name shows at the start of the hint line.
//
// Only the drawing changes. The keys are today's: j/k move in the focused
// column (the deepest one opened), l/<CR> open, h goes back. The columns
// right of the focused one are previews of what is under the cursor, so
// they follow it live. Applies to the Artists and Albums sections; the
// others draw as today.

import (
	"fmt"
	"os"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/anopheles/navigo/internal/library"
)

var protoColNames = []string{
	"A today: one list at a time",
	"B full width: columns, queue on <Tab>",
	"C beside the queue: three columns",
	"D beside the queue: focus + preview",
	"E queue below the columns",
}

var protoCol = func() int {
	if v := os.Getenv("NAVIGO_COLUMNS"); len(v) == 1 && v[0] >= 'A' && v[0] < 'A'+byte(len(protoColNames)) {
		return int(v[0] - 'A')
	}
	return 1
}()

func protoColNext() { protoCol = (protoCol + 1) % len(protoColNames) }

func protoColLabel() string {
	return lipgloss.NewStyle().Reverse(true).Render(" F2 " + protoColNames[protoCol] + " ")
}

// protoColumn is one column: a frame of the path, or a preview of what is
// under the cursor of the column left of it.
type protoColumn struct {
	f       *frame
	focused bool // the frame the keys act on
	preview bool
}

func protoColumnKind(k listKind) bool {
	return k == artistList || k == artistPage || k == albumList || k == albumTracks
}

// protoColumns is the path from the section's list to the focused list,
// then previews to its right; false outside Artists and Albums.
func (m *Model) protoColumns() ([]protoColumn, bool) {
	if protoCol == 0 || len(m.lib) < 2 || m.state.Library == nil {
		return nil, false
	}
	if k := m.lib[1].kind; k != artistList && k != albumList {
		return nil, false
	}
	var cols []protoColumn
	for i := 1; i < len(m.lib); i++ {
		if !protoColumnKind(m.lib[i].kind) {
			return nil, false
		}
		cols = append(cols, protoColumn{f: &m.lib[i], focused: i == len(m.lib)-1})
	}
	for {
		next, ok := m.protoPreview(cols[len(cols)-1].f)
		if !ok {
			break
		}
		cols = append(cols, protoColumn{f: next, preview: true})
	}
	if protoCol == 3 && len(cols) > 2 { // focus + preview, or parent + focus
		at := 0
		for i, c := range cols {
			if c.focused {
				at = i
			}
		}
		if at == len(cols)-1 {
			at--
		}
		cols = cols[at : at+2]
	}
	return cols, true
}

// protoPreview is the list under f's cursor.
func (m *Model) protoPreview(f *frame) (*frame, bool) {
	switch f.kind {
	case artistList:
		as := m.artistsOf(f)
		if f.cur >= len(as) {
			return nil, false
		}
		p := &frame{kind: artistPage, artist: as[f.cur].ID, title: as[f.cur].Name}
		m.settle(p, 1)
		return p, true
	case artistPage:
		a, ok := m.artistRows(f.artist).album(f.cur)
		if !ok {
			return nil, false
		}
		return &frame{kind: albumTracks, album: a.ID, title: a.Name}, true
	case albumList:
		as := m.albumsOf(f)
		if f.cur >= len(as) {
			return nil, false
		}
		return &frame{kind: albumTracks, album: as[f.cur].ID, title: as[f.cur].Name}, true
	}
	return nil, false
}

// protoWeight is a column's share of the width.
func protoWeight(k listKind) int {
	switch k {
	case artistList:
		return 3
	case albumTracks:
		return 5
	}
	return 4
}

// protoColumnRows draws the columns side by side, w wide and h tall.
func (m *Model) protoColumnRows(cols []protoColumn, w, h int, windowFocused bool) []string {
	total := 0
	for _, c := range cols {
		total += protoWeight(c.f.kind)
	}
	avail := w - (len(cols) - 1)
	widths := make([]int, len(cols))
	used := 0
	for i, c := range cols {
		widths[i] = avail * protoWeight(c.f.kind) / total
		used += widths[i]
	}
	widths[len(widths)-1] += avail - used

	blocks := make([][]string, len(cols))
	for i, c := range cols {
		blocks[i] = m.protoColumnBlock(c, widths[i], h, windowFocused)
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

// protoColumnBlock is one column: a header with its name, then its rows.
func (m *Model) protoColumnBlock(c protoColumn, w, h int, windowFocused bool) []string {
	f := c.f
	head := " " + f.title
	switch f.kind {
	case artistList:
		head = " Artists"
	case albumList:
		head = " Albums"
	}
	hs := titleOff
	if c.focused && windowFocused {
		hs = titleOn
	} else if c.preview {
		hs = dimStyle
	}
	out := []string{hs.Render(fit(head, w))}
	h--

	n := m.rows(f)
	f.offset = scroll(f.cur, f.offset, h, n)
	m.markQueued()
	if n == 0 {
		out = append(out, dimStyle.Render(fit(m.emptyRows(f), w)))
	}
	live := c.focused && windowFocused
	for i := f.offset; i < n && len(out) < h+1; i++ {
		m.marks.on = true
		row := m.protoRow(f, i, w)
		m.marks.on = false
		if live {
			row = m.highlight(row)
		}
		cursor := i == f.cur && !c.preview
		selected := c.focused && m.inSelection(libraryWindow, i)
		out = append(out, m.styleRow(row, cursor, live, false, selected))
	}
	return pad(out, w, h+1)
}

// protoRow is a row cut down to fit a column: artists by name, albums by
// name and year, tracks as today (which drop the artist below 40 cells).
func (m *Model) protoRow(f *frame, i, w int) string {
	switch f.kind {
	case artistList:
		return " " + fit(m.starred(string(m.artistsOf(f)[i].ID), m.artistsOf(f)[i].Name), w-1)
	case artistPage:
		a, ok := m.artistRows(f.artist).album(i)
		if !ok {
			return dimStyle.Render(fit(" ── Appears on ──", w))
		}
		return m.protoAlbum(a, w)
	case albumList:
		return m.protoAlbum(m.albumsOf(f)[i], w)
	}
	ts := m.tracksOf(f)
	return m.trackLine("  ", trackNumber(ts, i), ts[i], w)
}

func (m *Model) protoAlbum(a library.Album, w int) string {
	year := ""
	if a.Year > 0 && w >= 20 {
		year = fmt.Sprint(a.Year)
	}
	yw := 0
	if year != "" {
		yw = 6
	}
	return m.albumLead(a) + fit(m.starred(string(a.ID), a.Name), w-1-yw) + fmt.Sprintf("%*s", yw, year)
}

// protoBody draws the windows for layouts B to E; false draws them as today.
func (m *Model) protoBody(h int) ([]string, bool) {
	cols, ok := m.protoColumns()
	if !ok {
		return nil, false
	}
	libFocused := m.focus == libraryWindow
	full := func(f bool) []string {
		if libFocused || f {
			m.paneAt = [2]paneHit{{0, m.w}, {}}
			title := m.libTitle() + "   " + m.keyHint("window", "queue")
			return m.pane(title, m.protoColumnRows(cols, m.w-2, h, true), m.w, true)
		}
		m.paneAt = [2]paneHit{{}, {0, m.w}}
		title := m.queueTitle() + "   " + m.keyHint("window", "library")
		return m.pane(title, m.queueRows(m.w-2, h, true), m.w, true)
	}
	if m.w < narrow || protoCol == 1 {
		return full(false), true
	}
	if protoCol == 4 { // queue below
		total := h + 2
		lh := max(total*3/5-2, 3)
		qh := max(total-lh-4, 1)
		top := m.pane(m.libTitle(), m.protoColumnRows(cols, m.w-2, lh, libFocused), m.w, libFocused)
		bottom := m.pane(m.queueTitle(), m.queueRows(m.w-2, qh, !libFocused), m.w, !libFocused)
		m.paneAt = [2]paneHit{{0, m.w}, {}}
		return append(top, bottom...), true
	}
	lw := m.w * 3 / 5
	rw := m.w - lw
	m.paneAt = [2]paneHit{{0, lw}, {lw, rw}}
	left := m.pane(m.libTitle(), m.protoColumnRows(cols, lw-2, h, libFocused), lw, libFocused)
	right := m.pane(
		m.queueTitle(),
		append(m.queueCard(rw-2), m.queueRows(rw-2, m.listHeight(queueWindow), !libFocused)...),
		rw,
		!libFocused,
	)
	var lines []string
	for i := range left {
		lines = append(lines, left[i]+right[i])
	}
	return lines, true
}
