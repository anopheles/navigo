# navigo

A keyboard-driven terminal client for Navidrome/Subsonic music servers. These are the words navigo uses for its own concepts.

## Input

**Mode**:
One of Normal, Visual, Command-line, Search or Prompt; it decides which bindings apply to a key press.
_Avoid_: state, insert mode

**Command**:
A named action the user can type at the `:` line, such as `play-pause` or `seek +10`. Every key does exactly one command.
_Avoid_: action, shortcut

**Binding**:
A key sequence paired with a command line, within one scope.
_Avoid_: shortcut, keymap entry, mapping

**Key sequence**:
One or more keys typed in order that together match a binding, such as `gg` or `<C-d>`.
_Avoid_: combo, chord

**Pure prefix**:
A key sequence that only begins longer bindings and is not bound on its own, so navigo waits for the next key without a time limit.

**Ambiguous prefix**:
A key sequence that is bound on its own and also begins a longer binding; it resolves after the timeout.

**Motion**:
A command that moves the cursor through a list and can follow an operator, such as `j`, `G` or `}`.

**Operator**:
A command that acts on the rows a motion covers, or on the current row when doubled. In v1 the operators are `d` (remove) and `y` (yank).

**Count**:
A number typed before a motion or command that repeats or scales it, such as `5j` or `3.`.

**Scope**:
Where a binding applies: a popup, a kind of list, or the whole mode. Scopes form a chain from narrow to wide (a playlist's tracks, then any track list, then global), and the narrowest scope that matches wins.

**Leader**:
A user-configurable prefix key (`\` by default) that is free for the user's own bindings.

**Register**:
The single clipboard that `y` and `d` fill and `p`/`P` paste from, shared by every view.
_Avoid_: clipboard, buffer

**Confirmation**:
A y/n question asked before a destructive server change; its keys are fixed and cannot be rebound.

**Jump list**:
The history of where the user has navigated, walked back and forth with `<C-o>`/`<C-i>`.

## Lists

**Container list**:
A list of things that hold tracks (artists, albums, playlists); you go into a row with `l` or `<CR>` and come back out with `h`.

**Track list**:
A list of individual tracks, such as an album's or a playlist's tracks.
_Avoid_: song list

**Queue**:
The ordered tracks navigo will play, edited like a vim buffer.
_Avoid_: playlist (a playlist lives on the server), now playing list

**Queue entry**:
One row of the queue: a track placed there once. The same track queued twice is two queue entries.
_Avoid_: queue item, slot

**Shuffle**:
A one-shot command that reorders the queue at random; it can be undone like any other queue edit, and there is no hidden play order.
_Avoid_: shuffle mode, random play

**Mix**:
A one-shot command that replaces the queue with tracks picked at random from the whole library, leaning toward starred, well-rated tracks not played lately; it can be undone like any other queue edit, and nothing keeps refilling the queue.
_Avoid_: radio, autoplay, instant mix, song mix, track mix

**Repeat**:
What happens at the end of a track or the queue: off, all (start the queue again) or one (play the same queue entry again).
_Avoid_: loop

**Role**:
A meaning a colour carries in navigo (playing, key, error, selection); the user can recolour a role but not add or merge roles.
_Avoid_: colour slot

**Theme**:
A named set of colours for the four roles, picked with `theme:` and changed role by role with `colors:`. It colours only the roles, never the text or background, which stay the terminal's.
_Avoid_: preset, skin

**Saved queue**:
The queue, its current entry and position, the volume and the repeat mode, kept between runs per server and user so that Resume can bring them back.
_Avoid_: session, playback state

**Resume**:
Starting navigo with the saved queue loaded and paused where it stopped; nothing plays until the user asks.
_Avoid_: session restore, autoplay

**Group**:
A run of related rows in a list: consecutive tracks from the same album in a track list or the queue, or an artist's own albums and their Appears on albums on the artist's page; `{`/`}` jump between groups.

## Screen

**Window**:
One of the two side-by-side panes of the main screen: the library window, which shows one list at a time, and the queue window, which always shows the queue.
_Avoid_: pane, panel, split, tab

**Section**:
A top-level entry of the library window (Home, Artists, Albums, Playlists, Search, Help); together they form the section list.
_Avoid_: tab, view, screen, page

**Mark**:
A named place you jump to with `'` followed by a key, such as `'a` for Artists.
_Avoid_: bookmark, hotkey

**Now-playing bar**:
The two lines at the bottom of the screen showing the current track and its progress; it is never focused.
_Avoid_: status bar, player bar

**Popup**:
A box drawn over both windows for a short interruption, such as a Confirmation or track details.
_Avoid_: modal, dialog, overlay

**Message**:
A line in the status line about something that just happened; information clears itself after a few seconds, errors stay until replaced or dismissed, and every message is kept in the message history.
_Avoid_: toast, notification, alert

**Indicator**:
A short note in the now-playing bar about a state that lasts, such as being offline, syncing, or mpv missing; it shows for as long as the state does.
_Avoid_: badge, status, icon

**Hint line**:
The row above the now-playing bar that lists the keys that matter for the row under the cursor and the current mode, drawn from the bindings in effect; on by default.
_Avoid_: status bar, footer, cheat sheet

**Key menu**:
The popup `?` opens, listing the keys for what's under the cursor; pressing one of them runs it.
_Avoid_: help (Help is the section with every key), cheat sheet

**Prefix menu**:
The small popup that lists what can follow a pure prefix once it has waited 500 ms.
_Avoid_: which-key, hint popup

## Data

**Library**:
The music on the server: its artists, albums, tracks and playlists.
_Avoid_: collection, database

**Artist**:
Anyone credited as an artist on a track or as the album artist on an album; each artist the server lists for a track counts. Composers, conductors and other roles are not artists.
_Avoid_: performer, band, contributor

**Album artist**:
The artist an album as a whole is credited to, such as Various Artists for a compilation.
_Avoid_: main artist, album owner

**Appears on**:
An album where an artist has tracks but is not the album artist, such as a guest track on a compilation.
_Avoid_: featured on, guest albums, compilations

**Cache**:
navigo's local, disposable copy of the library's metadata, used to browse and search without waiting on the server; it never holds audio and the server always wins.
_Avoid_: database, index, offline mode, download

**Offline**:
When navigo can't reach the server: lists and search still work from the cache, while playback, writes and sync wait for it to come back. A server that answers with an error is not offline.
_Avoid_: offline mode, disconnected

**Sync**:
Bringing the cache up to date with the library.
_Avoid_: refresh, scan (a scan is the server reading its own files)

## Account

**Server**:
The one Navidrome/Subsonic server navigo is connected to, identified by its URL.
_Avoid_: host, instance, backend

**Credentials**:
The server URL, username and password navigo logs in with; the password is never shown or stored as a token.
_Avoid_: login, account, auth

**Login form**:
The full-screen form that asks for credentials and checks them against the server before saving; it opens on first run, when saved credentials are rejected, and on `:login`.
_Avoid_: setup wizard, onboarding, login screen
