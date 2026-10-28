package ui

import (
	"strings"

	"github.com/anopheles/navigo/internal/library"
)

// queueMark says a library row is already in the queue. It is a glyph, so
// it reads without colour.
const queueMark = "•"

// rowMarks are the queue's tracks and albums, for the library row being
// drawn. They are off for the queue window, which is the queue itself.
type rowMarks struct {
	on     bool
	tracks map[library.TrackID]bool
	albums map[library.AlbumID]bool
}

// markQueued collects what the queue holds, once per frame.
func (m *Model) markQueued() {
	m.marks = rowMarks{
		tracks: map[library.TrackID]bool{},
		albums: map[library.AlbumID]bool{},
	}
	for _, e := range m.state.Queue {
		m.marks.tracks[e.Track.ID] = true
		m.marks.albums[e.Track.AlbumID] = true
	}
}

// trackMark puts the queue mark in a track row's gutter: in its second
// cell, so the cursor's ▎ keeps the first, or in the only cell of a
// one-cell gutter.
func (m *Model) trackMark(mark string, t library.Track) string {
	if !m.marks.on || !m.marks.tracks[t.ID] {
		return mark
	}
	if mark == " " {
		return queueMark
	}
	if strings.HasSuffix(mark, "  ") {
		return strings.TrimSuffix(mark, " ") + queueMark // "  " is " •"
	}
	return mark
}

// albumLead is the cell before an album's name: the queue mark when any of
// its tracks is queued.
func (m *Model) albumLead(a library.Album) string {
	if m.marks.on && m.marks.albums[a.ID] {
		return queueMark
	}
	return " "
}

// ratingTag is " ★3" for a rated track, or "" where rows show no ratings.
func (m *Model) ratingTag(t library.Track, titleW int) string {
	if !m.marks.on || t.Rating == 0 || titleW < 12 {
		return ""
	}
	return " ★" + string(rune('0'+t.Rating))
}
