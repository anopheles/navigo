// Package library holds navigo's view of the server's music: typed IDs and
// the album and track types the core and UI share. The snapshot, cache
// file, sync and search come later.
package library

import (
	"time"

	"github.com/anopheles/navigo/internal/subsonic"
)

// IDs are the server's, typed so one kind can't be passed for another.
type (
	ArtistID   string
	AlbumID    string
	TrackID    string
	PlaylistID string
)

// Album is one album, without its tracks.
type Album struct {
	ID         AlbumID
	Name       string
	Artist     string
	ArtistID   ArtistID
	Year       int
	TrackCount int
	Duration   time.Duration
}

// Track is one track. Artist and album names are copied in for display.
type Track struct {
	ID       TrackID
	Title    string
	Artist   string
	ArtistID ArtistID
	Album    string
	AlbumID  AlbumID
	Disc     int
	Number   int
	Duration time.Duration
}

// AlbumFrom converts the server's album.
func AlbumFrom(a subsonic.Album) Album {
	return Album{
		ID:         AlbumID(a.ID),
		Name:       a.Name,
		Artist:     a.Artist,
		ArtistID:   ArtistID(a.ArtistID),
		Year:       a.Year,
		TrackCount: a.SongCount,
		Duration:   time.Duration(a.Duration) * time.Second,
	}
}

// TrackFrom converts the server's song.
func TrackFrom(s subsonic.Song) Track {
	return Track{
		ID:       TrackID(s.ID),
		Title:    s.Title,
		Artist:   s.Artist,
		ArtistID: ArtistID(s.ArtistID),
		Album:    s.Album,
		AlbumID:  AlbumID(s.AlbumID),
		Disc:     s.DiscNumber,
		Number:   s.Track,
		Duration: time.Duration(s.Duration) * time.Second,
	}
}
