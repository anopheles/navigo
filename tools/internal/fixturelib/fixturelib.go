// Package fixturelib defines the synthetic fixture library: made-up artists,
// albums, tracks, playlists and stars. genlibrary turns it into tagged,
// silent audio files and recordfixtures seeds and records a Navidrome
// serving them. Nothing here comes from a real library.
package fixturelib

import "fmt"

// Album is one album directory of the fixture library.
type Album struct {
	Title       string
	AlbumArtist string
	Year        int
	Genre       string
	Compilation bool
	Discs       int
	Tracks      []Track
}

// Track is one audio file. Artist is empty when it is the album artist.
type Track struct {
	Title   string
	Artist  string
	Disc    int
	Number  int
	Seconds int
}

// TrackRef names a track by its album and position.
type TrackRef struct {
	AlbumArtist string
	Album       string
	Disc        int
	Number      int
}

// Playlist is created on the server by recordfixtures.
type Playlist struct {
	Name   string
	Tracks []TrackRef
}

// LongAlbumTracks is more than the 500 rows Navidrome returns per page, so
// paging is exercised.
const LongAlbumTracks = 510

// durations are the only track lengths, so genlibrary encodes few files.
var durations = []int{31, 64, 97, 142, 185, 238}

// Albums is the whole fixture library. It has two albums called "Blue" and
// a "Blue Hour" (search), a compilation with a guest artist (album artist
// differs from track artist) and a track credited to two artists, wide and accented names (layout), and one
// album with more than 500 tracks (paging).
var Albums = []Album{
	album("Aurora Lane", "Blue", 2019, "Dream Pop", "Tidewater", "Blue", "Paper Lanterns", "Slow Orbit", "Harbour Lights", "Blue Again", "Glass Garden", "Last Ferry"),
	album("Aurora Lane", "Blue Hour", 2021, "Dream Pop", "Dusk Signal", "Low Tide", "Streetlamp", "Blue Hour", "Wintering", "Afterglow"),
	album("The Quiet Ones", "Blue", 2015, "Indie", "Undertow", "Quiet Room", "Blue Static", "Radio Silence", "Homeward"),
	album("The Quiet Ones", "Static Bloom", 2018, "Indie", "Static Bloom", "Fieldnotes", "Small Hours", "Copper Wire", "Overpass", "Weathervane", "Coda"),
	album("Zoë Brandt", "Ångström Days", 2020, "Electronic", "Ångström", "Café Nocturne", "Über Alles Still", "Ça Va"),
	album("森の音", "夜明け", 2017, "Ambient", "朝霧", "川の歌", "夜明け前"),
	longAlbum(),
	compilation(),
}

// Playlists are created by recordfixtures. "Morning Mix" holds one track
// twice; "Empty" has no tracks.
var Playlists = []Playlist{
	{Name: "Morning Mix", Tracks: []TrackRef{
		{"Aurora Lane", "Blue", 1, 2},
		{"The Quiet Ones", "Blue", 1, 1},
		{"Zoë Brandt", "Ångström Days", 1, 2},
		{"森の音", "夜明け", 1, 1},
		{"Aurora Lane", "Blue", 1, 2},
	}},
	{Name: "Empty"},
}

// Played are tracks recordfixtures plays once (a scrobble with
// submission), so Home's Most played and Recently played have albums.
var Played = []TrackRef{
	{"Aurora Lane", "Blue Hour", 1, 2},
	{"The Quiet Ones", "Static Bloom", 1, 1},
}

// Starred names what recordfixtures stars. Album and track stars use
// TrackRef; Number 0 means the album itself.
var (
	StarredArtists = []string{"The Quiet Ones"}
	Starred        = []TrackRef{
		{"Aurora Lane", "Blue Hour", 1, 0},
		{"Aurora Lane", "Blue", 1, 6},
		{"Various Artists", "Night Drive Vol. 1", 1, 3},
	}
)

// TrackCount is the number of audio files in the library.
func TrackCount() int {
	n := 0
	for _, a := range Albums {
		n += len(a.Tracks)
	}
	return n
}

func album(artist, title string, year int, genre string, tracks ...string) Album {
	a := Album{Title: title, AlbumArtist: artist, Year: year, Genre: genre, Discs: 1}
	for i, t := range tracks {
		a.Tracks = append(a.Tracks, Track{Title: t, Disc: 1, Number: i + 1, Seconds: durations[(i*7+len(title))%len(durations)]})
	}
	return a
}

// longAlbum is split over two discs so disc numbers are exercised too.
func longAlbum() Album {
	a := Album{Title: "The Long Haul", AlbumArtist: "Long Haul Orchestra", Year: 2022, Genre: "Classical", Discs: 2}
	perDisc := LongAlbumTracks / 2
	for i := range LongAlbumTracks {
		a.Tracks = append(a.Tracks, Track{
			Title:   fmt.Sprintf("Mile %03d", i+1),
			Disc:    i/perDisc + 1,
			Number:  i%perDisc + 1,
			Seconds: 20,
		})
	}
	return a
}

func compilation() Album {
	a := Album{Title: "Night Drive Vol. 1", AlbumArtist: "Various Artists", Year: 2023, Genre: "Synthwave", Compilation: true, Discs: 1}
	for i, t := range []struct{ artist, title string }{
		{"Aurora Lane", "Neon Coast"},
		{"The Quiet Ones", "Exit 14"},
		{"Pell Mell", "Night Drive"},
		{"Pell Mell feat. Zoë Brandt", "Blue Highway"}, // Navidrome splits on " feat. "
	} {
		a.Tracks = append(a.Tracks, Track{Title: t.title, Artist: t.artist, Disc: 1, Number: i + 1, Seconds: durations[i%len(durations)]})
	}
	return a
}
