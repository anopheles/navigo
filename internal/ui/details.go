package ui

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"time"

	"github.com/anopheles/navigo/internal/app"
	"github.com/anopheles/navigo/internal/command"
	"github.com/anopheles/navigo/internal/library"
)

func volume(m *Model, c command.Call) error {
	if len(c.Args) != 1 {
		return errors.New("volume: give points, such as volume +5, or a level: volume 60")
	}
	arg := c.Args[0]
	n, err := strconv.Atoi(strings.TrimPrefix(arg, "+"))
	if err != nil {
		return fmt.Errorf("volume: %q is not a number", arg)
	}
	if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-") {
		m.event(app.ChangeVolume{By: n * c.N()})
	} else {
		m.event(app.ChangeVolume{Set: true, To: n})
	}
	return nil
}

// seed is a fresh seed for shuffling.
func (m *Model) seed() uint64 {
	if m.seedFn != nil {
		return m.seedFn()
	}
	return rand.Uint64()
}

// item is an artist, album or track a row shows.
type item struct {
	group library.Group
	id    string
	name  string
}

// rowItem is what row i of a library list shows; false for headings,
// sections and playlists.
func (m *Model) rowItem(f *frame, i int) (item, bool) {
	lib := m.state.Library
	if lib == nil || i < 0 || i >= m.rows(f) {
		return item{}, false
	}
	album := func(a library.Album) (item, bool) { return item{library.AlbumGroup, string(a.ID), a.Name}, true }
	track := func(t library.Track) (item, bool) { return item{library.TrackGroup, string(t.ID), t.Title}, true }
	switch f.kind {
	case artistList:
		a := m.artistsOf(f)[i]
		return item{library.ArtistGroup, string(a.ID), a.Name}, true
	case artistPage:
		if a, ok := m.artistRows(f).album(i); ok {
			return album(a)
		}
	case albumList:
		return album(m.albumsOf(f)[i])
	case homeAlbums:
		return album(m.albumsOf(f)[i])
	case albumTracks:
		return track(m.tracksOf(f)[i])
	case playlistTracks:
		return track(lib.PlaylistTracks(f.playlist)[i])
	case homeTracks:
		return track(m.homeTracksOf(f)[i])
	case searchList:
		v := f.search
		if a, ok := v.artist(i); ok {
			return item{library.ArtistGroup, string(a.ID), a.Name}, true
		}
		if a, ok := v.album(i); ok {
			return album(a)
		}
		if t, ok := v.track(i); ok {
			return track(t)
		}
	}
	return item{}, false
}

// chosenItems are the selection's items, or the row's.
func (m *Model) chosenItems() []item {
	if m.focus == queueWindow {
		from, to := m.queueCur, m.queueCur+1
		if m.visual != nil {
			from, to = m.selection()
		}
		var out []item
		for _, q := range m.state.Queue[from:min(to, len(m.state.Queue))] {
			out = append(out, item{library.TrackGroup, string(q.Track.ID), q.Track.Title})
		}
		return out
	}
	from, to := m.top().cur, m.top().cur+1
	if m.visual != nil {
		from, to = m.selection()
	}
	var out []item
	for i := from; i < to; i++ {
		if it, ok := m.rowItem(m.top(), i); ok {
			out = append(out, it)
		}
	}
	return out
}

// star is *: it stars the chosen items, or unstars them when all are
// starred already.
func (m *Model) star(command.Call) error {
	if f := m.top(); m.focus == libraryWindow && f.kind == searchList && m.visual == nil {
		if _, ok := f.search.playlist(f.cur); ok {
			return errors.New("playlists can't be starred")
		}
	}
	items := m.chosenItems()
	m.endVisual()
	if len(items) == 0 {
		return errors.New("nothing to star here")
	}
	e := app.SetStars{Name: items[0].name}
	if len(items) > 1 {
		e.Name = fmt.Sprintf("%d items", len(items))
	}
	for _, it := range items {
		if !m.state.Library.IsStarred(it.id) {
			e.On = true
		}
		switch it.group {
		case library.ArtistGroup:
			e.Artists = append(e.Artists, it.id)
		case library.AlbumGroup:
			e.Albums = append(e.Albums, it.id)
		default:
			e.Tracks = append(e.Tracks, it.id)
		}
	}
	m.event(e)
	return nil
}

