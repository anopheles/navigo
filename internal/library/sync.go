package library

import (
	"context"
	"fmt"
	"time"

	"github.com/anopheles/navigo/internal/subsonic"
)

// pageSize is Navidrome's cap per request.
const pageSize = 500

// pageTimeout bounds one sync request: a page of a big library can take
// longer than an ordinary call.
const pageTimeout = 60 * time.Second

// page returns a context for one sync request.
func page(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, pageTimeout)
}

// Progress is told how many tracks a full sync has fetched so far and how
// many the server says it has (0 when it doesn't say).
type Progress func(done, total int)

// Due reports whether old needs a full sync: there is none, it is for
// another server or user, or the server has scanned since.
func Due(ctx context.Context, c *subsonic.Client, old *Snapshot, server, user string) (bool, subsonic.ScanStatus, error) {
	st, err := c.ScanStatus(ctx)
	if err != nil {
		return false, st, err
	}
	stale := old == nil || old.Server != server || old.User != user || !st.LastScan.Equal(old.LastScan)
	return stale, st, nil
}

// Sync fetches the whole library. It pages through an empty-query search3,
// which lists everything on Navidrome, and falls back to getAlbumList2 and
// getAlbum on servers where that returns nothing. Playlists and stars are
// fetched too.
func Sync(ctx context.Context, c *subsonic.Client, server, user string, st subsonic.ScanStatus, progress Progress) (*Snapshot, error) {
	if progress == nil {
		progress = func(int, int) {}
	}
	artists, albums, tracks, err := searchAll(ctx, c, st.Count, progress)
	if err == nil && len(tracks) == 0 && st.Count > 0 {
		artists, albums, tracks, err = albumByAlbum(ctx, c, st.Count, progress)
	}
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	playlists, starred, err := extras(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("sync: %w", err)
	}
	return Build(server, user, st.LastScan, artists, albums, tracks, playlists, starred), nil
}

// Refresh fetches what changes without a scan, playlists and stars, onto
// old.
func Refresh(ctx context.Context, c *subsonic.Client, old *Snapshot) (*Snapshot, error) {
	playlists, starred, err := extras(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("refresh: %w", err)
	}
	return old.WithExtras(playlists, starred), nil
}

// searchAll pages each kind until it comes back short. A kind that is
// done is asked for with a count of 0.
func searchAll(ctx context.Context, c *subsonic.Client, total int, progress Progress) ([]Artist, []Album, []Track, error) {
	var artists []Artist
	var albums []Album
	var tracks []Track
	n := subsonic.SearchCounts{ArtistCount: pageSize, AlbumCount: pageSize, SongCount: pageSize}
	for n.ArtistCount+n.AlbumCount+n.SongCount > 0 {
		pctx, cancel := page(ctx)
		res, err := c.Search3(pctx, "", n)
		cancel()
		if err != nil {
			return nil, nil, nil, err
		}
		for _, a := range res.Artists {
			artists = append(artists, ArtistFrom(a))
		}
		for _, a := range res.Albums {
			albums = append(albums, AlbumFrom(a))
		}
		for _, s := range res.Songs {
			tracks = append(tracks, TrackFrom(s))
		}
		progress(len(tracks), total)
		n = next(n, len(res.Artists), len(res.Albums), len(res.Songs))
	}
	return artists, albums, tracks, nil
}

// next moves each kind on by a page, or stops it when its page was short.
func next(n subsonic.SearchCounts, artists, albums, songs int) subsonic.SearchCounts {
	step := func(count, offset, got int) (int, int) {
		if count == 0 || got < count {
			return 0, 0
		}
		return count, offset + got
	}
	n.ArtistCount, n.ArtistOffset = step(n.ArtistCount, n.ArtistOffset, artists)
	n.AlbumCount, n.AlbumOffset = step(n.AlbumCount, n.AlbumOffset, albums)
	n.SongCount, n.SongOffset = step(n.SongCount, n.SongOffset, songs)
	return n
}

// albumByAlbum is the fallback: every album, then each album's songs.
// Artists come from the songs and albums.
func albumByAlbum(ctx context.Context, c *subsonic.Client, total int, progress Progress) ([]Artist, []Album, []Track, error) {
	pctx, cancel := page(ctx)
	list, err := c.Albums(pctx, subsonic.ByName)
	cancel()
	if err != nil {
		return nil, nil, nil, err
	}
	var albums []Album
	var tracks []Track
	var artists []Artist
	seen := map[ArtistID]bool{}
	for _, a := range list {
		pctx, cancel := page(ctx)
		full, err := c.Album(pctx, a.ID)
		cancel()
		if err != nil {
			return nil, nil, nil, err
		}
		albums = append(albums, AlbumFrom(full))
		if id := ArtistID(full.ArtistID); id != "" && !seen[id] {
			seen[id] = true
			artists = append(artists, Artist{ID: id, Name: full.Artist})
		}
		for _, s := range full.Songs {
			tracks = append(tracks, TrackFrom(s))
		}
		progress(len(tracks), total)
	}
	return artists, albums, tracks, nil
}

func extras(ctx context.Context, c *subsonic.Client) ([]Playlist, []string, error) {
	list, err := c.Playlists(ctx)
	if err != nil {
		return nil, nil, err
	}
	playlists := make([]Playlist, 0, len(list))
	for _, p := range list {
		full, err := c.Playlist(ctx, p.ID)
		if err != nil {
			return nil, nil, err
		}
		pl := Playlist{ID: PlaylistID(p.ID), Name: p.Name, Owner: p.Owner}
		for _, s := range full.Songs {
			pl.Tracks = append(pl.Tracks, TrackID(s.ID))
		}
		playlists = append(playlists, pl)
	}
	st, err := c.Starred(ctx)
	if err != nil {
		return nil, nil, err
	}
	var starred []string
	for _, a := range st.Artists {
		starred = append(starred, a.ID)
	}
	for _, a := range st.Albums {
		starred = append(starred, a.ID)
	}
	for _, s := range st.Songs {
		starred = append(starred, s.ID)
	}
	return playlists, starred, nil
}
