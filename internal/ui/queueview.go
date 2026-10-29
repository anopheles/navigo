package ui

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

// queueFields are the fields :sort takes in the queue.
var queueFields = []string{"title", "artist", "album", "year", "plays", "length", "genre"}

// shownRows are the queue rows the queue window shows while filtered:
// those that match, and the playing one. key says when to look again.
type shownRows struct {
	key  string
	rows []int
}

// shownQueue is the queue rows the window shows, or nil for all of them.
func (m *Model) shownQueue() []int {
	if !m.queueView.filtered() {
		return nil
	}
	q := m.state.Queue
	_, playing, _ := m.state.NowPlaying()
	key := fmt.Sprintf("%p|%d|%d", q, len(q), playing)
	if m.queueShown.key == key && m.queueShown.rows != nil {
		return m.queueShown.rows
	}
	rows := make([]int, 0, len(q))
	for i, e := range q {
		if i == playing || len(apply(m.queueView, []library.Track{e.Track}, trackFacts)) > 0 {
			rows = append(rows, i)
		}
	}
	m.queueShown = shownRows{key, rows}
	return rows
}

// queueLen is the number of rows the queue window shows.
func (m *Model) queueLen() int {
	if rows := m.shownQueue(); rows != nil {
		return len(rows)
	}
	return len(m.state.Queue)
}

// queueIndex is the queue row that shown row i is.
func (m *Model) queueIndex(i int) int {
	if rows := m.shownQueue(); rows != nil {
		if i >= 0 && i < len(rows) {
			return rows[i]
		}
		return len(m.state.Queue)
	}
	return i
}

// queueEntry is the entry on shown row i.
func (m *Model) queueEntry(i int) (app.QueueEntry, bool) {
	if j := m.queueIndex(i); j >= 0 && j < len(m.state.Queue) {
		return m.state.Queue[j], true
	}
	return app.QueueEntry{}, false
}

// queueEntries are the entries on shown rows from to to.
func (m *Model) queueEntries(from, to int) []app.QueueEntry {
	var out []app.QueueEntry
	for i := from; i < min(to, m.queueLen()); i++ {
		if e, ok := m.queueEntry(i); ok {
			out = append(out, e)
		}
	}
	return out
}

// sortQueue is :sort in the queue window: it reorders the queue itself,
// the current track first, as S does.
func (m *Model) sortQueue(c command.Call) error {
	if len(c.Args) == 0 {
		return errors.New("the queue has no usual order · u undoes the sort")
	}
	field := strings.ToLower(c.Args[0])
	if !slices.Contains(queueFields, field) {
		return fmt.Errorf(
			"sort: %q isn't a field here; try %s",
			c.Args[0],
			strings.Join(queueFields, ", "),
		)
	}
	desc := len(c.Args) > 1 &&
		slices.Contains([]string{"desc", "down", "reverse"}, strings.ToLower(c.Args[1]))
	m.event(app.SortQueue{By: field, Cmp: func(a, b library.Track) int {
		c := compareBy(field, trackFacts(a), trackFacts(b))
		if desc {
			return -c
		}
		return c
	}})
	m.queueCur = 0
	return nil
}

// filterQueue is :filter in the queue window: it hides rows, and what
// plays stays the same.
func (m *Model) filterQueue(c command.Call) error {
	var id string
	if e, ok := m.queueEntry(m.queueCur); ok {
		id = fmt.Sprint(e.ID)
	}
	v := m.queueView
	if err := setFilter(&v, c.Args); err != nil {
		return err
	}
	m.queueView, m.queueShown = v, shownRows{}
	m.queueCur, m.queueOff = 0, 0
	for i := range m.queueLen() {
		if e, _ := m.queueEntry(i); fmt.Sprint(e.ID) == id {
			m.queueCur = i
			break
		}
	}
	if !v.filtered() {
		m.setMsg("filter cleared", false)
		return nil
	}
	m.setMsg(fmt.Sprintf("%d shown · :filter alone shows all", m.queueLen()), false)
	return nil
}

// shownRow is the shown row of queue row j, or the first shown after it.
func (m *Model) shownRow(j int) int {
	rows := m.shownQueue()
	if rows == nil {
		return j
	}
	i, _ := slices.BinarySearch(rows, j)
	return i
}
