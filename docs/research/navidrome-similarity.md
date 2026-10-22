# What Navidrome returns for similar tracks and artists

Research for [#67](https://github.com/anopheles/navigo/issues/67) (map [#66](https://github.com/anopheles/navigo/issues/66), Mix around a seed).

**Applies to Navidrome v0.64.2** (latest release, 2026-09-24), source at tag `v0.64.2`, commit
[`1011457`](https://github.com/navidrome/navidrome/tree/101145742f4762164202cb4858c9126258bdc463).
This code changes fast: Instant Mix arrived in v0.60, the sonic similarity extension in v0.62, faster matching in v0.63, and `topSongsByArtistId` in v0.64. Check this note again before you rely on it for an older or newer server.

Source links below are permalinks to that commit. `src:` means the `navidrome/navidrome` repo at that commit.

## Summary

- **The server is not empty without Last.fm any more.** The default `Agents` setting is `deezer,lastfm,listenbrainz`, plus a `local` agent that is always added last. Deezer needs no API key and is on by default. ListenBrainz needs no key, is on by default, and only works when files carry MusicBrainz IDs. Last.fm does nothing unless the admin sets both `LastFM.ApiKey` and `LastFM.Secret`.
- **`getSimilarSongs`/`getSimilarSongs2` almost never come back empty.** Navidrome accepts a **track, album, artist, playlist or genre id** on both endpoints, not only the ids the spec lists. If no agent answers, it falls back to library-only data: tracks from the seed's genre, then tracks sampled from the seed, and at the last step the seed tracks themselves. So a non-empty answer does not prove an agent is set up.
- **Every song result is a track in the library.** Agent suggestions go through a matcher (ID, then MBID, then ISRC, then fuzzy title and artist). Anything that matches no track is dropped. Missing files never match, and the user's library permissions apply. Only `getArtistInfo(2)` with `includeNotPresent=true` returns artists the server does not have, each with `id="-1"`.
- **Caching:** similar artists (in `getArtistInfo2`) are stored in the database. The first request fetches them while the client waits. After that the stored copy is served, and once it is older than 24 h a background refresh is queued. `getSimilarSongs` and `getTopSongs` have no stored cache. Each request calls the agents again, behind an in-memory HTTP cache that holds 100 entries for 10 s. An artist seed can take many outbound requests, one after another, so it can take seconds.
- **No API tells a client which agents are set up.** `getOpenSubsonicExtensions` lists `topSongsByArtistId` on every v0.64 server. It lists `sonicSimilarity` only when a plugin provides it, so that one capability can be detected. For everything else, the best probe is `getArtistInfo2` on an artist: a non-empty `similarArtist` list proves that some similar-artist agent works.

## The agent chain

`src:` [`core/agents/agents.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/agents/agents.go)

- `getEnabledAgentNames`: the agents run in the order of `conf.Server.Agents`, and `local` is always added at the end. If `Agents` is empty, **only** `local` runs.
- `callAgent`: tries each agent in turn and **stops at the first one that returns a non-empty list**. This happens *before* library matching. So if Deezer returns 50 top tracks and none of them are in the library, later agents (including `local`) are never asked. Matching then finds nothing.
- An agent that asks the server to back off (a `RetryLaterError`, from throttling or a used-up quota) is skipped for a cooldown, which defaults to 1 min.
- Unknown Artist returns not-found, and Various Artists returns nothing, for similar artists, top songs and similar-by-artist.

Defaults, from `src:` [`conf/configuration.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/conf/configuration.go):
`agents = "deezer,lastfm,listenbrainz"`, `lastfm.enabled = true` with an empty `apikey`/`secret`, `deezer.enabled = true`, `listenbrainz.enabled = true`, `devexternalartistfetchmultiplier = 1.5`.
`EnableExternalServices = false` turns Last.fm, Deezer and ListenBrainz off and sets `Agents = ""`, which leaves only `local` (`disableExternalServices`).

The Navidrome docs ([External services](https://www.navidrome.org/docs/usage/integration/external-services/)) agree: Last.fm "Requires API keys"; Deezer is "Enabled by default, no setup required"; ListenBrainz is "Enabled by default … works best when your files have MusicBrainz IDs"; `local` is "always appended automatically as a final fallback".

### What each agent provides

| Agent | Similar artists | Top songs | Similar songs (per track) | What it needs |
|---|---|---|---|---|
| Deezer ([`adapters/deezer/deezer.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/adapters/deezer/deezer.go)) | yes (related artists, matched by **name** only) | yes (title, album, duration) | **no** | nothing, on by default |
| Last.fm ([`adapters/lastfm/agent.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/adapters/lastfm/agent.go)) | yes (`artist.getSimilar`, name and MBID) | yes (`artist.getTopTracks`) | yes (`track.getSimilar`) | `LastFM.ApiKey` **and** `LastFM.Secret`. Without them the agent is never built (`lastFMConstructor` returns nil) |
| ListenBrainz ([`adapters/listenbrainz/agent.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/adapters/listenbrainz/agent.go)) | yes (Labs `similar-artists`) | yes (`popularity/top-recordings-for-artist`) | yes (Labs `similar-recordings`) | an **artist MBID** for artists, a **recording MBID** for tracks. Without one it returns not-found |
| local ([`core/agents/local_agent.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/agents/local_agent.go)) | **no** | the artist's tracks that are **starred or rated 5**, sorted by play count | **random tracks sharing the seed's genre**, seed left out; nothing if the seed has no genre | always on |
| Spotify | removed in v0.61 ([release notes](https://github.com/navidrome/navidrome/releases/tag/v0.61.0)); `Spotify.ID`/`Secret` are listed as removed options | | | |
| Plugins (`MetadataAgent`) | can implement any of these, plus similar-by-album and similar-by-artist, which no built-in agent has | | | named in `Agents` |

There is no built-in Last.fm key: the default `apikey` is `""`.

## `getSimilarSongs` / `getSimilarSongs2`

Spec ([Subsonic API](http://www.subsonic.org/pages/api.jsp), [OpenSubsonic](https://opensubsonic.netlify.app/docs/endpoints/getsimilarsongs2/)): `id` is required (artist, album or song for v1; artist for v2), and `count` defaults to 50. The spec describes the result as "a random collection of songs from the given artist and similar artists".

Navidrome: [`server/subsonic/browsing.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/server/subsonic/browsing.go) `GetSimilarSongs2` calls `GetSimilarSongs` and only changes the wrapping element, so **both behave the same and accept the same ids**. The logic is in [`core/external/provider_similarsongs.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/external/provider_similarsongs.go) `SimilarSongs`:

- A `count` of 0 or less returns an empty list. Counts above 500 are cut to 500.
- **Track id:** first the chain's `GetSimilarSongsByTrack` (Last.fm, ListenBrainz, then local, in `Agents` order), matched to the library. If that fills fewer than `count`, it tops up with the *similar-artists fallback* (below).
- **Album id:** first `GetSimilarSongsByAlbum` (no built-in agent has it). Then a *seed mix*: 5 random tracks of the album, each run through `GetSimilarSongsByTrack` at the same time, merged, de-duplicated and **shuffled**. If nothing matches, it returns the seed tracks themselves.
- **Artist id:** first `GetSimilarSongsByArtist` (no built-in agent has it). Then the similar-artists fallback. Then a seed mix of 5 random tracks credited to the artist as artist or album artist.
- **Playlist id:** a seed mix of 5 random tracks from the playlist. Smart playlists are evaluated first.
- **Genre id:** a seed mix of 5 random tracks of the genre.
- **Similar-artists fallback** (`similarSongsFallback`): finds the track's or album's artist, fetches up to 15 similar artists through the chain (not stored), then each artist's top songs (at least 20 each, matched to the library), and picks tracks at random, weighted toward the seed artist and higher-ranked songs. **The seed artist's own top songs are among the candidates, so the seed track itself can come back.**
- Order: matched agent results keep the agent's order. Seed-mix results are shuffled.

What a client sees:

| Setup | Track seed | Album / playlist / genre seed | Artist seed |
|---|---|---|---|
| No external agents (`EnableExternalServices=false` or `Agents=""`) | random tracks from the seed's genre. With no genre, only the artist's starred or 5-star tracks, so it can be **empty** | seed tracks run through genre similarity. Worst case **≤5 of the seed's own tracks** | genre similarity from 5 sampled tracks. Worst case ≤5 of the artist's tracks |
| Defaults, no Last.fm key | ListenBrainz if the track has a recording MBID, else local genre tracks, then topped up with Deezer similar artists and their top songs | as above, with ListenBrainz and local per track | Deezer similar artists and top songs, then the seed mix |
| Last.fm key set | Last.fm `track.getSimilar`, matched to the library | Last.fm per seed track | Last.fm or Deezer similar artists and top songs, by `Agents` order |

## `getTopSongs`

Spec: `artist` (name) is required and `count` defaults to 50. The OpenSubsonic [`topSongsByArtistId`](https://opensubsonic.netlify.app/docs/extensions/topsongsbyartistid/) extension adds `id`, which wins over `artist` when both are given. Navidrome advertises it from v0.64 ([`server/subsonic/opensubsonic.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/server/subsonic/opensubsonic.go)).

Navidrome ([`core/external/provider.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/external/provider.go) `TopSongs`/`getMatchingTopSongs`): it finds the artist by id, or by name (case-insensitive LIKE, first hit). It asks the chain for `count × 1.5` songs and matches them to the library, capped at `count`. An unknown artist, not-found or throttling all give an empty `topSongs` with status ok.

- With no agents: the local agent, which returns **only starred or 5-star tracks** by play count. Usually empty.
- With defaults: **Deezer first**. Because the chain stops at the first non-empty answer, an artist Deezer knows gets Deezer's chart even when few of those songs are in the library. Local and ListenBrainz are never asked.
- With Last.fm: only used when it comes before Deezer in `Agents` or Deezer returns nothing.

## `getArtistInfo` / `getArtistInfo2` (similar artists)

Spec: `count` defaults to 20 similar artists and `includeNotPresent` to false. v1 also takes album or song ids.

Navidrome (`browsing.go` `getArtistInfo` → `provider.UpdateArtistInfo`):

- If the artist has no stored info (`ExternalInfoUpdatedAt` is zero), Navidrome calls the agents **while the request waits**: MBID, image, biography, URL and **up to 100 similar artists** (two at a time). It stores them, **including artists not in the library**. If any agent was throttling, the timestamp is not set, so the next request tries again.
- If the stored info is older than `DevArtistInfoTimeToLive` (default **24 h**, `consts.ArtistInfoTimeToLive`), the stored info is served right away and a refresh is queued. The queue holds 2,000 items and works through one every 5 s, with a 15 s timeout each.
- Similar artists come from the first agent that returns any (Deezer, then Last.fm, then ListenBrainz with an MBID). Local has none. They are matched to library artists by ID, then MBID, then name (case-insensitive). Artists in the library come first, in the agent's order. With `includeNotPresent=true` the rest follow with `id="-1"`.
- `lastFmUrl` is the external URL from whichever agent gave one, not always Last.fm. The image URLs always point at Navidrome itself.
- With no agents: `similarArtist` is always empty.

## OpenSubsonic extensions that help

- **`topSongsByArtistId` v1:** use `getTopSongs?id=` instead of guessing by name. Present on every v0.64 server.
- **`sonicSimilarity` v1** ([spec](https://opensubsonic.netlify.app/docs/extensions/sonicsimilarity/)): adds `getSonicSimilarTracks(id, count=10)` and `findSonicPath(startSongId, endSongId, count=25)`. Each returns `sonicMatch` entries (a song plus a `similarity` score) matched to the library. In Navidrome (v0.62+) it is **advertised only when a plugin provides the `SonicSimilarity` capability** ([`core/sonic/sonic.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/sonic/sonic.go) `HasProvider`). Without one, the endpoints return HTTP 404 ([`server/subsonic/sonic_similarity.go`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/server/subsonic/sonic_similarity.go)). This is the only similarity source a client can detect up front, and it works on track seeds only.

## Restricted to the library?

Yes, for every song endpoint. The [`core/matcher`](https://github.com/navidrome/navidrome/blob/101145742f4762164202cb4858c9126258bdc463/core/matcher/doc.go) package matches agent songs by ID, then MusicBrainz recording ID, then ISRC, then fuzzy title and artist (Jaro-Winkler, `Matcher.FuzzyThreshold` default 85). Duration, artist overlap and album break ties. "Missing tracks … are never matched". The media-file and artist repositories apply the requesting user's library filter (`applyLibraryFilter`, `applyLibraryFilterToArtistQuery` in `persistence/`). A fuzzy match can pick a different recording with the same title and artist (a live version or remaster, say). Similar artists not in the library show up only with `includeNotPresent=true`.

## Latency and caching

- `getSimilarSongs`/`getTopSongs`: **nothing stored**. Each request calls the agents live. Each agent's HTTP client caches responses in memory for 10 s (100 entries; `utils/cache/cached_http_client.go`, TTL `DefaultHttpClientTimeOut` = 10 s), and each outbound request times out after 10 s.
- The worst case is an artist seed with Deezer: 1 similar-artists lookup (a search plus related artists), then **up to 16 artists' top songs, fetched one after another**, each a search plus a top-tracks request. That is around 30 outbound requests. A seed mix runs its 5 per-track lookups at the same time.
- With `local` only, everything is SQLite queries and fast.
- `getArtistInfo2`: slow (several outbound requests) the first time for each artist, then a database read.
- No timeout for these handlers turned up, so a client should set its own and treat a timeout as "no server data".

## Detecting an agent up front

- Nothing in the Subsonic or OpenSubsonic API reports the `Agents` setting, API keys or `EnableExternalServices`.
- `getOpenSubsonicExtensions`: `sonicSimilarity` means a sonic plugin is present. `topSongsByArtistId` says nothing about agents.
- The response's `type: "navidrome"` and `serverVersion` identify the server and version, so the behaviour above can be keyed to them.
- Heuristic probe: `getArtistInfo2` on one artist from the library. A non-empty `similarArtist` (try `includeNotPresent=true`) or a `biography` means at least one external agent works. An empty one can also just mean the artist is obscure or the agent was throttled.
- Not a usable probe: a non-empty `getSimilarSongs`. The local genre fallback and the seed fallback fill it with no agent at all, so a client cannot tell agent results from fallback results.

## Notes for the decisions in #66

- ADR 0006's reason ("empty without an agent such as Last.fm") is out of date for v0.60+. On a default install the server returns something for every seed type, often only genre-random tracks or the seed's own tracks. The real risk is now **weak results that look normal**, not emptiness.
- Navidrome's getSimilarSongs already does most of the seeded-mix shapes the map wants (album, playlist and genre seeds), using only the library when there are no agents. That is server-side and not cache-first: it needs the network and can take seconds.
