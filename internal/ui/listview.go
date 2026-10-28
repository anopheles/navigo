package ui

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

// listView is a :sort and :filter on a flat list. The rows it shows are
// kept until what they came from changes (key).
type listView struct {
	sortBy           string
	desc             bool
	genre            string
	yearFrom, yearTo int // 0, 0: no year filter

	key     string
	albums  []library.Album
	artists []library.Artist
	tracks  []library.Track
}

func (v listView) active() bool { return v.sortBy != "" || v.genre != "" || v.yearFrom != 0 }

// filtered is whether a :filter is set, which can leave a list empty.
func (v listView) filtered() bool { return v.genre != "" || v.yearFrom != 0 }

// describe is the view for the title, such as "year ↓ · genre rock".
func (v listView) describe() string {
	var parts []string
	if v.sortBy != "" {
		arrow := "↑"
		if v.desc {
			arrow = "↓"
		}
		parts = append(parts, v.sortBy+" "+arrow)
	}
	if v.genre != "" {
		parts = append(parts, "genre "+v.genre)
	}
	switch {
	case v.yearFrom != 0 && v.yearFrom == v.yearTo:
		parts = append(parts, fmt.Sprintf("year %d", v.yearFrom))
	case v.yearFrom != 0:
		parts = append(parts, fmt.Sprintf("years %d–%d", v.yearFrom, v.yearTo))
	}
	return strings.Join(parts, " · ")
}

// facts are what sorting and filtering look at in an item.
type facts struct {
	name, artist, genre    string
	year, plays, n, number int
	length                 time.Duration
	starred                time.Time
}

func albumFacts(a library.Album) facts {
	return facts{
		name:   a.SortName,
		artist: a.Artist,
		genre:  a.Genre,
		year:   a.Year,
		plays:  a.PlayCount,
		n:      a.TrackCount,
		length: a.Duration,
	}
}

func artistFacts(a library.Artist) facts { return facts{name: a.SortName, n: a.AlbumCount} }

func trackFacts(t library.Track) facts {
	return facts{
		name:   t.Title,
		artist: t.Artist,
		genre:  t.Genre,
		year:   t.Year,
		plays:  t.PlayCount,
		number: t.Disc*10000 + t.Number,
		length: t.Duration,
	}
}

// sortFields are the fields :sort takes on each kind of list, the first
// being what the field is called. Starred tracks adds starred first.
var sortFields = map[listKind][]string{
	albumList:   {"name", "artist", "year", "plays", "tracks", "length", "genre"},
	artistList:  {"name", "albums"},
	albumTracks: {"number", "title", "artist", "year", "plays", "length", "genre"},
	homeTracks:  {"title", "artist", "year", "plays", "length", "genre"},
}

func fieldsFor(f *frame) []string {
	switch {
	case f.kind == homeAlbums:
		return sortFields[albumList]
	case f.kind == homeTracks && f.homeKind == starredTracks:
		return append([]string{"starred"}, sortFields[homeTracks]...)
	}
	return sortFields[f.kind]
}

func compareBy(field string, a, b facts) int {
	switch field {
	case "name", "title":
		return cmp.Compare(strings.ToLower(a.name), strings.ToLower(b.name))
	case "artist":
		return cmp.Compare(strings.ToLower(a.artist), strings.ToLower(b.artist))
	case "genre":
		return cmp.Compare(strings.ToLower(a.genre), strings.ToLower(b.genre))
	case "year":
		return cmp.Compare(a.year, b.year)
	case "plays":
		return cmp.Compare(a.plays, b.plays)
	case "tracks", "albums":
		return cmp.Compare(a.n, b.n)
	case "number":
		return cmp.Compare(a.number, b.number)
	case "length":
		return cmp.Compare(a.length, b.length)
	case "starred":
		return a.starred.Compare(b.starred)
	}
	return 0
}

// apply filters and sorts items as the view says; equal items keep their
// order.
func apply[T any](v listView, items []T, f func(T) facts) []T {
	out := make([]T, 0, len(items))
	for _, it := range items {
		x := f(it)
		if v.genre != "" && !strings.Contains(strings.ToLower(x.genre), strings.ToLower(v.genre)) {
			continue
		}
		if v.yearFrom != 0 && (x.year < v.yearFrom || x.year > v.yearTo) {
			continue
		}
		out = append(out, it)
	}
	if v.sortBy != "" {
		slices.SortStableFunc(out, func(a, b T) int {
			c := compareBy(v.sortBy, f(a), f(b))
			if v.desc {
				return -c
			}
			return c
		})
	}
	return out
}

// viewKey names what a frame's rows come from, so a view knows when to
// sort again: a new snapshot, or a new answer for one of Home's lists.
func (m *Model) viewKey(f *frame) string {
	key := fmt.Sprintf("%p", m.state.Library)
	if f.kind == homeAlbums {
		l := m.state.Home[f.homeKind].Albums
		key += fmt.Sprintf("|%p|%d", l, len(l))
	}
	if f.kind == homeTracks && f.homeKind == playedTracks {
		l := m.state.Played
		key += fmt.Sprintf("|%p|%d", l, len(l))
	}
	if f.kind == homeTracks && f.homeKind != starredTracks && f.homeKind != playedTracks {
		l := m.state.Recs().Tracks
		key += fmt.Sprintf("|%p|%d", l, len(l))
	}
	return key
}

