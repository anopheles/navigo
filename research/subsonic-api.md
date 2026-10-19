# Subsonic / OpenSubsonic API surface for navigo v1

Ticket: anopheles/navigo#2. Researched 2026-10-19.

## Answer in brief

- **Write a thin client of our own.** It needs about 17 endpoints, JSON only (`f=json`), token+salt auth with a fresh salt on each request, and Go structs that cover the OpenSubsonic fields Navidrome returns. The only maintained Go library is GPL-3.0, has no tagged release, and reuses one salt for every request.
- **Target Subsonic 1.16.1 plus OpenSubsonic.** Send `v=1.16.1`. Call `getOpenSubsonicExtensions` once at login and gate optional features on what it returns.
- **Offline cache:** sync the library by paging `search3` with an empty query. That is the only bulk path Navidrome does not cap. Detect changes with `getIndexes?ifModifiedSince=` or the `lastModified` / `getScanStatus.lastScan` values. Navidrome ignores `ifModifiedSince` on `getArtists`.

## Sources and how claims were checked

| Tag | Source |
|---|---|
| [OS-API] | OpenSubsonic API reference: https://opensubsonic.netlify.app/docs/api-reference/ |
| [OS-EXT] | OpenSubsonic extension index: https://opensubsonic.netlify.app/docs/extensions/ |
| [ND-DOC] | Navidrome Subsonic compatibility page: https://www.navidrome.org/docs/developers/subsonic-api/ |
| [ND-SRC] | Navidrome source, `server/subsonic/*.go` on `master` (fetched 2026-10-19): https://github.com/navidrome/navidrome/tree/master/server/subsonic |
| [LIVE] | Read-only requests against a live Navidrome **0.63.2 (be10f89c)** server: `ping`, `getOpenSubsonicExtensions`, `getArtists`, `getIndexes`, `getArtist`, `getAlbum`, `getAlbumList2`, `search3`, `getPlaylists`, `getPlaylist`, `getStarred2`, `getScanStatus`, `getMusicFolders`, `getUser`, `getPlayQueue`, plus header-only `stream` and `getCoverArt` |
| [GO] | GitHub API, `proxy.golang.org` and the `@v/list` pages for each Go module |

## 1. Request basics

**Auth** [OS-API]:
- **Token:** `u`, `t=md5(password+salt)` as lowercase hex, and `s=salt`. The salt is at least 6 characters and should change on every call. This is the v1 default.
- **API key:** the `apiKey` param replaces `u/p/t/s`. It needs the `apiKeyAuthentication` extension. That extension is in Navidrome `master` [ND-SRC `opensubsonic.go`], but 0.63.2 does not advertise it [LIVE]. Support it later, behind detection.
- **Password:** `u` and `p` (plain, or `enc:`+hex). Since 1.13.0 the spec reserves it for testing. Don't use it.
- On error 41 or 42, retry with a different auth method [OS-API].

**Common params:** `v=1.16.1`, `c=navigo`, `f=json` [OS-API]. Navidrome reports `"version":"1.16.1"` [LIVE].

**Envelope** [OS-API, LIVE]. Every non-binary response looks like this:

```json
{"subsonic-response":{"status":"ok","version":"1.16.1","type":"navidrome",
  "serverVersion":"0.63.2 (be10f89c)","openSubsonic":true, ...payload...}}
```

On failure, `status` is `"failed"` and the response carries `"error":{"code":70,"message":"Album not found"}` [LIVE]. HTTP status is still 200, so the client must check `status`.

**Error codes** [OS-API]: 0 generic, 10 missing param, 20/30 version mismatch, 40 bad credentials, 41 token auth not supported, 42 auth mechanism not supported, 43 conflicting auth, 44 invalid API key, 50 not authorized, 70 not found.

**IDs** are opaque strings (MD5 hashes or UUIDs) [ND-DOC]. Store them as `string`.

