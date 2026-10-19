// Package library holds navigo's view of the server's music: typed IDs
// now, and later the snapshot types, the cache file, sync and search.
package library

// IDs are the server's, typed so one kind can't be passed for another.
type (
	ArtistID   string
	AlbumID    string
	TrackID    string
	PlaylistID string
)