// albumsOf is the albums a list of albums shows, sorted and filtered.
func (m *Model) albumsOf(f *frame) []library.Album {
	var base []library.Album
	switch {
	case f.kind == homeAlbums && f.homeKind == neverPlayed:
		base = m.neverPlayedAlbums(f)
	case f.kind == homeAlbums:
		base = m.homeAlbums(f.homeKind)
	case m.state.Library != nil:
		base = m.state.Library.Albums
	}
	if !f.view.active() {
		return base
	}
	if k := m.viewKey(f); f.view.key != k || f.view.albums == nil {
		f.view.key, f.view.albums = k, apply(f.view, base, albumFacts)
	}
	return f.view.albums
}

// artistsOf is the artists the Artists list shows, sorted.
func (m *Model) artistsOf(f *frame) []library.Artist {
	if m.state.Library == nil {
		return nil
	}
	base := m.state.Library.Artists
	if !f.view.active() {
		return base
	}
	if k := m.viewKey(f); f.view.key != k || f.view.artists == nil {
		f.view.key, f.view.artists = k, apply(f.view, base, artistFacts)
	}
	return f.view.artists
}

// tracksOf is the tracks an album shows, sorted and filtered.
func (m *Model) tracksOf(f *frame) []library.Track {
	if m.state.Library == nil {
		return nil
	}
	base := m.state.Library.AlbumTracks(f.album)
	if !f.view.active() {
		return base
	}
	if k := m.viewKey(f); f.view.key != k || f.view.tracks == nil {
		f.view.key, f.view.tracks = k, apply(f.view, base, trackFacts)
	}
	return f.view.tracks
}

var errNotSortable = errors.New(
	"only Albums, Artists, Home's lists and an album's tracks can be sorted and filtered",
)

// viewFrame is the list :sort and :filter work on.
func (m *Model) viewFrame() (*frame, error) {
	f := m.top()
	if m.focus != libraryWindow || fieldsFor(f) == nil {
		return nil, errNotSortable
	}
	return f, nil
}

// changeView changes the list's view, keeping the cursor on its item.
func (m *Model) changeView(f *frame, change func(*listView)) {
	id := ""
	if f.cur < m.rows(f) {
		id = m.rowID(f, f.cur)
	}
	v := f.view
	change(&v)
	v.key, v.albums, v.artists, v.tracks = "", nil, nil, nil
	f.view = v
	f.cur, f.offset = 0, 0
	for i := range m.rows(f) {
		if m.rowID(f, i) == id {
			f.cur = i
			break
		}
	}
}

// sortList is :sort [field [desc]]; alone it puts the order back.
func (m *Model) sortList(c command.Call) error {
	f, err := m.viewFrame()
	if err != nil {
		return err
	}
	if len(c.Args) == 0 {
		m.changeView(f, func(v *listView) { v.sortBy, v.desc = "", false })
		m.setMsg("back in the usual order", false)
		return nil
	}
	field := strings.ToLower(c.Args[0])
	fields := fieldsFor(f)
	if !slices.Contains(fields, field) {
		return fmt.Errorf(
			"sort: %q isn't a field here; try %s",
			c.Args[0],
			strings.Join(fields, ", "),
		)
	}
	desc := len(c.Args) > 1 &&
		slices.Contains([]string{"desc", "down", "reverse"}, strings.ToLower(c.Args[1]))
	m.changeView(f, func(v *listView) { v.sortBy, v.desc = field, desc })
	m.setMsg("sorted by "+field+" · :sort alone puts the order back", false)
	return nil
}

// filterList is :filter genre <text> or :filter year <year>[-<year>];
// alone it clears the filter.
func (m *Model) filterList(c command.Call) error {
	f, err := m.viewFrame()
	if err != nil {
		return err
	}
	if len(c.Args) == 0 {
		m.changeView(f, func(v *listView) { v.genre, v.yearFrom, v.yearTo = "", 0, 0 })
		m.setMsg("filter cleared", false)
		return nil
	}
	if f.kind == artistList {
		return errors.New("filter: artists have no genre or year; filter Albums instead")
	}
	if len(c.Args) < 2 {
		return errors.New("filter: give genre <text> or year <year>, such as filter year 1990-1999")
	}
	switch strings.ToLower(c.Args[0]) {
	case "genre":
		g := strings.Join(c.Args[1:], " ")
		m.changeView(f, func(v *listView) { v.genre = g })
	case "year":
		from, to, err := years(c.Args[1])
		if err != nil {
			return err
		}
		m.changeView(f, func(v *listView) { v.yearFrom, v.yearTo = from, to })
	default:
		return fmt.Errorf("filter: %q isn't a filter; use genre or year", c.Args[0])
	}
	m.setMsg(fmt.Sprintf("%d shown · :filter alone shows all", m.rows(f)), false)
	return nil
}

// years reads "1994" or "1990-1999".
func years(s string) (int, int, error) {
	a, b, ranged := strings.Cut(s, "-")
	from, err1 := strconv.Atoi(a)
	to, err2 := from, error(nil)
	if ranged {
		to, err2 = strconv.Atoi(b)
	}
	if err1 != nil || err2 != nil || from <= 0 || to < from {
		return 0, 0, fmt.Errorf("filter: %q isn't a year or a range like 1990-1999", s)
	}
	return from, to, nil
}

// viewWords are what :sort and :filter complete.
func (m *Model) sortWords() []string { return fieldsFor(m.top()) }

func (m *Model) filterWords() []string { return []string{"genre ", "year "} }