**`formPost`** is supported [LIVE]. It allows POST with form-encoded params, which keeps credentials out of URLs and proxy logs [OS-API]. This is optional, but it is a cheap hardening step for every endpoint except `stream` and `getCoverArt`. Those two must stay GET URLs that we hand to mpv or fetch directly.

## 2. Endpoints navigo v1 needs

| Purpose | Endpoint | Key params | JSON payload key and notable fields |
|---|---|---|---|
| Login / health | `ping` | none | none beyond the envelope (`type`, `serverVersion`, `openSubsonic`) |
| Capabilities | `getOpenSubsonicExtensions` | none | `openSubsonicExtensions: [{name, versions[]}]` |
| Artists view | `getArtists` | `musicFolderId?` | `artists: {lastModified, ignoredArticles, index:[{name, artist:[{id,name,coverArt,albumCount,artistImageUrl,sortName,roles[]}]}]}` |
| Artist detail | `getArtist` | `id` | `artist: {id,name,albumCount,coverArt,sortName,roles, album:[AlbumID3]}` |
| Albums view / home shelves | `getAlbumList2` | `type`, `size`(max 500), `offset`, `fromYear/toYear`, `genre` | `albumList2: {album:[AlbumID3]}` |
| Album detail | `getAlbum` | `id` | `album: AlbumID3 + song:[Child]` |
| Playlists | `getPlaylists` | none | `playlists: {playlist:[{id,name,comment,songCount,duration,owner,created,changed,coverArt,readonly}]}` |
| Playlist detail | `getPlaylist` | `id` | `playlist: {…same…, entry:[Child]}` |
| Search | `search3` | `query`, `artistCount/Offset`, `albumCount/Offset`, `songCount/Offset` | `searchResult3: {artist[], album[], song[]}` |
| Playback | `stream` | `id`, `maxBitRate?`, `format?`, `timeOffset?` | binary; URL handed to mpv |
| Cover art (cache only; terminal art is out of scope) | `getCoverArt` | `id`, `size` | binary image |
| Play counts | `scrobble` | `id` (repeatable), `time?`, `submission` | empty |
| Favourites | `star` / `unstar` | `id` / `albumId` / `artistId` | empty |
| Favourites view | `getStarred2` | none | `starred2: {artist[], album[], song[]}` |
| Cache freshness | `getScanStatus` | none | `scanStatus: {scanning,count,folderCount,lastScan,scanType,elapsedTime}` |
| Cache freshness (cheap check) | `getIndexes` | `ifModifiedSince` (ms) | `indexes: {lastModified, ignoredArticles, index?}` |

Nice to have, but not needed for v1: `getPlayQueue` / `savePlayQueue` (resume the queue across devices; Navidrome returns `current` as a string id [ND-DOC]), `getRandomSongs` (max 500), `setRating`, `getGenres`, `getMusicFolders` (multi-library), `getUser` (role flags such as `streamRole` and `downloadRole`).

### AlbumID3 shape (Navidrome 0.63.2) [LIVE]
`id, name, artist, artistId, coverArt, songCount, duration, created, year, userRating, genres[], musicBrainzId, isCompilation, sortName, discTitles[], originalReleaseDate{}, releaseDate{}, releaseTypes[], recordLabels[], moods[], artists[{id,name}], displayArtist, explicitStatus, version`. When the item is starred, a `starred` timestamp and `playCount` / `played` also appear.

### Child (song) shape [LIVE]
`id, parent, isDir, title, album, artist, track, year, coverArt, size, contentType, suffix, duration, bitRate, path, created, albumId, artistId, type, bpm, comment, sortName, mediaType, musicBrainzId, isrc[], genres[], replayGain{}, channelCount, samplingRate, bitDepth, moods[], artists[], displayArtist, albumArtists[], displayAlbumArtist, contributors[], displayComposer, explicitStatus, groupings[], works[], movements[]`. `discNumber`, `starred`, `playCount` and `userRating` appear when they have values.

