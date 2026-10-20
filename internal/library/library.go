// Package library holds navigo's view of the server's music: typed IDs,
// the domain types the core and UI share, the immutable Snapshot that a
// sync builds, the cache file it is kept in between runs, and search.
package library

import (
	"slices"
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

// Artist is anyone credited on a track or as an album artist.
type Artist struct {
	ID       ArtistID
	Name     string
	SortName string
	// AlbumCount is the number of albums the artist is album artist of.
	AlbumCount int
}

// ArtistRef names one artist credited on a track.
type ArtistRef struct {
	ID   ArtistID
	Name string
}

// Album is one album, without its tracks.
type Album struct {
	ID         AlbumID
	Name       string
	SortName   string
	Artist     string
	ArtistID   ArtistID
	Year       int
	Genre      string
	TrackCount int
	Duration   time.Duration
	PlayCount  int
}

// Track is one track. Artist and album names are copied in for display;
// Artists is every artist credited on it.
type Track struct {
	ID        TrackID
	Title     string
	Artist    string
	ArtistID  ArtistID
	Artists   []ArtistRef
	Album     string
	AlbumID   AlbumID
	Disc      int
	Number    int
	Year      int
	Genre     string
	Duration  time.Duration
	PlayCount int
}

// Credits reports whether a track credits an artist.
func (t Track) Credits(id ArtistID) bool {
	return slices.ContainsFunc(t.Artists, func(a ArtistRef) bool { return a.ID == id })
}

// Playlist is a server playlist; one track can appear in it several times.
type Playlist struct {
	ID     PlaylistID
	Name   string
	Owner  string
	Tracks []TrackID
}

// AlbumFrom converts the server's album.
func AlbumFrom(a subsonic.Album) Album {
	return Album{
		ID:         AlbumID(a.ID),
		Name:       a.Name,
		SortName:   a.SortName,
		Artist:     a.Artist,
		ArtistID:   ArtistID(a.ArtistID),
		Year:       a.Year,
		Genre:      a.Genre,
		TrackCount: a.SongCount,
		Duration:   time.Duration(a.Duration) * time.Second,
		PlayCount:  a.PlayCount,
	}
}

// TrackFrom converts the server's song.
func TrackFrom(s subsonic.Song) Track {
	artists := make([]ArtistRef, 0, len(s.Artists))
	for _, a := range s.Artists {
		artists = append(artists, ArtistRef{ID: ArtistID(a.ID), Name: a.Name})
	}
	if len(artists) == 0 && s.ArtistID != "" { // plain Subsonic: one artist
		artists = append(artists, ArtistRef{ID: ArtistID(s.ArtistID), Name: s.Artist})
	}
	return Track{
		ID:        TrackID(s.ID),
		Title:     s.Title,
		Artist:    s.Artist,
		ArtistID:  ArtistID(s.ArtistID),
		Album:     s.Album,
		AlbumID:   AlbumID(s.AlbumID),
		Disc:      s.DiscNumber,
		Number:    s.Track,
		Year:      s.Year,
		Genre:     s.Genre,
		Duration:  time.Duration(s.Duration) * time.Second,
		PlayCount: s.PlayCount,
		Artists:   artists,
	}
}

// ArtistFrom converts the server's artist.
func ArtistFrom(a subsonic.Artist) Artist {
	return Artist{ID: ArtistID(a.ID), Name: a.Name, SortName: a.SortName}
}
