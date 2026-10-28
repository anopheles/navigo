# navigo v1 specification

navigo is a fast, keyboard-driven terminal client for Navidrome and other Subsonic/OpenSubsonic servers, written in Go. It runs on Linux, macOS and Windows; Linux is the primary target for end-to-end testing.

**Main goal: easy to learn.** A new user must be able to search a track, jump to its album, select tracks, queue and play them without reading any docs. Keys are vim all the way, and every one can be rebound.

This spec collects the decisions made on the planning map, [anopheles/navigo#1](https://github.com/anopheles/navigo/issues/1). Each section states what v1 does and links to the ticket holding the reasoning and the alternatives. The words navigo uses are defined in [`GLOSSARY.md`](../GLOSSARY.md), and the hard-to-reverse choices are recorded as ADRs in [`docs/adr/`](adr/). The [last section](#the-walking-skeleton-and-this-spec) lists what the walking skeleton in this repo already does and what is still to build.

## Contents

1. [Scope](#scope)
2. [Architecture](#architecture)
3. [Domain model](#domain-model)
4. [Server API](#server-api)
5. [Login and credentials](#login-and-credentials)
6. [Configuration](#configuration)
7. [Screens and navigation](#screens-and-navigation)
8. [Keys](#keys)
9. [Discoverability](#discoverability)
10. [Search](#search)
11. [Queue and playback](#queue-and-playback)
12. [Offline cache and sync](#offline-cache-and-sync)
13. [ListenBrainz Recommendations](#listenbrainz-recommendations)
14. [Resume](#resume)
15. [Offline, errors and messages](#offline-errors-and-messages)
16. [OS media controls](#os-media-controls)
17. [Colours](#colours)
18. [Cover art](#cover-art)
19. [Testing](#testing)
20. [Licence and dependencies](#licence-and-dependencies)
21. [Filled in while writing this spec](#filled-in-while-writing-this-spec)
22. [The walking skeleton and this spec](#the-walking-skeleton-and-this-spec)

## Scope

**In v1:**
- Browsing: Home, Artists, Albums, Playlists, Search and Help.
- Playback through mpv, with a queue edited like a vim buffer.
- Playlist editing on the server, and starring.
- OS media controls.
- An offline library cache.
- Login with the OS keyring.
- Full key rebinding in YAML.
- **Added after the first release** ([#48](https://github.com/anopheles/navigo/issues/48)): ReplayGain, Resume (the queue comes back at the next start) and colour roles.
- **Recommendations** ([#47](https://github.com/anopheles/navigo/issues/47)): a Mix leans toward the user's ListenBrainz Recommendations once `listenbrainz.user` is set.

**Out of scope:**
- **Beyond-parity features:** lyrics, scrobbling settings and multiple servers. Cover art is built ([#60](https://github.com/anopheles/navigo/issues/60)); artist and playlist images are not.
- **ListenBrainz beyond Recommendations** ([#47](https://github.com/anopheles/navigo/issues/47)): scrobbling to it (Navidrome does), its generated playlists, and Recommendations not in the library.
- **Offline playback** (downloading audio) ([#9](https://github.com/anopheles/navigo/issues/9)).
- **Release packaging** (goreleaser, package managers).
- **A single accent colour:** one colour driving several roles; roles are recoloured one by one or by a named theme ([#52](https://github.com/anopheles/navigo/issues/52)).
- **Free-text filtering within a list:** `/` jumps between matches, and the Search section covers the whole library ([#26](https://github.com/anopheles/navigo/issues/26)).

## Architecture

Decided in [What is navigo's architecture and domain model?](https://github.com/anopheles/navigo/issues/10), with ADRs [0001](adr/0001-single-writer-core-with-effects-as-data.md)–[0004](adr/0004-shuffle-reorders-the-queue.md); [0006](adr/0006-mix-is-picked-from-the-cache.md), [0007](adr/0007-a-mix-around-a-seed-asks-the-server-for-similar-artists.md) and [0009](adr/0009-the-saved-queue-lives-on-the-server.md) came after v1.

**State flow:**
- A **UI-free core** (`internal/app`) holds the session state: library snapshot, queue, register, undo history, player status and offline state.
- `Apply(state, event)` returns the new state and **effects as plain data**, such as `Play`, `SetNeighbours`, `FetchSync`, `Scrobble`, `PublishNowPlaying` and `Notice`.
- The Bubble Tea model is the only caller of `Apply`. A runner executes the effects as commands and sends their results back as events.
- Every input arrives as a message on that one path: key presses, mpv events, MPRIS calls, sync results. Cursors, section paths and the jump list stay in the UI.
- Player effects run strictly in the order the core issued them, even though Bubble Tea runs commands concurrently.

**Package layout:**

```
cmd/navigo/          wiring only
internal/app/        core: session state, queue, register, effects
internal/library/    domain types, snapshot, cache file, sync, search
internal/subsonic/   thin HTTP client (+ subsonictest: the fake server)
internal/listenbrainz/ Recommendations client (+ a fake ListenBrainz)
internal/mpv/        IPC client + process spawn
internal/player/     Player interface, mpv adapter, in-memory fake
internal/mediactl/   MediaControls: MPRIS on Linux, stubs elsewhere
internal/keymap/     engine, notation, binding table
internal/command/    command-line parser + registry
internal/config/     config.yaml, per-OS paths
internal/creds/      password sources and the keyring
internal/ui/         Bubble Tea model, windows, lists, bars, login form
```

**Player:** this hides mpv completely (process, IPC, playlist, entry IDs). Its interface:
- **Calls:** `Play(Span{Prev, Current, Next})`, `SetNeighbours(prev, next)`, `Pause`/`Resume`, `SeekTo`/`SeekBy`, `SetVolume`, `SetReplayGain(mode, preamp)`, `Stop`.
- **Events:** `TrackStarted`, `TrackEnded{Finished|Stopped|Failed}`, `Position`, `Paused`, `Buffering`, `Volume`, `Died`.
- **Adapters:** two, the mpv one and an in-memory fake.

**mpv** ([#4](https://github.com/anopheles/navigo/issues/4), [ADR 0002](adr/0002-navigo-owns-the-queue.md)):
- **Spawn:** navigo starts its own mpv with `--idle=yes --no-terminal --no-video --audio-display=no --no-config --load-scripts=no --ytdl=no`, `--replaygain=<mode>` and `--replaygain-preamp=<dB>`, then `player.mpv_args`, then `--input-ipc-server`.
- **IPC:** a unix socket in a 0700 temp directory, or a unique named pipe on Windows (go-winio). No cgo.
- **Playlist:** mpv holds only the previous, current and next queue entries, so playback is gapless and mpv's own Previous works. navigo owns the queue.
- **Stream URLs:** signed with a fresh salt, sent over IPC only, never on the command line. The minimum mpv is 0.38.

**Logging:** `log/slog` to `navigo.log` in the user cache directory, at warn level, or debug with `--debug` / `NAVIGO_DEBUG=1`. Tokens and stream URLs are never logged, and nothing goes to the terminal while the TUI runs.

**Build:** module `github.com/anopheles/navigo`, Go 1.26, `CGO_ENABLED=0` for linux, darwin and windows on amd64 and arm64.

## Domain model

[#10](https://github.com/anopheles/navigo/issues/10), [#21](https://github.com/anopheles/navigo/issues/21). Terms: **Library**, **Artist**, **Album artist**, **Appears on**, **Queue**, **Queue entry**, **Group** in the glossary.

- IDs are typed strings: `ArtistID`, `AlbumID`, `TrackID`, `PlaylistID`.
- A **Track** carries its artist and album names for display, **each artist the server credits it with** (OpenSubsonic's per-track list), disc and track numbers, and its duration.
- The **Library** is an immutable snapshot (maps by ID plus ordered slices) that a sync replaces in one step. Artists carry the server's sort name.
- **Artists** are everyone credited on a track or as an album artist, Various Artists included. Composers, conductors and other roles are left out.
- The queue holds **queue entries** (`EntryID` + track), so one track can be queued twice and edits and undo stay unambiguous.
- **Shuffle** is a one-shot, undoable reorder of the queue ([ADR 0004](adr/0004-shuffle-reorders-the-queue.md)). **Repeat** is a mode: off, all or one.
- A **Mix** is a one-shot, undoable replacement of the queue with tracks picked from the cache, from the whole library or around a **Seed** ([ADR 0006](adr/0006-mix-is-picked-from-the-cache.md)). Around a seed, the server's **similar artists** count too when it can be reached ([ADR 0007](adr/0007-a-mix-around-a-seed-asks-the-server-for-similar-artists.md)). It is not a mode: the picked tracks are ordinary queue entries. When set up, a Mix leans toward the user's **Recommendations** from ListenBrainz ([ADR 0008](adr/0008-listenbrainz-is-a-second-optional-service.md)).

## Server API

[Which Subsonic/OpenSubsonic API surface does navigo v1 need?](https://github.com/anopheles/navigo/issues/2), plus timeouts from [#23](https://github.com/anopheles/navigo/issues/23).

- **Client:** navigo's own thin JSON client (`f=json`, `v=1.16.1`). Token+salt auth with a **fresh salt per request** and per stream URL, and never `p=`.
- **Endpoints:**
  - session and library: `ping`, `getOpenSubsonicExtensions`, `getArtists`, `getArtist`, `getArtistInfo2` (similar artists for a Mix around a seed: `count=20`, without `includeNotPresent`, so only library artists come back), `getAlbumList2`, `getAlbum`, `getPlaylists`, `getPlaylist`, `search3`;
  - playback: `stream`, `getCoverArt`, `scrobble`;
  - the saved queue: `getPlayQueue`, `savePlayQueue`, or `getPlayQueueByIndex`, `savePlayQueueByIndex` when the server offers `indexBasedQueue` ([Resume](#resume));
  - stars, ratings and change detection: `star`/`unstar`, `setRating` (one call per track), `getStarred2`, `getScanStatus`, `getIndexes`;
  - playlist edits: `createPlaylist` (also with `playlistId`, to reorder: it replaces the tracks), `updatePlaylist` (also with `name`, to rename), `deletePlaylist`.
- **Extensions:** `formPost` (calls go as POST once offered, keeping tokens out of URLs), `indexBasedQueue` (the saved queue's current entry is an index, not a track ID) and `transcodeOffset` (seeking in transcoded streams). Optional features are gated on `getOpenSubsonicExtensions`.
- **Limits:** Navidrome caps list endpoints at 500. An empty-query `search3` lists the whole library with offsets; that's the sync path, with a `getAlbumList2`+`getAlbum` fallback.
- **Scrobbles:** `stream` doesn't count a play. navigo sends `scrobble submission=false` when a track starts and `submission=true` after half the track or 4 minutes listened, whichever comes first, once per queue-entry play. Seeks don't count toward that.
- **Timeouts:** 10 s for API calls, 60 s per sync page, 2 s for a Mix's similar-artist lookups, 3 s for the saved queue ([Resume](#resume)).
- **Errors:** a failed response is `subsonic-response.status="failed"` with a code: 40 wrong credentials, 41 token auth unsupported (clear message, no fallback to a plain password), 70 not found.

## Login and credentials

[How does navigo log in and store server credentials?](https://github.com/anopheles/navigo/issues/15)

**Login form:**
- It's full-screen, and opens on first run, when the saved credentials are rejected, and on `:login`.
- It asks for server URL, username and password, checks them with `ping`, shows errors in place, and saves only on success.
- A URL without a scheme tries `https://`, then `http://`.

**Password sources**, in order:
1. `server.password_command`: its first line of output;
2. `server.password` in the config;
3. the OS keyring (go-keyring), under service `navigo` and key `user@url`.

The form saves to the keyring. If the keyring isn't available, it offers (y/n) to write the password into `config.yaml` instead.

**At runtime** the password is kept in memory only and a fresh token is made for each request. No token is ever written to disk.

**Rules:**
- `:logout` deletes the keyring entry and the stored [Recommendations](#listenbrainz-recommendations), and returns to the form. Logging in as another user or server starts a fresh session.
- If `config.yaml` holds a password and group or others can read it, navigo warns but still runs (Unix only). navigo creates the file with mode 0600.
- No API-key login in v1 (Navidrome 0.63 doesn't offer it). One `server:` block only.

## Configuration

One YAML file, `navigo/config.yaml` under the user config directory (`$XDG_CONFIG_HOME` or `~/.config` on Linux, `~/Library/Application Support` on macOS, `%AppData%` on Windows). `--config` points elsewhere. Every key is optional.

```yaml
server:
  url: https://music.example.com
  user: ann
  password_command: pass show navidrome   # or password: …, or nothing (keyring)

listenbrainz:        # Recommendations for a Mix; off without user
  user: ann
  url: https://api.listenbrainz.org       # the default; or a self-hosted ListenBrainz

player:
  mpv_path: /opt/mpv/bin/mpv               # when mpv isn't on PATH
  mpv_args: [--audio-device=pulse/hdmi]    # extra mpv options; must start with --
  replaygain: track                        # off | track | album
  replaygain_preamp: 0                     # dB, -15 to 15

hints: true          # the hint line; :hints on|off for the session
powerline: false     # Nerd Font arrows between the now-playing bar's blocks
resume: true         # bring the queue back at the next start (false = off)
sync_queue: false    # also keep the queue on the server, so other devices start from it

cover_art: true      # the playing album's cover: true, false, blocks or kitty

theme: catppuccin-mocha   # a named palette for the roles (optional)
colors:              # recolour a role on top of it; '#rrggbb' or an ANSI name or 0-15
  playing: green     # or cover: the colour of the playing album's cover
  key: yellow
  error: red
  selection: gray

keys:
  leader: '\'        # expanded at load
  timeout: 1000      # ms before an ambiguous prefix runs its shorter binding
  hints: true        # prefix menus after a pure prefix
  hint-delay: 500    # ms before a prefix menu appears
  defaults: true     # false = start every mode and scope empty
  normal:
    global:
      '<C-d>': half-page-down
      '<leader>x': 'seek +60'
    queue:
      'D': queue-clear
      'J': ~           # delete: fall through to the wider scope
    albums:
      'd': nop         # dead key
  visual:
    global: {}
  cmdline:             # Command-line, Search and Prompt share it
    '<C-k>': kill-to-end
```

**The `keys:` block** ([How is rebinding expressed in the YAML config?](https://github.com/anopheles/navigo/issues/7)):
- It's nested as mode → scope → key sequence → command line, with vim key notation.
- `~` deletes a binding, `nop` makes a dead key, and `defaults: false` wipes all defaults.
- A binding in `normal` whose command is a motion also applies in Visual mode and after an operator.
- **Validation:** a YAML syntax error is fatal. A bad binding is skipped, and all problems are listed in a popup as `config.yaml:LINE:COL: message`. navigo refuses to start if the final table offers no way to quit, and `navigo check-config` validates and exits non-zero.
- **Session changes:** `:nmap`/`:vmap`/`:cmap` (with `--in <scope>`) and the `unmap` forms last for the session only. `:reload-config` rebuilds the table and discards them.
- Only the y/n keys of a Confirmation can't be rebound.

**The new keys** ([#53](https://github.com/anopheles/navigo/issues/53)):
- `player.replaygain` is `off`, `track` (the default) or `album`; `player.replaygain_preamp` is in dB, from -15 to 15, default 0. [Queue and playback](#queue-and-playback).
- `resume` is on unless set to `false`. [Resume](#resume).
- `theme` names a palette for the roles; `colors` then recolours single roles on top of it. [Colours](#colours).
- `colors` takes the roles `playing`, `key`, `error` and `selection`. [Colours](#colours).
- **Validation:** a bad value (an unknown mode, a preamp that isn't a number or is out of range, a colour that can't be read) is skipped, keeps its default and is listed in the same popup as bad bindings, as `config.yaml:LINE:COL: message`. Unknown top-level keys are still ignored, but an unknown role inside `colors` is reported. `navigo check-config` covers all of it.
- **`:reload-config`** applies all three again: ReplayGain returns to the configured mode and preamp (dropping a `:replaygain` override), colours change at once, and `resume: false` stops saving, deletes this device's saved queue and stops reading and writing the server's, which stays as it is.

**`listenbrainz`** ([How does the user turn it on?](https://github.com/anopheles/navigo/issues/82)):
- `user` turns [Recommendations](#listenbrainz-recommendations) on; without it they're off. There's no `enabled` key and no `:` command that sets it. `url` defaults to `https://api.listenbrainz.org`.
- **Validation:** an empty `user` or a `url` that isn't `http(s)` is skipped, listed in the same popup, and leaves Recommendations off. Whether the user exists is only found out by the first fetch; `navigo check-config` stays offline.
- **`:reload-config`:** turning them on, or changing `user` or `url`, fetches at once (past the 24 h wait); turning them off stops a Mix leaning toward them at once and deletes the stored list.

## Screens and navigation

[What screens, layout and navigation model does navigo have?](https://github.com/anopheles/navigo/issues/8), [#13](https://github.com/anopheles/navigo/issues/13), [#21](https://github.com/anopheles/navigo/issues/21).

**Layout** (top to bottom):
- **Two windows** side by side. The **library window** (3/5 of the width) shows one list at a time, drilled into with `l`/`<CR>` and out with `h`. The **queue window** (2/5) always shows the queue; its title gives the count and total length, `Queue (12) · 48:10` ([#61](https://github.com/anopheles/navigo/issues/61)).
- **The hint line.**
- **The progress rule**: a faint rule under the windows that fills as the track plays.
- **The now-playing bar** (1 line, never focused), laid out like a Neovim status line: the mode (`NORMAL`, `VISUAL`, `COMMAND`, `SEARCH`) on a block of colour, ■/▶/⏸, then title — artist · album; on the right the indicators and the repeat and volume flags, the position in the queue (`2/6`, or `6 in queue` with nothing playing) and elapsed/total time. `powerline: true` joins its blocks with a Nerd Font's arrows.
- **The status line**, which shows messages, the `:` line and pending keys.

**Windows:**
- **Switching:** `<C-w>h`/`<C-w>l`/`<C-w>w`, with `<Tab>` as another key for `<C-w>w`.
- **Closing the queue window:** `<C-w>o` closes it until `:queue` or `'q`.
- **Narrow terminals:** below 100 columns only one window is shown at a time, and `<C-w>w`/`<Tab>` swaps them.
- Window widths are fixed.

**Popups** (a Confirmation, track info, the key menu, `:messages`) appear over both windows. They take the `popup` scope, and `q`/`<Esc>` close them.

**Sections:** Home, Artists, Albums, Playlists, Search, Help. There is no Tracks section. Each section remembers its drill-down path and cursor.
- **Home:** Mix, which runs a whole-library Mix (`:mix library`) rather than opening a list, then the album lists in this order: Recently added, Most played, Recently played, Random albums, Starred; then, while [Recommendations](#listenbrainz-recommendations) are on, Recommended.
- **Artists:** every artist with at least one track, sorted by the server's sort name. Each row shows the artist's own album count, or "appears on N" for artists with no albums of their own.
- **An artist's page:** their own albums, then a `── Appears on ──` heading (skipped by the cursor) and the albums where they only have tracks. `{`/`}` jump between the two groups. Opening an Appears on album puts the cursor on that artist's first track.
- **Marks in library lists** ([#93](https://github.com/anopheles/navigo/issues/93)): a track already in the queue shows `•` in its row's gutter (after the cursor's `▎`), and an album with any queued track shows it before its name. A starred row keeps its `★`, and a rated track shows `★3` after its title. Artists and the queue window carry no marks, and rows under 40 cells keep the `•` and drop the rating. Every mark is a glyph, so none needs colour.
- **Help:** generated from the bindings in effect, and searchable with `/`. It describes `gm`'s chain (row, what plays, whole library) once.

**Marks** jump to a section: `'h` Home, `'q` the queue window, `'a` Artists, `'b` Albums, `'p` Playlists, `'s` Search, `'?` Help. They are ordinary bindings to `:home`, `:artists`, ….

**Jump list** (`<C-o>`/`<C-i>`):
- **Recorded:** marks, section commands, `ga`/`gr`/`gp`, `gg`/`G`, and `/` with `n`/`N`.
- **Not recorded:** `j`/`k`, `<C-d>`/`<C-u>` and `l`/`h`.
- `<C-i>` works where the terminal tells it apart from Tab, and `:forward` works everywhere.

**Startup:** opens on Home, focus in the library window, with the queue empty or, when [Resume](#resume) has one saved, restored and paused. The mouse is limited to four things: a click or drag on the progress rule seeks there; the wheel over the rule or the now-playing bar changes the volume by 5 points a notch; a click on a list row moves the cursor there (and the focus to its window), and a second click on it within 400 ms does what `<CR>` does; and the wheel over a list scrolls it 3 rows a notch.

## Keys

[What is navigo's vim keymap grammar and default key table?](https://github.com/anopheles/navigo/issues/6), amended by [#13](https://github.com/anopheles/navigo/issues/13).

**Modes:**
- **Normal.**
- **Visual:** `V`/`v`, whole rows. `:` applies a command to the selection.
- **Command-line** (`:`), **Search** (`/`) and **Prompt** (text asked for by a popup). These three share one line-editing table: `<C-w>`, `<C-u>`, `<C-a>`/`<C-e>`, history on `<Up>`/`<Down>` and `<C-p>`/`<C-n>`, `<Esc>` cancels and `<CR>` accepts.

**Grammar:** `[count] operator [count] motion | [count] operator operator | [count] command`.
- **Operators:** `d` (remove) and `y` (yank) only. There is one unnamed register shared by every view. `"x` is parsed and rejected.
- **Not in v1:** text objects, macros, dot-repeat.
- **Counts:** a count multiplies motions and scalable commands (`3.` seeks 30 s; `3a` adds three times) and is ignored elsewhere.
- **Pure prefixes** wait with no time limit. **Ambiguous prefixes** run their shorter binding after `keys.timeout`. Unknown sequences are dropped, and pending keys show at the right of the status line.

**Commands:** every key runs a **command** typeable at `:` (`:play-pause`, `:seek +10`). A binding is a key sequence → command line, never keys → keys. Each command has a name, a short label (hint line), a description (key menu and help), and its motion/operator flags.

**Scopes**, narrow to wide; the narrowest match wins:
- `popup`, above everything else.
- `sections`, `home`, `artists`, `albums`, `playlists` → `containers` → `global`.
- `album-tracks`, `playlist-tracks` → `tracks` → `global`.
- `queue` → `global`.
- `help` → `global`.
- `search` on top of each search row kind's chain.

**Default table:**

| Area | Keys |
|---|---|
| Motions | `j` `k` `<Down>` `<Up>` · `gg` `G` `[count]G` · `<C-d>` `<C-u>` `<C-f>` `<C-b>` · `<C-e>` `<C-y>` · `H` `M` `L` · `zz` `zt` `zb` · `{` `}` (groups) · `[count]%` · scroll wheel |
| Navigation | `l` `<CR>` into · `h` out · `ga` album · `gr` artist · `gp` playing entry in the queue · `<C-o>` `<C-i>` jump list · marks `'h` `'q` `'a` `'b` `'p` `'s` `'?` |
| Find | `/` forward (backward: `:find-back`, no key) · `n` `N` · `:noh` / `<Esc>` clear highlights |
| Playback | `<Space>` play/pause · `>` `<` next/previous · `.` `,` seek ±10 s · `=` `+` `-` volume ±5 · `S` shuffle · `gm` mix around the row under the cursor (`[count]gm` for that many tracks) · `R` repeat off → all → one · `X` stop |
| Add and play | `<CR>` play now · `a` play next · `A` add to end · `i` info · `*` star/unstar · `gc` the playing cover, large · `yy` `y{motion}` yank |
| Queue | `dd` `x` `d{motion}` remove · `yy` · `p` `P` paste after/before · `J` `K` move · `u` `<C-r>` undo/redo (from either window) |
| Playlist tracks | as the queue, but edits go to the server: every removal asks y/n first; `u`/`<C-r>` explain that `P` puts the last removal back |
| Playlist list | `dd` deletes the playlist after y/n |
| Windows | `<C-w>h` `<C-w>l` `<C-w>w` `<Tab>` · `<C-w>o` |
| Help and quit | `?` key menu · `<F1>` `:help` · `<Esc>` cancel · `q` closes a popup only · `:q` `:qa` `<C-c>` `ZZ` `ZQ` quit |

`s` and `r` are deliberately unbound, `\` is the leader and has no default bindings, and `<C-Space>` is never a default (because of a Windows console bug).

**`:`-only commands:**
- library and playlists: `:sort <field> [desc]` and `:filter genre <text>|year <year>[-<year>]` (each alone undoes it) on Albums, Artists, Home's lists and an album's tracks, `:playlist-add <name>`, `:playlist-new`, `:playlist-rename <name>` (the playlist under the cursor or the one open; a name another playlist has is refused, [#63](https://github.com/anopheles/navigo/issues/63)), `:sync`;
- session: `:login`, `:logout`, `:reconnect`, `:hints on|off`;
- playback: `:mix library` mixes from the whole library whatever is under the cursor (Tab completes `library`; Home's Mix row runs it, and binding `gm` to it gives the old `gm`); `:replaygain [off|track|album]` changes the ReplayGain mode for the session (no argument cycles through them) and has no default key;
- rating ([#64](https://github.com/anopheles/navigo/issues/64)): `:rate 1`–`5` rates the track under the cursor, the selection's tracks, or with nothing under the cursor what plays; `:rate 0` clears it. Albums and artists can't be rated, a rating is refused offline like a star, and the cache takes it once the server agrees, so Mix sees it at once. The `i` popup shows a track's rating as `★★★★☆` or `not rated`;
- the rest: `:messages`, `:info`, `:map`, `:reload-config`.

The `:` line has Tab completion for command names and arguments.

## Discoverability

[How does a new user discover navigo without reading docs?](https://github.com/anopheles/navigo/issues/13). Every aid is drawn from the live binding table, so they all follow rebinding.

- **Hint line:** the keys that matter for the row under the cursor and the mode, such as `<CR> play  A add to queue  a play next  V select  ga album  <C-o> go back  's search  <Tab> other window  '? help`. In Visual mode it shows counts ("A add 3 tracks to queue"). It's on by default. `gm mix` shows only on rows that can be a seed.
- **Empty-queue message:** the empty queue window says what to press first, including `Or let navigo pick:  gm mix from the whole library`.
- **Prefix menus:** these appear after a pure prefix (`g`, `'`, `<C-w>`, `d`, leader) has waited 500 ms.
- **Key menu (`?`):** a popup with the keys for what's under the cursor, grouped as Play, Queue, Select, Go and Help. Pressing a key runs it. The Play group names what `gm` would mix from: `mix from <name>`, or on a row that can't be a seed `mix from <what plays>` or `mix from the whole library`. It scrolls to fit 24 rows.
- **The `:` line teaches keys:** typing fuzzy-matches command names and descriptions, listing each match's key. Matches that apply here come first, then whole words, then scattered letters. Running a command that has a key shows `tip: next time press A`.
- **Every queue change says what it did** and how to undo it: "3 tracks added to the end of the queue (12 in queue) · u undoes".
- **A `/` that finds nothing** offers `<CR>` to search the whole library with the same text.

## Search

[How are search results laid out and navigated?](https://github.com/anopheles/navigo/issues/26)

**What it searches:** only the cache, ignoring case and accents. It never calls `search3`.

**Layout:**
- Results come in **fixed groups, in this order: Artists, Albums, Playlists, Tracks.** Each is under a heading the cursor skips, and `{`/`}` jump between groups.
- The cursor starts on the **best match**.
- **Caps:** 10 artists, 20 albums, 10 playlists and 200 tracks, each capped group ending in a `… N more` row that `<CR>` expands.
- **No match:** `nothing in the library matches "xyz"`, plus "still syncing, results may be incomplete" during the first sync.

**Matching:**
- Every query word must match the start of a word: "blu ho" finds Blue Hour.
- **Tracks** match on title + artist + album, **albums** on name + album artist, **playlists** and **artists** on name.
- **Rank:** exact name, then name starting with the query, then all words in the name, then words spread over artist or album. Ties go to play count, then alphabetical order.

**Typing:**
- Results update **live** from the first character. `<CR>` moves into them; `<Esc>` restores the previous view.
- `'s` reopens the prompt with the last query filled in and selected. Search history is saved.

**Rows** behave as they do elsewhere:
- **Tracks:** `<CR>` plays the track within its album, `a`/`A` add just the track, and `ga`/`gr` go to its album or artist.
- **Albums:** `<CR>`/`l` open the album, and `a`/`A` add all of it.
- **Artists:** `<CR>`/`l` open the artist page.
- **Playlists** ([#59](https://github.com/anopheles/navigo/issues/59)): a row shows the name and, at the right, `N tracks`. `<CR>`/`l` open the playlist's tracks, and `a`/`A` add all of them. `*` says playlists can't be starred, and a playlist's own keys (`dd`, `J`, `K`) work in the Playlists section, not here. A playlist takes part in the best match like any other result, and a created, renamed or deleted playlist is found (or no longer found) at once.
- **Visual mode** can span groups. `a`/`A` add selected albums, playlists and tracks, and skip artist rows with a note.

## Queue and playback

[#8](https://github.com/anopheles/navigo/issues/8), [#10](https://github.com/anopheles/navigo/issues/10), [ADR 0002](adr/0002-navigo-owns-the-queue.md), [ADR 0004](adr/0004-shuffle-reorders-the-queue.md).

**Building the queue:**
- **`<CR>` on a track** replaces the queue with its context (the whole album or playlist) and plays from that track. `u` brings the old queue back, with no confirmation, since the queue is the client's.
- **`a`** adds after the current track ("play next"); **`A`** adds to the end. Neither starts playback.
- **`<CR>` in the queue** plays that row.

**Playing:**
- After every `TrackStarted`, the core sends the player the new neighbours, so the next track is already loaded in mpv and plays gaplessly.
- **Shuffle** (`S`) reorders the queue once, as one undoable step. **Repeat** (`R`) cycles off → all → one.
- **`:seek`** ([#62](https://github.com/anopheles/navigo/issues/62)) moves by seconds (`+10`, `-10`) or by a time (`+1:30`, `-0:30`), and goes to a time (`1:30`, `1:02:03`) or a point of the track (`50%`): a sign moves, no sign goes to. A bare number is refused (`seek: "90" is ambiguous · +90 moves by it, 1:30 goes to it`), since `90` and `1:30` would look alike but do different things; a binding with one is reported by `navigo check-config`. A count multiplies only the relative forms.

**Mix** (`gm`, `:mix`) replaces the queue with 50 tracks, or as many as a count says, and plays the first; `u` brings the old queue back. It mixes around the row under the cursor (see Around a seed), else around what plays, else from the whole library. `:mix library` and Home's Mix row always mix from the whole library. It works offline, from the cache.
- **Before the first sync:** a Mix asked for while the first sync runs waits for it, says so (`mixing from <name> once the library has synced` for a seed), and runs once the library is there, unless the queue or what plays changed since. Home's Mix row shows the sync's progress meanwhile. If the sync fails or the server can't be reached, the Mix is dropped; asking again says why (`nothing to mix · the library couldn't sync`, with `:sync` to try again). Starring, rating and playlist edits before the first sync say why they can't run instead of doing nothing.
- **Weights:** a track's chance is 1 × starred (×3) × rating (1★ left out, 2★ ×0.5, 3★ or unrated ×1, 4★ ×1.5, 5★ ×2) × staleness (played within 7 days ×0.1, rising in a straight line to ×1 at 90 days; never played ×1) × Recommendation (×3; see [ListenBrainz Recommendations](#listenbrainz-recommendations)). Play count doesn't count, so the mix doesn't lean to all-time favourites.
- **Variety:** at most 3 tracks per artist and 2 per album, and never one artist twice in a row. Artists whose names differ only by case are one artist here and in closeness; Browse still lists them apart.
- **Too few:** when the rules run out, it takes what's left and says "mixed 23 of 50 tracks"; with nothing to pick it says why and leaves the queue alone. It never pads with unrelated tracks.
- The weights, size and limits are fixed; there are no settings for them.

**Around a seed** ([Which tracks does a Mix around a seed pick?](https://github.com/anopheles/navigo/issues/69), [How is a Mix around a seed asked for and shown?](https://github.com/anopheles/navigo/issues/70), [ADR 0007](adr/0007-a-mix-around-a-seed-asks-the-server-for-similar-artists.md)):
- **Seed:** the artist, album, playlist or track under the cursor, in any list, the queue or search. A track row is always a track seed, even in an album's tracks. On a row that can't be a seed (a section, one of Home's album lists, Help), what plays is the seed. `gm` is not bound in Visual mode.
- **Closeness:** a track's closeness to the seed is the sum of: in the seed +3; a seed artist +2 × √(that artist's share of the seed's tracks); a similar artist +2.5 × rank (1 for the server's first, falling evenly to 0.5 for its last; a track by several counts the best rank); a genre +1 × the share of the seed with that genre. When similar artists came back, genre only adds to a track that already has another signal; with none, genre alone makes a track a candidate. A track with closeness 0 is not a candidate.
- **Pick weight:** closeness² × the Weights above, Recommendation included ([#84](https://github.com/anopheles/navigo/issues/84)). A 1★ track is left out even if it's in the seed, and a Recommendation never makes a track a candidate on its own.
- **From the seed:** a track in the seed, or crediting an artist who makes up at least half of it. These fill at most a third of the Mix (16 of 50), skip the 3-per-artist limit, and skip the 2-per-album limit only when the seed is an album. Never one artist twice in a row applies to everyone.
- **Opening track:** a track seed plays first whatever its rating or staleness. Other seeds open with a weighted pick from the seed's own tracks.
- **Similar artists:** on every Mix, `getArtistInfo2` for the top 3 seed artists by share (one for an artist seed), in parallel, matched by ID and by name without case. navigo stores none of them. While they load, the status line says `mixing from <name>…`, the queue stays as it is and keys keep working; if the queue or what plays changes first, the Mix is dropped, and a second `gm` replaces the pending one.
- **Without them:** offline, the lookup is skipped and the Mix is instant. Past the 2 s limit, on a Subsonic error or with an empty list, the Mix is picked from the cache alone with no extra message; the time limit doesn't count as going offline (Navidrome is slow the first time it looks an artist up), but a connection failure does, as for any request.
- **Messages:** `<name>` is what the row shows (artist, album title, playlist name or track title): `mixed 50 tracks from <name>`, `mixed 4 of 50 tracks from <name>`, or `nothing to mix from <name>` with the queue left alone. A whole-library Mix says `mixed 50 tracks`.

**Undo:** `u`/`<C-r>` walk one queue history from either window. Playlist edits on the server can't be undone.

**ReplayGain** ([How should ReplayGain behave in navigo?](https://github.com/anopheles/navigo/issues/50)): mpv does the work, from the gain tags in the stream (they survive transcoding), so it needs nothing from the server and navigo ignores the server's `replayGain` fields.
- **Modes:** `off`, `track` and `album`; there is no `auto`, because the queue is mixed after any edit, shuffle or Mix. `album` falls back to `track` per file. The default is `track`.
- **Clipping and fallback** are mpv's defaults (the gain is capped at `1/peak` where a peak is known; untagged tracks play at their own level) and are not settings.
- **Set** with `player.replaygain` and `player.replaygain_preamp`, or `:replaygain` for the session. A change is heard after about a second, once mpv's buffer drains, and the message confirms it at once.
- **Volume** is separate: ReplayGain never changes it, and it can lift a track above unity.
- **Shown** as a faint `RG track` or `RG album` in the now-playing bar when on, and the `i` popup shows the track and album gain the playing file is tagged with (mpv cannot report the gain it applies), or "no replaygain tags".
- **mpv 0.38** is still the minimum.

## Offline cache and sync

[How does the offline library cache work?](https://github.com/anopheles/navigo/issues/9)

**Cache-first:** every list and search reads the in-memory library snapshot, online or offline. The server is contacted only for sync, playback (`stream`, `scrobble`), writes (stars, playlist edits) and Home's album lists. Those come from `getAlbumList2` (50 albums) each time one opens, so Recently played is current; offline, a list shows the last answer of the session, or says it needs the server.

**What it holds:** library metadata only (the covers are separate files, see [Cover art](#cover-art)): artists, albums, tracks (with their artist lists), playlists with their tracks, stars, play counts, when each track was last played and the user's ratings. No audio, no cover art, and no client state (the [saved queue](#resume) has its own file).

**Store:**
- One versioned `encoding/gob` file per server and user, in `navigo/` under the user cache directory, written atomically.
- It's disposable: a version mismatch or a corrupt file means a full sync.
- A submission scrobble the server accepts updates the track's play count and last played in the cache, so a Mix made before the next sync doesn't pick what just played; the next sync brings the server's numbers.

**Sync:**
- **At startup:** load the snapshot and render at once, then check in the background whether the server rescanned (`getScanStatus.lastScan` / `getIndexes.lastModified`). If it did, run a full sync. Playlists and stars are always refreshed.
- **`:sync`** forces a full sync.
- No timer, no incremental diffing.
- The new snapshot replaces the old one in one step, and cursors stay on the same item by ID.
- **First run:** sections show sync progress, and search and playback wait for the sync.

## ListenBrainz Recommendations

[Map: a Mix that leans toward ListenBrainz Recommendations](https://github.com/anopheles/navigo/issues/47), [ADR 0008](adr/0008-listenbrainz-is-a-second-optional-service.md). Term: **Recommendation**.

**What:** ListenBrainz suggests up to 1,000 recordings for a user, regenerated about once a day. navigo reads them from ListenBrainz itself, without a token, since Navidrome can neither pass them on nor tell navigo the user name. They're off unless `listenbrainz.user` is set ([Configuration](#configuration)).

**Fetching** ([When does navigo fetch Recommendations, and where does it keep them?](https://github.com/anopheles/navigo/issues/81)):
- At the startup sync check (which also runs after coming back online) when the stored list is over 24 h old or missing, and always on `:sync`. No timer and no indicator.
- `GET <url>/1/cf/recommendation/user/<user>/recording?count=1000`. When `last_updated` equals the stored one, that's all; otherwise one `POST <url>/1/metadata/recording/` names them all (`inc=artist`). At most 2 requests, with the 10 s API timeout.
- **Decoded leniently:** the endpoint is experimental, so only `mbids[].recording_mbid` is required; `score` and the rest are ignored.

**Stored:** in its own versioned `encoding/gob` file per server and user beside the cache, written atomically and disposable like it. It holds the `url` and `user` it was fetched for, `last_updated`, when it was fetched, and each Recommendation's MBID, title and artists, not track IDs. A file for another `url` or `user` is ignored and fetched again. `:logout`, or turning Recommendations off, deletes it.

**Matching** ([Which tracks does a Recommendation lift, and by how much?](https://github.com/anopheles/navigo/issues/80)):
- In memory, each time the library snapshot or the list is replaced.
- A track matches when one of its artists (display or credited) equals one of the Recommendation's (its credit or each artist) **and** its title equals the Recommendation's. Both are folded: lowercase, accents dropped, `&` read as "and", other runs of non-word characters as one space. Titles also match with bracketed parts and a trailing ` - …` stripped from either side.
- A title alone never matches, and MusicBrainz IDs aren't used.
- Every copy of a song (album, compilation, live album) matches and gets the full ×3.

**In a Mix:**
- ×3 in the [Weights](#queue-and-playback), for a whole-library Mix and around a seed; ListenBrainz's score doesn't count. A 1★ track stays out, and staleness and variety apply as usual.
- A Mix never waits for a fetch. It uses the stored list however old, offline too; during the first fetch it has none.
- A Mix's messages and rows don't change. The `i` popup shows `recommended by ListenBrainz` on a track that matches.

**On Home:** a **Recommended** row after Starred, only while Recommendations are on, opens a list of tracks from the cache, titled `Recommended (534)`:
- One row per Recommendation in the library, played or not, in ListenBrainz's order; of several copies of a song, the first in the library. `:sort` and `:filter` work as on Home's other lists.
- Rows look like a playlist's tracks. `<CR>` plays the list from that row, as with a playlist; `a`/`A`, `gm`, `i`, `*`, `ga`/`gr` work as on any track.
- A new list or a sync updates it in place, the cursor staying on its track. Offline it works from the stored list.
- **Empty:** `no Recommendations yet · navigo asks ListenBrainz at startup and on :sync`, `ListenBrainz has no Recommendations for ann yet` (204), or `none of your 1,000 Recommendations are in the library`.

**Failures** (never offline, which is about the server only):
- **Unreachable, timeout, 5xx or a reply that can't be read:** keep the stored list and try again at the next startup or `:sync`. `:messages` only: `couldn't reach ListenBrainz · keeping the Recommendations from 22 Oct`, or `· no Recommendations yet`.
- **429:** wait `X-RateLimit-Reset-In` seconds and retry once, then as above.
- **404:** error `ListenBrainz has no user "ann" · check listenbrainz.user in config.yaml`; the stored list is dropped.
- **204** (not generated yet): no Recommendations; info `no Recommendations from ListenBrainz for ann yet · they can take a day or two after your first listens`.
- Recommendations missing from the metadata reply are dropped quietly.
- **A new list** says in `:messages` only: `507 of 1,000 Recommendations are in the library`.

## Resume

[Should navigo restore the last session, and what exactly?](https://github.com/anopheles/navigo/issues/51), which reverses the choice in [#9](https://github.com/anopheles/navigo/issues/9) to start with an empty queue. Terms: **Saved queue**, **Resume**.

- **What is saved:** the queue entries (track IDs in order), the current entry, the position in seconds, the volume and the repeat mode. Not the cursor, section, register or undo history; the restored queue starts a fresh undo history.
- **What is shared between devices** ([ADR 0009](adr/0009-the-saved-queue-lives-on-the-server.md)): the queue, the current entry and the position, kept on the server as its play queue (`savePlayQueue`, `getPlayQueue`). The volume and repeat mode stay on the device, since the speakers differ, and are never sent. The server records `navigo` as the client that changed the queue.
- **Where:** each device keeps its own small versioned file per server and user in the state directory, not in the disposable cache, written atomically with mode 0600. A version mismatch or a corrupt file is dropped. The file holds everything above and is the fallback when the server's queue can't be used.
- **When it is written:** on a change to the queue, current entry, volume or repeat (debounced to about 1 s), and on quit; the position every 10 s while playing, and on pause and quit. Each write goes to the file and to the server. A queue the server already holds is not sent again, so a change of volume or repeat alone costs no request, and neither does restoring the server's own queue.
- **Restore:** at startup, once the login has answered, the queue loads **paused at the saved position**: `restored 12 tracks · <Space> plays from 2:14`. Nothing starts on its own; the first play seeks to the position.
- **Which queue wins at startup:** the server's. navigo asks for it for up to 3 s. If the server answers with a queue, that queue is restored, with the volume and repeat from this device's file. In every other case the file is used: the server is unreachable, slow, errors, doesn't have the calls, or holds an empty queue. A device with no file restores the server's queue and leaves its volume and repeat alone.
- **A running session is never changed by another device.** navigo reads the server's queue once, at startup. There is no polling and no live channel: a queue another device saves later shows up at the next start.
- **Edits that never reached the server lose.** Changes made while the server could not be reached stay in the file only. At the next start, if the server then holds a queue, that queue wins and the edits are gone. Two devices running at once overwrite each other: the last write wins.
- **A failed write to the server is quiet.** It is logged, shows no message, and never marks navigo as offline. While navigo is offline nothing is sent. The next change sends the queue again.
- **Missing tracks:** entries that are not in the loaded library are dropped once, at startup, with a count (`restored 11 of 12 tracks`). With nothing left, navigo starts empty and says nothing. The later background sync does not check again. The dropped entries are also gone from the server's queue after the next write.
- **The same track twice:** with `indexBasedQueue` the server stores the current entry as an index, so the copy that was playing comes back. A server without it stores a track ID, and a track that is in the queue twice restores as the first of its copies.
- **Long queues:** the IDs of a long queue would not fit in a GET URL. the save goes out as a POST form once its query passes about 4,000 characters, or always when the server offers `formPost`.
- **Settings:** on unless `resume: false`, which turns off both the file and the server's queue, reading and writing. The server's queue is **opt in**: it is used only with `sync_queue: true`, and without it navigo never reads or writes the play queue. `:reload-config` applies `sync_queue` at once for writes; the server's queue is still read only at startup. Emptying the queue empties the saved one and clears the server's. `:logout` deletes this device's file and leaves the server's queue, which the other devices still want; another user or server has its own file and its own queue.
- **Offline** needs nothing extra: the cached library resolves the entries, the file is used, and `<Space>` gives the usual "the queue is ready" message. A start where the login can't reach the server doesn't ask for the server's queue.
- **What reaches the server (with `sync_queue`):** nothing until this session has read the server's queue once, so a laptop that started offline, or whose first read failed, can't overwrite the queue another device left; turning `sync_queue` on, or `resume` on while it is on, with `:reload-config` counts as asking to replace it. Without `sync_queue` the saved queue is read at once at startup, with no wait for the login. With it, the start waits for the login to answer, but no longer than 3 s: after that, or when the login fails in a way that is neither an unreachable server nor rejected credentials, the queue is restored from this device's file and the session does not sync. The same happens when the server's queue can't be read (it errors, times out or lacks the calls). The restore then says so, `restored 11 tracks (not synced this session)`, and a notice `queue not synced this session · couldn't get the server's queue` is kept in `:messages`. A start that is offline already says so and adds nothing. A restore that dropped tracks the cache lacked (`restored 11 of 12 tracks`), or found none of them, leaves the server's queue whole until the queue is edited. While a track plays, a change of position alone is sent once it has moved a minute; pausing, stopping and quitting send the exact position. A quit waits for the server no longer than the 3 s limit, however many writes are out.
- **Two navigo instances on one device:** the last one to write wins, in the file and on the server.

## Offline, errors and messages

[How does navigo lose and regain the server, and what do play and writes do meanwhile?](https://github.com/anopheles/navigo/issues/23), [How are errors shown, and what happens when mpv is missing, too old or dies?](https://github.com/anopheles/navigo/issues/24). Terms: **Offline**, **Message**, **Indicator**, **Popup**.

**What offline means:**
- navigo is offline when a request can't reach the server: connection refused, DNS, timeout, TLS, HTTP 5xx, or a reply that isn't from a Subsonic server. **One failure is enough.**
- A Subsonic error from the server is not offline. Rejected credentials open the login form.
- ListenBrainz failing is never offline ([ListenBrainz Recommendations](#listenbrainz-recommendations)).

**Startup:** with a cache, navigo opens from it as usual, shows `offline`, and checks the credentials once the server is back. Without one, the sections say "can't reach `<url>` · retrying", and `:login` is offered.

**While offline:**
- Lists and search work.
- **Queue changes still work**, but playback doesn't start: "offline · the queue is ready · `<Space>` plays it once the server is back".
- **Writes are refused.** Their keys stay in the hints and explain why when pressed. Saving the queue to the server is not one of them: it is skipped without a word, and the file keeps the queue ([Resume](#resume)).
- A sync cut off by going offline is thrown away; the old snapshot stays.
- **Playback never starts on its own on reconnect.**

**Playback failures:**
- **A stream that fails to load** triggers a ping. If the server is gone, navigo goes offline and stops on that track; mpv is never allowed to run through the queue. If the server answers, the file is broken: skip it with a message, and stop after 3 failures in a row.
- **No audio output** ([#77](https://github.com/anopheles/navigo/issues/77)): when mpv can't open the audio output (`audio output initialization failed`, as when the system's sound server hangs), no file is to blame, so nothing is skipped. Playback stops on that track with `couldn't play <title> · no audio output · check the system's sound` and `<Space> tries again`, and the indicator `no audio output` stays until a track really plays.
- **mpv not answering:** a player call unanswered after 5 s shows the indicator `mpv isn't responding · no audio output?` until it answers.
- **A 10 s buffering stall** also triggers a ping.
- The current track usually plays on from mpv's buffer.

**Recovery:**
- Retries back off at 2, 4, 8 … up to 60 s, and run immediately on play, a write, `:sync` or `:reconnect`.
- A successful try checks the credentials, says "back online", and runs the startup sync check.
- The now-playing bar shows `offline · next try in 8s`.

**Three tiers:**
- **Messages** (status line): info clears after 8 s, errors stay until replaced or `<Esc>`, and the newest always wins. `:messages` shows the last 100 with their times. Going offline and coming back each get one info message.
- **Indicators** (now-playing bar): lasting states, joined with " · " in the order mpv problems, offline, syncing, ReplayGain. The least important are cut first on narrow terminals.
- **Popups:** only when the user must act (bad bindings at startup, the can't-quit check). Never for the server or mpv.

**mpv:**
- **Checked at startup.** If it's missing or older than 0.38, browsing still works. The indicator says `mpv not found · install mpv 0.38 or newer to play` (or "too old"), and play keys repeat it with the OS's install hint. `player.mpv_path` covers an mpv outside the PATH.
- **If it dies,** `<Space>` restarts it and resumes the current track where it stopped; nothing restarts on its own. After 3 deaths within a minute, navigo points to the log and stops offering the restart for a minute (or until `:reconnect`).

**Unexpected errors:**
- An error navigo can't put in plain words ends with `· details in <path to navigo.log>`.
- A panic restores the terminal, prints one line naming the log, and writes the stack trace there. The log's first line mentions `--debug`.

## OS media controls

[How do we do OS media controls from Go without cgo?](https://github.com/anopheles/navigo/issues/5)

All of it sits behind one `MediaControls` interface, with a build-tagged file per OS and no cgo anywhere.
- **Linux:** navigo implements MPRIS itself with godbus/dbus v5. It owns `org.mpris.MediaPlayer2.navigo` at `/org/mpris/MediaPlayer2`, with the `MediaPlayer2` and `MediaPlayer2.Player` interfaces. The adapter caches the core's `PublishNowPlaying` effect to answer property reads, and turns D-Bus calls into events.
- **macOS and Windows:** mpv's built-in Now Playing and SMTC integration (mpv 0.39+) does the work. navigo sets `force-media-title` per file and keeps the neighbouring entries in mpv's playlist, so the OS's Next/Previous work. On mpv 0.38, playback works and a one-time notice explains the missing controls. Windows may show "Unknown app".

## Colours

[Which colours does navigo use by default?](https://github.com/anopheles/navigo/issues/25)

**Palette** (the default): only the terminal's 16 colours and text attributes (reverse, bold, faint, underline), so navigo follows the user's terminal theme on light and dark backgrounds. Each colour means one thing: **magenta = playing, cyan = a key, red = an error.**

| Element | Look |
|---|---|
| Cursor row, focused window / other window | reverse / a `▎` in the row's first column; with the accent fades (below), a tint of the accent instead: a fifth of it in the focused window, a tenth in the other |
| Playing row, progress fill | bold magenta (the row fades across when the accent fades are on); the progress bar moves in half cells (`━` for a whole, `╸` for a half), and when `playing` is a `#rrggbb` colour the fill fades up to it from a darker shade of it |
| Keys (hint line, key menu, empty-queue message, prefix menus) | bold cyan |
| Error messages | red |
| Labels, empty states, unfilled progress | faint |
| Rows of tracks, albums and artists | the title plain; the artist, number, year, duration and counts faint; a starred name's ★ gold on a true-colour terminal; a queue track that has played faint all through. A row the cursor, a selection or the playing mark styles, or that a find marks, stays whole |
| Window frames: focused / other | a rounded border in the terminal's text colour / a faint one, with the title in the top edge: reverse bold / bold |
| Visual selection | blue background, underlined; with the accent fades (below) and no `selection` set, a tint of the accent's 75° hue instead |
| Hint line | no background |
| Now-playing bar | the mode on the role that goes with it (`playing` for Normal, `selection` for Visual, `key` for the `:` line and Search) in black, the play state and queue position on bright black, the time on the mode's colour; the track and flags with no background |
| Login and popup borders | faint; the focused login field's label cyan |

**Recolouring** ([What can a user change about navigo's look?](https://github.com/anopheles/navigo/issues/52), reversing the "no colour settings" choice above): the `colors` block recolours four **roles**: `playing`, `key`, `error` and `selection`. A value is one of the 16 colours by name (`red`, `bright-red`, `gray`), by number from 0 to 15, or `#rrggbb`.
- **The other elements** are attributes (reverse, bold, faint, underline) and follow whatever colours are set.
- **Themes** (added after the first decision against presets): `theme:` picks a named palette for the four roles, and a role in `colors:` replaces the theme's colour for it. The themes are `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, `catppuccin-mocha`, `gruvbox-dark`, `gruvbox-light`, `nord`, `dracula`, `solarized-dark`, `solarized-light`, `tokyo-night`, `tokyo-night-storm`, `tokyo-night-day`, `one-dark`, `one-light`, `rose-pine`, `rose-pine-moon`, `rose-pine-dawn`, `everforest-dark`, `kanagawa`, `monokai`. Names are not case-sensitive, and an unknown one is reported with this list. A theme gives only the four role colours, as `#rrggbb`; the text and background are the terminal's, so a theme looks right in a terminal that uses the same one, and the selection colour is a background chosen to read against that theme's text. With no `theme:`, the roles are the terminal's own palette, which follows whatever theme the terminal has.
- **No single accent:** one accent colour would merge "playing" with "focus" and, for red, with "error".
- **Accent fades:** when `playing` is a `#rrggbb` colour (a theme, `colors:`, or the default below) on a terminal with 24-bit colour and colours on, the playing colour also fades towards blue (two hues turned by 35° and 75°), and that single accent drives: the focused window's frame (the middle colour) and its title (the whole fade, instead of reverse), the playing row, the card's title, the now-playing bar's title, and the progress rule. The focused window's cursor row becomes a fifth of the accent over the terminal's own background (asked for with OSC 11; black until the terminal answers) instead of reverse, so it suits light and dark terminals. Unless a theme or `colors:` sets `selection`, the Visual selection becomes a third of the accent's last hue (turned 75°) over the same background, without underline: it follows the cover like the rest of the accent, and its hue keeps it apart from the cursor moving through it. With `playing` one of the 16 colours, on fewer colours, or with `NO_COLOR`, none of this is drawn and the reverse cursor and plain frame stay: nothing depends on the fades.
- **Playing from the cover:** with no `theme:` and no `playing:` (or `playing: cover`), a terminal that reports 24-bit colour (`COLORTERM=truecolor`) takes the playing colour from the playing album's cover: its dominant vivid colour (hues vote in twelve bins, by saturation squared, so a small bright patch beats a large dull field), evened out to a mid-tone (saturation 0.5 to 0.9, lightness 0.52 to 0.66) that reads on light and dark backgrounds. The accent, the progress bar and the playing row follow it, and change with the album. Until a cover is in, for a cover with no colour, and when nothing plays, the colour is `#c25bd8`, a mid-tone magenta. Every other terminal, and `NO_COLOR`, keeps the terminal's own magenta. Setting `playing:` to a colour, or a `theme:`, replaces both; `cover_art: false` leaves the magenta.
- **Low colour depth:** a hex colour snaps to the nearest colour of a 256- or 16-colour terminal.
- **Safeguard:** the Visual selection stays underlined whatever colour it has (except the accent's tint, which navigo mixes itself and only draws with colours on), so a `selection` close to the text colour can't hide it, and it shows with colours off. navigo can't know the terminal's text colour, so it cannot check for that.

**Without colour:** no meaning depends on colour alone, so navigo works fully with colours off. Any non-empty `NO_COLOR` turns colours off.

## Cover art

[Cover art in the now-playing bar](https://github.com/anopheles/navigo/issues/60). Tried first as a prototype on the real screen with a real server (`prototype/cover-art`).

- **Where:** while a track plays, a **card** heads the queue window: the album's cover on the left (12×6 cells), then the title, artist and album, the year and genre, and the track that plays next (none under repeat one or at the end of the queue), and a faint rule under it. It shows when the terminal is at least 100 columns by 28 rows, and takes seven rows from the queue; the now-playing bar keeps everything else. Below that size, with nothing playing, or with cover art off, the queue has its whole window. (It was first a six-row card in place of the now-playing bar; it moved to the queue so that nothing playing leaves no empty rows under the windows.) `gc` (the `cover` command) shows the cover large in a popup, at any size that fits; `q` closes it.
- **What is drawn:** with kitty graphics where the terminal has them (kitty, Ghostty, WezTerm; under tmux only with `allow-passthrough on`), by unicode placeholders, which scroll and redraw like text. Otherwise with block characters, two by two pixels a cell, written for navigo: a clear pixel is the terminal's own background, so a cover with transparency works on light, translucent and pictured terminals, and a half-clear pixel is blended against the terminal's background (asked for with OSC 11; without an answer it is drawn from half opaque). A cover that isn't square is centred on a transparent square.
- **When it is off:** `cover_art: false`, `NO_COLOR`, or a terminal with fewer than 256 colours. `cover_art: blocks` forces the blocks, such as over a slow SSH link; `cover_art: kitty` forces kitty graphics. A bad value is reported like any other.
- **Fetching:** `getCoverArt` for the album's ID (the song's, if the track has no album), when a track starts, in the background. One fetch serves every track of the album. The server may ignore the `size` it is asked for (Navidrome does), so covers are decoded (WebP, PNG, JPEG) and scaled by navigo to 384 pixels.
- **Cache:** the files the server sent, in `cache/covers/` under the cache directory, named by a hash of the ID and trimmed to 100 MB by least recent use at start. IDs are trusted not to change under a cover, so a file is never refreshed.
- **States:** a faint placeholder while it loads; blank cells (the text stays) if the server has no cover or the fetch failed, with no message. A failed fetch is tried again after 30 s, when the cover is next wanted.
- **Kitty images** are kept in the terminal at the sizes drawn, the eight most recently used; the oldest is freed beyond that.
- **Not built:** artist and playlist images, kitty graphics other than unicode placeholders, sixel, the cover in the library and queue windows.

## Testing

[How is navigo tested end to end?](https://github.com/anopheles/navigo/issues/14), [ADR 0003](adr/0003-fake-the-server-at-http.md).

- **Unit tests:**
  - the core, by the effects it returns;
  - the keymap engine and command parser, as pure functions;
  - the mpv IPC client, against a fake socket;
  - the subsonic client, against the `httptest` fake server;
  - the ListenBrainz client, against an `httptest` fake ListenBrainz with hand-written fixtures (a list, an unchanged `last_updated`, 204, 404, 429 then success, 5xx, metadata missing MBIDs), and Recommendation matching as a pure function. There's no e2e scenario: a synthetic ListenBrainz user can't be recorded, and CI doesn't call the live API;
  - the config values and errors for ReplayGain, Resume and colours, the saved queue's round trip (including a corrupt file, missing tracks, offline start and `:logout`) and its sync between two devices (a second device restoring the first one's queue paused at its position, the server being unreachable, slow, erroring, lacking the calls or empty, volume and repeat staying local, `:logout` and `resume: false` leaving the server's queue, a failed write staying quiet), colour parsing and its degrade at each depth, and the Player call order for `SetReplayGain` on the fake.
- **The fake server** (`internal/subsonic/subsonictest`) replays JSON recorded from a throwaway Navidrome serving a **synthetic fixture library**; never the live one, since the repo is public. It checks token+salt auth and fails a test on a plain password, a reused salt or an unrecorded request. The exception is the play queue calls, which it answers from one queue per user, so tests can run two devices against it, and which it can fail on demand. Those answers are not recorded from Navidrome. Re-record with `tools/fixtures.sh up && tools/fixtures.sh record`.
- **Layer A (in-process UI):** the real Bubble Tea model, core and runner, with the real client against the fake server and the fake Player. Most checks are targeted assertions; a few golden frames cover the layouts (120×40, plus 80×40 for narrow), refreshed with `-update`.
- **Layer B (e2e):** `go test -tags e2e ./e2e` builds the binary and drives it in a pty through a terminal emulator, against a real Navidrome and real mpv (`--ao=null`), with its own HOME and XDG dirs. It runs against the live server from `.env.e2e` locally, and against the docker Navidrome with the fixture library through `tools/e2e/run.sh` in CI. It skips without credentials. There are three scenarios: the new-user path, transport, and cold start from the cache. Keymap edge cases stay in layer A.
- **CI:**
  - **check** (gofmt, vet including the e2e tag, tests, licence allowlist) runs in `golang:1.26` with Debian's mpv, because Ubuntu 24.04's mpv 0.37 is too old;
  - **e2e** runs the layer B script;
  - **build** cross-compiles the six targets.

  `tools/ci/local.sh` runs the same gate locally from a cached image.

## Licence and dependencies

[What licence does navigo use?](https://github.com/anopheles/navigo/issues/12), [ADR 0005](adr/0005-mit-licence-permissive-deps-only.md).

- **Licence:** navigo is **MIT**.
- **Linked modules** must be MIT, BSD-2-Clause, BSD-3-Clause, Apache-2.0, ISC or MPL-2.0; CI checks this with `go-licenses`. Spawned programs (mpv) are exempt.
- **No GPL-derived code:** GPL clients may only be observed, never used as source.
- **Before adding any module**, check its latest released version and use that.
- **Main modules:**
  - **UI:** Bubble Tea v2, Lip Gloss v2, Bubbles v2 (`textinput` only), `charmbracelet/x`.
  - **System:** godbus/dbus v5 (MPRIS), go-winio (Windows pipes), go-keyring.
  - **Config:** go.yaml.in/yaml/v3.
  - **Search:** golang.org/x/text (folding accents, also for matching Recommendations).
  - **Cover art:** golang.org/x/image/webp (BSD-3-Clause), to decode what the server sends.
  - **Tests only:** `x/xpty` and `x/vt`.

## Filled in while writing this spec

Small gaps no ticket settled, filled in here so the spec is complete; revisit them freely.

- **Where history lives.** The `:` and search histories are saved in a `navigo/` directory: `$XDG_STATE_HOME` (or `~/.local/state`) on Linux, and the user cache directory on macOS and Windows. Go has no per-OS state directory.
- **`/` in Help** filters the rows as you type rather than finding the next match, since Help is a reference to narrow down; `<Esc>` puts the old filter back. The key menu leaves motions out, so they scroll it.
- **The scroll wheel.** The motions row lists it, and the wheel scrolls the list under it as well as setting the volume (Startup).
- **Where `:sort` and `:filter` work.** On flat lists only: a playlist's tracks are left out because edits go to the server by row, and grouped or ranked lists (an artist page, search results, Help) keep their order.
- **MPRIS's Shuffle.** navigo's shuffle reorders the queue once (ADR 0004), so the Shuffle property is always false and can't be set; `S` shuffles.
- **The star key.** No ticket gave starring a key; `*` stars the row or the selection, or unstars them when all are starred (`:star`).
- **Two settings called "hints".** Top-level `hints` turns the **hint line** on or off ([#13](https://github.com/anopheles/navigo/issues/13)). `keys.hints` and `keys.hint-delay` control **prefix menus** ([#7](https://github.com/anopheles/navigo/issues/7)). They stay as decided, but renaming the second to `keys.prefix-menus` would avoid the confusion.

## The walking skeleton and this spec

The skeleton in this repo ([#16](https://github.com/anopheles/navigo/issues/16)–[#20](https://github.com/anopheles/navigo/issues/20)) logs in, lists albums, opens one and plays a track through mpv with vim keys, tested in layers A and B. Already built to this spec:
- **Server and fixtures:** the subsonic client, the fixture library and the fake server.
- **Playback:** the mpv IPC client and Player (window of three, sequenced effects).
- **Core:** queue, undo/redo, play now / add / play next, transport, scrobbling.
- **Library:** the snapshot, the gob cache file, startup and `:sync` sync with progress, and lists read from the cache ([#31](https://github.com/anopheles/navigo/issues/31)).
- **Input:** the keymap engine (counts, prefixes, scopes, notation) and the command registry.
- **Session:** the login form, keyring and config saving.
- **Screen:** the two windows and narrow layout, the hint line, the empty-queue message, the now-playing bar and the `:` line.
- **Offline:** detection, backoff with the countdown indicator, `:reconnect`, the 10/60 s timeouts, the stream-failure and buffering-stall pings, the 3-skip cap, and starting from the cache when the server is unreachable ([#32](https://github.com/anopheles/navigo/issues/32)).
- **Artists:** the Artists section and artist pages with the Appears on group, `{`/`}`, `ga`/`gr` (with a chooser for several artists), `'a`/`:artists`, and sections that keep their path ([#34](https://github.com/anopheles/navigo/issues/34)).
- **Search:** `'s`/`:search`, live results from the cache in Artists/Albums/Playlists/Tracks groups with caps and `… N more`, word-start matching without case or accents, ranking, the selected last query, and the saved history ([#38](https://github.com/anopheles/navigo/issues/38)).
- **Visual mode and queue editing:** `V`/`v` with the selection highlighted and counts in the hint line, `:` on the selection, spanning Search groups with artist rows skipped; the `d`/`y` operators with motions, counts and doubling; the one register shared by every view; `p`/`P`, `x`, `J`/`K`, each an undoable step that keeps the player's neighbours right; `"x` rejected ([#35](https://github.com/anopheles/navigo/issues/35)).
- **The `:` line:** fuzzy matching of names and descriptions with each match's key, what applies here first; `<Tab>`/`<S-Tab>` completion of names and arguments (`:search` from its history); `tip: next time press …`; and the history, saved in the state directory ([#41](https://github.com/anopheles/navigo/issues/41)).
- **Playlists:** the Playlists list and playlist tracks with `'p`; removals (`dd`, `x`, `d{motion}`, Visual `d`) after a y/n Confirmation, into the register; `p`/`P` and `J`/`K` on the server; `dd` on the list; `:playlist-add`, `:playlist-new`; no undo; edits refused offline ([#37](https://github.com/anopheles/navigo/issues/37)).
- **Playback extras:** `=`/`+`/`-` volume (kept for mpv started later), `S` shuffle keeping the current track first, `R` repeat off/all/one driving what plays next and shown in the now-playing bar, the `i`/`:info` popup, and `*` stars, refused offline ([#39](https://github.com/anopheles/navigo/issues/39)).
- **Help and menus:** the Help section (`'?`, `<F1>`, `:help`) built from the bindings in effect, by scope, with Visual mode and the commands no key runs, and `/` filtering it; the `?` key menu grouped Play, Queue, Select, Go, Help, where a listed key runs, motions scroll, and `<F1>` goes to Help; prefix menus after 500 ms, including after an operator ([#40](https://github.com/anopheles/navigo/issues/40)).
- **Rebinding:** the `keys:` block with its options, `~` and `nop`, validation with every problem in a popup as `config.yaml:LINE:COL: message`, the can't-quit refusal, `navigo check-config`, the table-driven `:` line (`cmdline:`), `:nmap`/`:vmap`/`:cmap` and the `unmap` forms, `:map`, `:reload-config`, and `:queue-clear` from the example ([#42](https://github.com/anopheles/navigo/issues/42)).
- **Home:** startup on Home, its five album lists from the server with the last answer kept for offline, `'h` and `:home` ([#33](https://github.com/anopheles/navigo/issues/33)).
- **Motions, find and jumps:** `<C-f>`/`<C-b>`, `<C-e>`/`<C-y>`, `H`/`M`/`L`, `zz`/`zt`/`zb`, `[count]%`; `/` and `:find-back` (smart-case, highlights, wrapping `n`/`N`, `:noh`/`<Esc>`), and a `/` that finds nothing offering `<CR>` to search the library; the jump list (`<C-o>`, `<C-i>`, `:forward`) recording marks, section commands, `ga`/`gr`/`gp`, `gg`/`G` and finds; `gp` ([#36](https://github.com/anopheles/navigo/issues/36)).
- **The remaining `:` commands:** `:logout` (stops playback, deletes the keyring entry, says when the config still holds a password), `:sort` and `:filter` with the view in the list's title and the cursor kept on its item, `:hints on|off`. The cache keeps genres and track years for `:filter` (cache version 2) ([#44](https://github.com/anopheles/navigo/issues/44)).
- **mpv's health:** the startup check (`mpv --version`, honouring `player.mpv_path`) with the indicator and the OS's install hint on play keys; `<Space>` after mpv died resumes the track where it stopped; 3 deaths in a minute stop restarts for a minute or until `:reconnect`, pointing to the log ([#30](https://github.com/anopheles/navigo/issues/30)).
- **Colours:** the 16-colour palette and attributes of the table above, the faint rule above the now-playing bar, and colours off for any non-empty `NO_COLOR`; the Visual selection and find matches are also underlined so they show without colour ([#28](https://github.com/anopheles/navigo/issues/28)).
- **OS media controls:** the core's `PublishNowPlaying`; MPRIS on Linux (`org.mpris.MediaPlayer2.navigo`, or `.instance<pid>` for a second navigo) with play, pause, next, previous, stop, seek, position, volume and loop status; `force-media-title` on every file for mpv's own controls on macOS and Windows, and a one-time notice there for mpv 0.38 ([#43](https://github.com/anopheles/navigo/issues/43)).
- **Messages:** the three tiers (status-line messages with expiry and `:messages`, indicator slots, popups), the log path on unexpected errors, and crash handling ([#29](https://github.com/anopheles/navigo/issues/29)).
- **Tests:** layer A, and the three layer B scenarios (the new-user path from search to playing, transport, cold start from the cache) on the fixture library and live ([#45](https://github.com/anopheles/navigo/issues/45)).

Still to build for v1: nothing; every item above is built. Added since, and still to build: ReplayGain, Resume and colour roles ([#54](https://github.com/anopheles/navigo/issues/54)).