// rate is :rate N: it rates the chosen tracks, or what plays.
func (m *Model) rate(c command.Call) error {
	n, err := rating(c.Args)
	if err != nil {
		return err
	}
	items := m.chosenItems()
	m.endVisual()
	if len(items) == 0 {
		if t, _, ok := m.state.NowPlaying(); ok {
			items = []item{{library.TrackGroup, string(t.ID), t.Title}}
		}
	}
	if len(items) == 0 {
		return errors.New("nothing to rate here")
	}
	e := app.SetRating{Rating: n, Name: items[0].name}
	for _, it := range items {
		if it.group != library.TrackGroup {
			return errors.New("only tracks can be rated")
		}
		e.Tracks = append(e.Tracks, library.TrackID(it.id))
	}
	if len(items) > 1 {
		e.Name = fmt.Sprintf("%d tracks", len(items))
	}
	m.event(e)
	return nil
}

// info is i and :info: details of the row under the cursor, or of the
// track that plays.
func (m *Model) info(command.Call) error {
	lib := m.state.Library
	items := m.chosenItems()
	if len(items) == 0 || m.visual != nil {
		if t, _, ok := m.state.NowPlaying(); ok {
			items = []item{{library.TrackGroup, string(t.ID), t.Title}}
		}
	}
	if len(items) == 0 || lib == nil {
		return errors.New("nothing to show details of here")
	}
	it := items[0]
	var lines []string
	add := func(label, value string) {
		if value != "" {
			lines = append(lines, dimStyle.Render(fmt.Sprintf("%-10s", label))+" "+value)
		}
	}
	yes := func(b bool) string {
		if b {
			return "★ yes"
		}
		return "no"
	}
	switch it.group {
	case library.TrackGroup:
		t, ok := lib.Track(library.TrackID(it.id))
		if !ok {
			t = m.queueTrack(it.id)
		}
		var names []string
		for _, a := range t.Artists {
			names = append(names, a.Name)
		}
		if len(names) == 0 {
			names = []string{t.Artist}
		}
		add("Artists", strings.Join(names, ", "))
		add("Album", t.Album)
		if t.Year > 0 {
			add("Year", strconv.Itoa(t.Year))
		}
		if t.Number > 0 {
			add("Track", fmt.Sprintf("%d on disc %d", t.Number, max(t.Disc, 1)))
		}
		add("Length", clock(t.Duration))
		add("Played", times(t.PlayCount))
		add("Rating", stars(t.Rating))
		if m.state.Recommended(t.ID) {
			add("", "recommended by ListenBrainz")
		}
		if np, _, ok := m.state.NowPlaying(); ok && np.ID == t.ID {
			add("ReplayGain", m.gainText())
		}
	case library.AlbumGroup:
		a, _ := lib.Album(library.AlbumID(it.id))
		add("Artist", a.Artist)
		if a.Year > 0 {
			add("Year", strconv.Itoa(a.Year))
		}
		add("Tracks", fmt.Sprintf("%d · %s", a.TrackCount, long(a.Duration)))
		add("Played", times(a.PlayCount))
	case library.ArtistGroup:
		a, _ := lib.Artist(library.ArtistID(it.id))
		add("Albums", strconv.Itoa(a.AlbumCount))
		add("Appears on", strconv.Itoa(lib.AppearsOn(a.ID)))
	}
	add("Starred", yes(lib.IsStarred(it.id)))
	m.endVisual()
	m.openPopup(it.name, lines)
	return nil
}

// queueTrack finds a track in the queue, for one the library lacks.
func (m *Model) queueTrack(id string) library.Track {
	for _, q := range m.state.Queue {
		if string(q.Track.ID) == id {
			return q.Track
		}
	}
	return library.Track{}
}

// stars is a rating as ★★★★☆, or "not rated".
func stars(n int) string {
	if n <= 0 {
		return "not rated"
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", max(5-n, 0))
}

func times(n int) string {
	if n == 1 {
		return "once"
	}
	return fmt.Sprintf("%d times", n)
}

// long is a duration as "1:02:03" or "42:05".
func long(d time.Duration) string {
	if d >= time.Hour {
		s := int(d.Round(time.Second).Seconds())
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return clock(d)
}