**Decoding rule:** Navidrome leaves out zero or empty fields (for example `discNumber`, `starred`). Decode leniently with `omitempty`-tolerant structs and don't require any optional field. Use `displayArtist` / `displayAlbumArtist` for UI strings and the `artists[]` arrays for navigation.

### `getAlbumList2` types [ND-SRC `album_lists.go`]
`newest, recent, random, alphabeticalByName, alphabeticalByArtist, frequent, starred, highest, byGenre (genre=), byYear (fromYear=,toYear=)`. Any other type returns error 0. These map directly to the "home" shelves: recently added, recently played, most played, random, starred.

### Playback details [LIVE, ND-DOC]
- A raw `stream` (no transcoding) returns `206` with `Accept-Ranges: bytes`, `Content-Type: audio/mpeg`, `Last-Modified` and `X-Content-Duration: 345.42`, so mpv can seek natively.
- With `format=opus&maxBitRate=128`, the response is `200`, `Accept-Ranges: none`, `audio/ogg`. Seeking then needs `timeOffset` (the `transcodeOffset` extension, which this server advertises) or a re-request.
- `stream` does **not** count a play. Only `scrobble` with `submission=true` does [ND-DOC]. Send `submission=false` when a track starts (it updates "now playing") and `submission=true` once it has played past half its length or 4 minutes. `submission` defaults to true in Navidrome, so always pass it explicitly [ND-SRC `media_annotation.go`].
- The stream URL that mpv receives contains `t`/`s`. Give each URL its own salt. Never log the URL.
- `getCoverArt` returns `Cache-Control: public, max-age=315360000` [LIVE]. The cover IDs are versioned (`al-<id>_<hash>`), so cached art can be kept forever and keyed by ID.

## 3. OpenSubsonic extensions worth using

Navidrome 0.63.2 advertises `transcodeOffset`, `formPost`, `songLyrics` (v1, v2), `indexBasedQueue`, `transcoding` and `playbackReport` [LIVE]. Navidrome `master` adds `topSongsByArtistId`, `apiKeyAuthentication` and `sonicSimilarity` [ND-SRC `opensubsonic.go`].

| Extension | Use in v1? | Why |
|---|---|---|
| `formPost` | **yes** | Keeps tokens out of request URLs and logs. Low effort. |
| `transcodeOffset` | **yes, if transcoding** | Lets us seek in transcoded streams by restarting with `timeOffset`. |
| `apiKeyAuthentication` | later | Stores an API key instead of a password. Detect it at runtime; 0.63.2 lacks it. |
| `playbackReport` | later | Reports richer playback progress. Scrobble covers v1. |
| `indexBasedQueue` | later | Only matters if we add `savePlayQueue` sync. |
| `transcoding` (client-decides transcode) | no | Raw stream plus mpv is simpler. Add `maxBitRate` as a config option. |
| `songLyrics`, `topSongsByArtistId`, `sonicSimilarity` | no | Lyrics are out of scope. The others go beyond parity. |

Also from OpenSubsonic: rely on the extra response fields (`sortName`, `displayArtist`, `artists[]`, `roles`, `mediaType`). They are what make sorting and multi-artist display correct.

## 4. What this means for the offline cache

1. **Bulk listing limits.** Navidrome caps `getAlbumList2` `size`, `getRandomSongs` `size` and `getSongsByGenre` `count` at **500** (`min(..., 500)`) [ND-SRC `album_lists.go`]. A request for `size=600` returned 500 [LIVE]. Page with `offset`.
2. **`search3` with an empty query lists everything, with no cap.** An empty `query` (the default is `""`) matches all items. The counts are passed straight through as `Max` with no clamp [ND-SRC `searching.go`]. On the live server, `songCount=100000` returned all **20,261** songs in one response, and `albumCount=100000` returned all **4,770** albums [LIVE]. Offsets work too. This is the efficient full-sync path:
   - artists: `getArtists` (one call, the whole index)
   - albums: `search3?query=&albumCount=N&albumOffset=…&artistCount=0&songCount=0`
   - songs: the same, with `songCount`
   - playlists: `getPlaylists`, then `getPlaylist` per id

   Page in chunks of a few thousand rather than one huge response. The empty-query behaviour is a Navidrome implementation detail, not something the spec promises. Keep a fallback (`getAlbumList2` paging plus `getAlbum` per album) for other servers.
