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
13. [Offline, errors and messages](#offline-errors-and-messages)
14. [OS media controls](#os-media-controls)
15. [Colours](#colours)
16. [Testing](#testing)
17. [Licence and dependencies](#licence-and-dependencies)
18. [Filled in while writing this spec](#filled-in-while-writing-this-spec)
19. [The walking skeleton and this spec](#the-walking-skeleton-and-this-spec)

## Scope

**In v1:**
- Browsing: Home, Artists, Albums, Playlists, Search and Help.
- Playback through mpv, with a queue edited like a vim buffer.
- Playlist editing on the server, and starring.
- OS media controls.
- An offline library cache.
- Login with the OS keyring.
- Full key rebinding in YAML.

**Out of scope:**
- **Beyond-parity features:** lyrics, scrobbling settings, multiple servers, cover art in the terminal.
- **Offline playback** (downloading audio) and **session restore**: v1 opens on Home with an empty queue ([#9](https://github.com/anopheles/navigo/issues/9)).
- **Release packaging** (goreleaser, package managers).
- **Configurable themes** ([#25](https://github.com/anopheles/navigo/issues/25)).
- **Free-text filtering within a list:** `/` jumps between matches, and the Search section covers the whole library ([#26](https://github.com/anopheles/navigo/issues/26)).

## Architecture

Decided in [What is navigo's architecture and domain model?](https://github.com/anopheles/navigo/issues/10), with ADRs [0001](adr/0001-single-writer-core-with-effects-as-data.md)–[0004](adr/0004-shuffle-reorders-the-queue.md).

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
- **Calls:** `Play(Span{Prev, Current, Next})`, `SetNeighbours(prev, next)`, `Pause`/`Resume`, `SeekTo`/`SeekBy`, `SetVolume`, `Stop`.
- **Events:** `TrackStarted`, `TrackEnded{Finished|Stopped|Failed}`, `Position`, `Paused`, `Buffering`, `Volume`, `Died`.
- **Adapters:** two, the mpv one and an in-memory fake.

**mpv** ([#4](https://github.com/anopheles/navigo/issues/4), [ADR 0002](adr/0002-navigo-owns-the-queue.md)):
- **Spawn:** navigo starts its own mpv with `--idle=yes --no-terminal --no-video --audio-display=no --no-config --load-scripts=no --ytdl=no`, then `player.mpv_args`, then `--input-ipc-server`.
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

## Server API

[Which Subsonic/OpenSubsonic API surface does navigo v1 need?](https://github.com/anopheles/navigo/issues/2), plus timeouts from [#23](https://github.com/anopheles/navigo/issues/23).

- **Client:** navigo's own thin JSON client (`f=json`, `v=1.16.1`). Token+salt auth with a **fresh salt per request** and per stream URL, and never `p=`.
- **Endpoints:**
  - session and library: `ping`, `getOpenSubsonicExtensions`, `getArtists`, `getArtist`, `getAlbumList2`, `getAlbum`, `getPlaylists`, `getPlaylist`, `search3`;
  - playback: `stream`, `getCoverArt`, `scrobble`;
  - stars and change detection: `star`/`unstar`, `getStarred2`, `getScanStatus`, `getIndexes`;
  - playlist edits: `createPlaylist` (also with `playlistId`, to reorder: it replaces the tracks), `updatePlaylist`, `deletePlaylist`.
- **Extensions:** `formPost` (calls go as POST once offered, keeping tokens out of URLs) and `transcodeOffset` (seeking in transcoded streams). Optional features are gated on `getOpenSubsonicExtensions`.
- **Limits:** Navidrome caps list endpoints at 500. An empty-query `search3` lists the whole library with offsets; that's the sync path, with a `getAlbumList2`+`getAlbum` fallback.
- **Scrobbles:** `stream` doesn't count a play. navigo sends `scrobble submission=false` when a track starts and `submission=true` after half the track or 4 minutes listened, whichever comes first, once per queue-entry play. Seeks don't count toward that.
- **Timeouts:** 10 s for API calls, 60 s per sync page.
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
- `:logout` deletes the keyring entry and returns to the form. Logging in as another user or server starts a fresh session.
- If `config.yaml` holds a password and group or others can read it, navigo warns but still runs (Unix only). navigo creates the file with mode 0600.
- No API-key login in v1 (Navidrome 0.63 doesn't offer it). One `server:` block only.

## Configuration

One YAML file, `navigo/config.yaml` under the user config directory (`$XDG_CONFIG_HOME` or `~/.config` on Linux, `~/Library/Application Support` on macOS, `%AppData%` on Windows). `--config` points elsewhere. Every key is optional.

```yaml
server:
  url: https://music.example.com
  user: ann
  password_command: pass show navidrome   # or password: …, or nothing (keyring)

player:
  mpv_path: /opt/mpv/bin/mpv               # when mpv isn't on PATH
  mpv_args: [--audio-device=pulse/hdmi]    # extra mpv options; must start with --

hints: true          # the hint line; :hints on|off for the session

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

**No colour settings in v1** ([#25](https://github.com/anopheles/navigo/issues/25)).

## Screens and navigation

[What screens, layout and navigation model does navigo have?](https://github.com/anopheles/navigo/issues/8), [#13](https://github.com/anopheles/navigo/issues/13), [#21](https://github.com/anopheles/navigo/issues/21).

**Layout** (top to bottom):
- **Two windows** side by side. The **library window** (3/5 of the width) shows one list at a time, drilled into with `l`/`<CR>` and out with `h`. The **queue window** (2/5) always shows the queue.
- **The hint line.**
- **The now-playing bar** (2 lines, never focused). Line 1 has ▶/⏸, title, artist, album and the position in the queue, plus indicators. Line 2 has the progress bar, elapsed/total time and the shuffle/repeat flags.
- **The status line**, which shows messages, the `:` line and pending keys.

**Windows:**
- **Switching:** `<C-w>h`/`<C-w>l`/`<C-w>w`, with `<Tab>` as another key for `<C-w>w`.
- **Closing the queue window:** `<C-w>o` closes it until `:queue` or `'q`.
- **Narrow terminals:** below 100 columns only one window is shown at a time, and `<C-w>w`/`<Tab>` swaps them.
- Window widths are fixed.

**Popups** (a Confirmation, track info, the key menu, `:messages`) appear over both windows. They take the `popup` scope, and `q`/`<Esc>` close them.

**Sections:** Home, Artists, Albums, Playlists, Search, Help. There is no Tracks section. Each section remembers its drill-down path and cursor.
- **Home:** a list of album lists, in this order: Recently added, Most played, Recently played, Random, Starred.
- **Artists:** every artist with at least one track, sorted by the server's sort name. Each row shows the artist's own album count, or "appears on N" for artists with no albums of their own.
- **An artist's page:** their own albums, then a `── Appears on ──` heading (skipped by the cursor) and the albums where they only have tracks. `{`/`}` jump between the two groups. Opening an Appears on album puts the cursor on that artist's first track.
- **Help:** generated from the bindings in effect, and searchable with `/`.

**Marks** jump to a section: `'h` Home, `'q` the queue window, `'a` Artists, `'b` Albums, `'p` Playlists, `'s` Search, `'?` Help. They are ordinary bindings to `:home`, `:artists`, ….

**Jump list** (`<C-o>`/`<C-i>`):
- **Recorded:** marks, section commands, `ga`/`gr`/`gp`, `gg`/`G`, and `/` with `n`/`N`.
- **Not recorded:** `j`/`k`, `<C-d>`/`<C-u>` and `l`/`h`.
- `<C-i>` works where the terminal tells it apart from Tab, and `:forward` works everywhere.

**Startup:** opens on Home, focus in the library window, empty queue. No mouse support.

## Keys

[What is navigo's vim keymap grammar and default key table?](https://github.com/anopheles/navigo/issues/6), amended by [#13](https://github.com/anopheles/navigo/issues/13).

**Modes:**
- **Normal.**
- **Visual:** `V`/`v`, whole rows. `:` applies a command to the selection.
- **Command-line** (`:`), **Search** (`/`) and **Prompt** (text asked for by a popup). These three share one line-editing table: `<C-w>`, `<C-u>`, `<C-a>`/`<C-e>`, history on `<Up>`/`<Down>`, `<Esc>` cancels and `<CR>` accepts.

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
| Playback | `<Space>` play/pause · `>` `<` next/previous · `.` `,` seek ±10 s · `=` `+` `-` volume ±5 · `S` shuffle · `R` repeat off → all → one · `X` stop |
| Add and play | `<CR>` play now · `a` play next · `A` add to end · `i` info · `*` star/unstar · `yy` `y{motion}` yank |
| Queue | `dd` `x` `d{motion}` remove · `yy` · `p` `P` paste after/before · `J` `K` move · `u` `<C-r>` undo/redo (from either window) |
| Playlist tracks | as the queue, but edits go to the server: every removal asks y/n first; `u`/`<C-r>` explain that `P` puts the last removal back |
| Playlist list | `dd` deletes the playlist after y/n |
| Windows | `<C-w>h` `<C-w>l` `<C-w>w` `<Tab>` · `<C-w>o` |
| Help and quit | `?` key menu · `<F1>` `:help` · `<Esc>` cancel · `<C-c>` cancel + "type :q to quit" · `q` closes a popup only · `:q` `:qa` `ZZ` `ZQ` quit |

`s` and `r` are deliberately unbound, `\` is the leader and has no default bindings, and `<C-Space>` is never a default (because of a Windows console bug).

**`:`-only commands:**
- library and playlists: `:sort <field>`, `:filter genre|year …`, `:playlist-add <name>`, `:playlist-new`, `:sync`;
- session: `:login`, `:logout`, `:reconnect`, `:hints on|off`;
- the rest: `:messages`, `:info`, `:map`, `:reload-config`.

The `:` line has Tab completion for command names and arguments.

## Discoverability

[How does a new user discover navigo without reading docs?](https://github.com/anopheles/navigo/issues/13). Every aid is drawn from the live binding table, so they all follow rebinding.

- **Hint line:** the keys that matter for the row under the cursor and the mode, such as `<CR> play  A add to queue  a play next  V select  ga album  <C-o> go back  's search  <Tab> other window  '? help`. In Visual mode it shows counts ("A add 3 tracks to queue"). It's on by default.
- **Empty-queue message:** the empty queue window says what to press first.
- **Prefix menus:** these appear after a pure prefix (`g`, `'`, `<C-w>`, `d`, leader) has waited 500 ms.
- **Key menu (`?`):** a popup with the keys for what's under the cursor, grouped as Play, Queue, Select, Go and Help. Pressing a key runs it. It scrolls to fit 24 rows.
- **The `:` line teaches keys:** typing fuzzy-matches command names and descriptions, listing each match's key. Matches that apply here come first, then whole words, then scattered letters. Running a command that has a key shows `tip: next time press A`.
- **Every queue change says what it did** and how to undo it: "3 tracks added to the end of the queue (12 in queue) · u undoes".
- **A `/` that finds nothing** offers `<CR>` to search the whole library with the same text.

## Search

[How are search results laid out and navigated?](https://github.com/anopheles/navigo/issues/26)

**What it searches:** only the cache, ignoring case and accents. It never calls `search3`.

**Layout:**
- Results come in **fixed groups, in this order: Artists, Albums, Tracks.** Each is under a heading the cursor skips, and `{`/`}` jump between groups.
- The cursor starts on the **best match**.
- **Caps:** 10 artists, 20 albums and 200 tracks, each capped group ending in a `… N more` row that `<CR>` expands.
- **No match:** `nothing in the library matches "xyz"`, plus "still syncing, results may be incomplete" during the first sync.

**Matching:**
- Every query word must match the start of a word: "blu ho" finds Blue Hour.
- **Tracks** match on title + artist + album, **albums** on name + album artist, **artists** on name.
- **Rank:** exact name, then name starting with the query, then all words in the name, then words spread over artist or album. Ties go to play count, then alphabetical order.

**Typing:**
- Results update **live** from the first character. `<CR>` moves into them; `<Esc>` restores the previous view.
- `'s` reopens the prompt with the last query filled in and selected. Search history is saved.

**Rows** behave as they do elsewhere:
- **Tracks:** `<CR>` plays the track within its album, `a`/`A` add just the track, and `ga`/`gr` go to its album or artist.
- **Albums:** `<CR>`/`l` open the album, and `a`/`A` add all of it.
- **Artists:** `<CR>`/`l` open the artist page.
- **Visual mode** can span groups. `a`/`A` add selected albums and tracks, and skip artist rows with a note.

## Queue and playback

[#8](https://github.com/anopheles/navigo/issues/8), [#10](https://github.com/anopheles/navigo/issues/10), [ADR 0002](adr/0002-navigo-owns-the-queue.md), [ADR 0004](adr/0004-shuffle-reorders-the-queue.md).

**Building the queue:**
- **`<CR>` on a track** replaces the queue with its context (the whole album or playlist) and plays from that track. `u` brings the old queue back, with no confirmation, since the queue is the client's.
- **`a`** adds after the current track ("play next"); **`A`** adds to the end. Neither starts playback.
- **`<CR>` in the queue** plays that row.

**Playing:**
- After every `TrackStarted`, the core sends the player the new neighbours, so the next track is already loaded in mpv and plays gaplessly.
- **Shuffle** (`S`) reorders the queue once, as one undoable step. **Repeat** (`R`) cycles off → all → one.

**Undo:** `u`/`<C-r>` walk one queue history from either window. Playlist edits on the server can't be undone.

## Offline cache and sync

[How does the offline library cache work?](https://github.com/anopheles/navigo/issues/9)

**Cache-first:** every list and search reads the in-memory library snapshot, online or offline. The server is contacted only for sync, playback (`stream`, `scrobble`) and writes (stars, playlist edits).

**What it holds:** library metadata only: artists, albums, tracks (with their artist lists), playlists with their tracks, stars and play counts. No audio, no cover art, no client state.

**Store:**
- One versioned `encoding/gob` file per server and user, in `navigo/` under the user cache directory, written atomically.
- It's disposable: a version mismatch or a corrupt file means a full sync.

**Sync:**
- **At startup:** load the snapshot and render at once, then check in the background whether the server rescanned (`getScanStatus.lastScan` / `getIndexes.lastModified`). If it did, run a full sync. Playlists and stars are always refreshed.
- **`:sync`** forces a full sync.
- No timer, no incremental diffing.
- The new snapshot replaces the old one in one step, and cursors stay on the same item by ID.
- **First run:** sections show sync progress, and search and playback wait for the sync.

## Offline, errors and messages

[How does navigo lose and regain the server, and what do play and writes do meanwhile?](https://github.com/anopheles/navigo/issues/23), [How are errors shown, and what happens when mpv is missing, too old or dies?](https://github.com/anopheles/navigo/issues/24). Terms: **Offline**, **Message**, **Indicator**, **Popup**.

**What offline means:**
- navigo is offline when a request can't reach the server: connection refused, DNS, timeout, TLS, HTTP 5xx, or a reply that isn't from a Subsonic server. **One failure is enough.**
- A Subsonic error from the server is not offline. Rejected credentials open the login form.

**Startup:** with a cache, navigo opens from it as usual, shows `offline`, and checks the credentials once the server is back. Without one, the sections say "can't reach `<url>` · retrying", and `:login` is offered.

**While offline:**
- Lists and search work.
- **Queue changes still work**, but playback doesn't start: "offline · the queue is ready · `<Space>` plays it once the server is back".
- **Writes are refused.** Their keys stay in the hints and explain why when pressed.
- A sync cut off by going offline is thrown away; the old snapshot stays.
- **Playback never starts on its own on reconnect.**

**Playback failures:**
- **A stream that fails to load** triggers a ping. If the server is gone, navigo goes offline and stops on that track; mpv is never allowed to run through the queue. If the server answers, the file is broken: skip it with a message, and stop after 3 failures in a row.
- **A 10 s buffering stall** also triggers a ping.
- The current track usually plays on from mpv's buffer.

**Recovery:**
- Retries back off at 2, 4, 8 … up to 60 s, and run immediately on play, a write, `:sync` or `:reconnect`.
- A successful try checks the credentials, says "back online", and runs the startup sync check.
- The now-playing bar shows `offline · next try in 8s`.

**Three tiers:**
- **Messages** (status line): info clears after 8 s, errors stay until replaced or `<Esc>`, and the newest always wins. `:messages` shows the last 100 with their times. Going offline and coming back each get one info message.
- **Indicators** (now-playing bar): lasting states, joined with " · " in the order mpv problems, offline, syncing. The least important are cut first on narrow terminals.
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

**Palette:** only the terminal's 16 colours and text attributes (reverse, bold, faint, underline), so navigo follows the user's terminal theme on light and dark backgrounds. Each colour means one thing: **magenta = playing, cyan = a key, red = an error.**

| Element | Look |
|---|---|
| Cursor row, focused window / other window | reverse / underline |
| Playing row, progress fill | bold magenta |
| Keys (hint line, key menu, empty-queue message, prefix menus) | bold cyan |
| Error messages | red |
| Labels, empty states, unfilled progress | faint |
| Window titles: focused / other | reverse bold / bold |
| Visual selection | blue background |
| Hint line, now-playing bar | no background; a faint rule above the now-playing bar |
| Login and popup borders | faint; the focused login field's label cyan |

**Without colour:** no meaning depends on colour alone, so navigo works fully with colours off. Any non-empty `NO_COLOR` turns colours off.

## Testing

[How is navigo tested end to end?](https://github.com/anopheles/navigo/issues/14), [ADR 0003](adr/0003-fake-the-server-at-http.md).

- **Unit tests:**
  - the core, by the effects it returns;
  - the keymap engine and command parser, as pure functions;
  - the mpv IPC client, against a fake socket;
  - the subsonic client, against the `httptest` fake server.
- **The fake server** (`internal/subsonic/subsonictest`) replays JSON recorded from a throwaway Navidrome serving a **synthetic fixture library**; never the live one, since the repo is public. It checks token+salt auth and fails a test on a plain password, a reused salt or an unrecorded request. Re-record with `tools/fixtures.sh up && tools/fixtures.sh record`.
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
  - **Search:** golang.org/x/text (folding accents).
  - **Tests only:** `x/xpty` and `x/vt`.

## Filled in while writing this spec

Small gaps no ticket settled, filled in here so the spec is complete; revisit them freely.

- **Where history lives.** The `:` and search histories are saved in a `navigo/` directory: `$XDG_STATE_HOME` (or `~/.local/state`) on Linux, and the user cache directory on macOS and Windows. Go has no per-OS state directory.
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
- **Search:** `'s`/`:search`, live results from the cache in Artists/Albums/Tracks groups with caps and `… N more`, word-start matching without case or accents, ranking, the selected last query, and the saved history ([#38](https://github.com/anopheles/navigo/issues/38)).
- **Visual mode and queue editing:** `V`/`v` with the selection highlighted and counts in the hint line, `:` on the selection, spanning Search groups with artist rows skipped; the `d`/`y` operators with motions, counts and doubling; the one register shared by every view; `p`/`P`, `x`, `J`/`K`, each an undoable step that keeps the player's neighbours right; `"x` rejected ([#35](https://github.com/anopheles/navigo/issues/35)).
- **The `:` line:** fuzzy matching of names and descriptions with each match's key, what applies here first; `<Tab>`/`<S-Tab>` completion of names and arguments (`:search` from its history); `tip: next time press …`; and the history, saved in the state directory ([#41](https://github.com/anopheles/navigo/issues/41)).
- **Playlists:** the Playlists list and playlist tracks with `'p`; removals (`dd`, `x`, `d{motion}`, Visual `d`) after a y/n Confirmation, into the register; `p`/`P` and `J`/`K` on the server; `dd` on the list; `:playlist-add`, `:playlist-new`; no undo; edits refused offline ([#37](https://github.com/anopheles/navigo/issues/37)).
- **Playback extras:** `=`/`+`/`-` volume (kept for mpv started later), `S` shuffle keeping the current track first, `R` repeat off/all/one driving what plays next and shown in the now-playing bar, the `i`/`:info` popup, and `*` stars, refused offline ([#39](https://github.com/anopheles/navigo/issues/39)).
- **Messages:** the three tiers (status-line messages with expiry and `:messages`, indicator slots, popups), the log path on unexpected errors, and crash handling ([#29](https://github.com/anopheles/navigo/issues/29)).
- **Tests:** layer A, and the three layer B scenarios (the new-user path from search to playing, transport, cold start from the cache) on the fixture library and live ([#45](https://github.com/anopheles/navigo/issues/45)).

Still to build for v1:

- **Sections:** Home (and starting on it), and Help. The skeleton starts on Albums.
- **Keys:** marks other than `'a`/`'b`/`'p`/`'q`/`'s`, `/` with `n`/`N`, the jump list, the remaining motions, and `gp`.
- **Discoverability:** the key menu, prefix menus, and the `/`-to-search offer (with `/` find).
- **Rebinding:** the `keys:` block, its validation popup, `:map` and friends, `:reload-config` and `navigo check-config`.
- **Crash cap:** the 3-crash cap for mpv (with the mpv slice).
- **mpv:** the startup check with install hints, `player.mpv_path`, and resuming after a crash.
- **Media controls:** MPRIS on Linux, and `force-media-title` for macOS and Windows.
- **Colours:** the 16-colour palette and `NO_COLOR`; the skeleton still uses fixed 256-colour values.
- **`:logout`.**