3. **`ifModifiedSince`:**
   - `getIndexes` honours it. With a future timestamp it returned only `lastModified` and `ignoredArticles`, no `index` [LIVE, ND-SRC `browsing.go`: `if lastScan.After(ifModifiedSince)`].
   - `getArtists` **ignores** it. Navidrome always passes `time.Time{}` [ND-SRC `browsing.go` `GetArtists`], and the live server returned the full index [LIVE].
   - Both responses carry `lastModified` (epoch ms, the last scan time).
4. **Change detection recipe:**
   - Store `lastModified` from `getArtists`/`getIndexes`, or `getScanStatus.lastScan`.
   - On startup or reconnect, call `getScanStatus` or `getIndexes?ifModifiedSince=<stored>`. If nothing changed, serve from the cache. If it did, run a full or incremental resync.
   - For an incremental resync, `getAlbumList2?type=newest` paged until it reaches known IDs picks up added albums. Removals need a full ID sweep.
   - User state (stars, play counts, ratings, playlists) changes without a scan. Refresh it separately: `getStarred2`, plus `getPlaylists` compared on `changed`.
5. **Scale reference:** this library has 3,072 artists, 4,770 albums, 20,261 songs, 44 playlists and 1 music folder [LIVE].

## 5. Go client libraries

| Module | Latest version (proxy.golang.org `@latest`) | Released | License | Notes |
|---|---|---|---|---|
| `github.com/delucks/go-subsonic` | `v0.0.0-20240806025900-2a743ec36238` (pseudo-version). Its git tags (`0.0.6`, tagged 2020-07-20) lack a `v` prefix, so Go modules ignore them | 2024-08-06 | GPL-3.0 | Upstream. No pushes since 2024-08. XML only. No OpenSubsonic fields. |
| `github.com/supersonic-app/go-subsonic` | `v0.0.0-20260807144657-acbe3dde04b7` (pseudo-version). No tags, no GitHub releases | 2026-08-07 | GPL-3.0 | An active fork. JSON support since 2026-03, OpenSubsonic models, `reportPlayback`. |

[GO] Both modules have an empty `@v/list`, meaning there are no semver releases.

**Recommendation: write a thin client, don't depend on either.**
- **License.** Both are GPL-3.0, so navigo would have to be GPL-3.0 too. That is a project decision nobody has made yet.
- **No releases.** We would pin a pseudo-version of an untagged fork.
- **Auth quality.** The fork computes one salt and token in `Authenticate()` and reuses them for every request [GO: `subsonic/client.go`]. The spec says the salt should change per call [OS-API], and we want a fresh salt for each stream URL handed to mpv.
- **Small surface.** The client is about 17 GET/POST calls returning one envelope type. A `Client` with `do(ctx, endpoint, url.Values, out any)` and typed methods comes to a few hundred lines. It also gives us `context.Context` cancellation, which suits a TUI, and lets us shape the structs to the cache schema.
- The fork's `models_opensubsonic.go` is a useful reference for field names, but don't copy code from it because of the GPL.

Suggested package shape: `internal/subsonic` with `Client{BaseURL, User, secret}`, `func (c *Client) StreamURL(id string, opts) string` (fresh salt per call), typed responses `Artist`, `AlbumID3`, `Child`, `Playlist`, and an `Error{Code int; Message string}` that implements `error`, with sentinels for 40, 41/42 and 70.

## Open points for later tickets

- Whether navigo supports non-Navidrome servers in v1. That decides whether the `getAlbumList2`+`getAlbum` fallback sync must ship.
- How to store credentials: a keyring, or an API key once `apiKeyAuthentication` ships in a Navidrome release.
- Whether to transcode by default (`maxBitRate`) or always stream the original file.
