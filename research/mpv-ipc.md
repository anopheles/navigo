# Driving mpv over JSON IPC from Go (Linux, macOS, Windows)

Ticket: anopheles/navigo#4. Researched 2026-10-19.

## Answer in brief

- **Write a thin IPC client of our own (about 300 lines).** Use `net.Dialer.DialContext("unix", path)` on Linux and macOS, and `winio.DialPipeContext` from `github.com/Microsoft/go-winio` **v0.6.4 (released 2026-10-13)** on Windows. That is the only new dependency. A test program cross-compiles with `CGO_ENABLED=0` for linux/amd64, darwin/amd64, darwin/arm64 and windows/amd64 [XBUILD].
- **Don't use the existing Go libraries.** `dexterlb/mpvipc` has no tags (latest pseudo-version `v0.0.0-20260722094525-0cf47d745b36`, 2026-07-22). It races when two goroutines send at once, a pending `Call` hangs forever if mpv dies, and on Windows it uses `natefinch/npipe.v2`, which has seen no changes since 2016. `blang/mpv` (last commit 2016-08-10) works only on unix, never implemented events, and spins in a busy loop on EOF [SRC].
- **Spawn mpv with:** `--idle=yes --no-terminal --no-video --audio-display=no --input-ipc-server=<path>`, plus `--no-config --load-scripts=no --ytdl=no` so user configs and scripts don't change behaviour. Pick a unique path for each run: `<os.MkdirTemp dir, 0700>/mpv.sock` on unix and `\\.\pipe\navigo-mpv-<pid>-<rand>` on Windows.
- **Observe these properties:** `pause`, `time-pos`, `duration`, `volume`, `playlist-pos`, `playlist-count`, `idle-active`, `core-idle` (for buffering). Handle these events: `start-file` and `end-file` (they carry `playlist_entry_id`, and `end-file` carries `reason`), plus `file-loaded`, `seek`, `playback-restart`, `idle` and `shutdown`. `eof-reached` is useless without `--keep-open`, so use `end-file reason=eof` instead.
- **The queue lives in the app.** Mirror a short window into mpv's playlist: the current track plus the next one. That keeps gapless playback and `--prefetch-playlist` working, while the app keeps control of shuffle, reordering, `dd`/`p` and repeat. Map mpv's `playlist_entry_id` (returned by `loadfile`) to our track ID.
- **Stream URLs:** send them over IPC with `loadfile`, never in mpv's argv, where `ps` would show them. Use Subsonic token auth (`t`/`s`) with a fresh salt for each URL; never `p=`. Don't enable mpv logging. Per-file `http-header-fields` options are **not** applied to prefetched entries [EXP], so put auth in the query string, not in headers.

## Sources

| Tag | Source |
|---|---|
| [IPC] | mpv manual, JSON IPC: https://github.com/mpv-player/mpv/blob/master/DOCS/man/ipc.rst (also https://mpv.io/manual/master/#json-ipc) |
| [OPT] | mpv manual, options: https://github.com/mpv-player/mpv/blob/master/DOCS/man/options.rst |
| [INPUT] | mpv manual, commands, events and properties: https://github.com/mpv-player/mpv/blob/master/DOCS/man/input.rst |
| [MPV-SRC] | mpv source `input/ipc-unix.c` and `input/ipc-win.c` on `master`, fetched 2026-10-19 |
| [SRC] | Source of `github.com/dexterlb/mpvipc` (HEAD 0cf47d7) and `github.com/blang/mpv` (HEAD d56d735) |
| [GO] | `proxy.golang.org/<module>/@latest`, the GitHub API (repo metadata, releases) |
| [EXP] | Throwaway Go experiment against local **mpv v0.41.0** on Linux: spawn, raw socket, observe, loadfile from a local HTTP server that logs headers |
| [XBUILD] | Throwaway module: `go get github.com/Microsoft/go-winio@latest` (pulled v0.6.4 and golang.org/x/sys v0.49.0), built with `CGO_ENABLED=0` for 4 GOOS/GOARCH pairs |

## 1. Transport per OS

**Server flag.** `--input-ipc-server=<path>` makes mpv listen [OPT]:
- **Linux/macOS:** a unix domain stream socket at that filesystem path. A leading `@` gives a Linux abstract socket, but the result is "unspecified" on other unices, so don't use it [OPT]. mpv `unlink()`s any existing file at the path before `bind()` [MPV-SRC ipc-unix.c]. The path has to fit in `sun_path`, which holds about 108 bytes on Linux and 104 on macOS; mpv fails with "Could not create IPC socket" if it doesn't [MPV-SRC]. `os.MkdirTemp("", "navigo-mpv-")` keeps the path short on both (macOS `$TMPDIR` is roughly 50 chars).
- **Windows:** a named pipe. If the path lacks the `\\.\pipe\` prefix, mpv adds it [OPT][MPV-SRC ipc-win.c]. mpv creates the pipe with `FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_REJECT_REMOTE_CLIENTS` and overlapped I/O, in message type with byte read mode, so byte-mode clients work [MPV-SRC]. The ACL grants GENERIC_READ|WRITE only to the current user's SID at the current or higher integrity level [MPV-SRC `create_restricted_sd`]. `FIRST_PIPE_INSTANCE` means a second mpv started with the same name fails, so every run needs a unique name.
- Go's `net.Dial("unix")` also works on Windows 10 and later, but mpv uses named pipes there, so we still need a pipe dialer.

**Pipe dialer.** `winio.DialPipeContext(ctx, path) (net.Conn, error)` "attempts to connect to a named pipe by `path` until `ctx` cancellation or timeout" [XBUILD `go doc`]. The import path is `github.com/Microsoft/go-winio`. The latest release is v0.6.4, tagged 2026-10-13 [GO]. The repo has 1084 stars, MIT licence, and commits within the last week [GO]. It is pure Go over `golang.org/x/sys/windows`, so it needs no cgo [XBUILD].

**Suggested shape** (build-tagged files):

```go
// ipc_unix.go   //go:build !windows
func dialIPC(ctx context.Context, p string) (net.Conn, error) { var d net.Dialer; return d.DialContext(ctx, "unix", p) }
// ipc_windows.go //go:build windows
func dialIPC(ctx context.Context, p string) (net.Conn, error) { return winio.DialPipeContext(ctx, p) }
```

**Startup race.** The socket appears a little after `exec.Cmd.Start()` (about 80 ms locally [EXP]). Retry the dial with a short backoff under a context deadline of a few seconds. Abort early if the process exits.

**Alternative: `--input-ipc-client=fd://N`.** This skips the socket and hands mpv a `socketpair()` FD. mpv quits when the connection closes, which is a nice lifetime coupling. On Windows it needs an inheritable overlapped duplex pipe handle passed as `handle://N` [OPT]. That works with `os/exec` `ExtraFiles` on unix but needs more work on Windows, so v1 should use `--input-ipc-server` on every OS and keep one code path.

**Security.** The protocol is "explicitly insecure": there is no auth, and the `run` command executes arbitrary programs [IPC]. mpv `fchmod`s the socket to 0600 (we observed `srw-------` [EXP]), and we also put it in a 0700 temp dir. On Windows the pipe ACL is limited to the user [MPV-SRC].

## 2. Protocol facts a client must handle

From [IPC] unless tagged, and confirmed in [EXP]:
- Messages are UTF-8 JSON, one per line, and **must** end with `\n`. Send minified JSON (`encoding/json.Marshal` output contains no raw newlines). Write each message with **one** `Write` call holding the payload plus `\n`, and allow only one writer at a time (a mutex or a writer goroutine). mpvipc does two `Write`s without a lock, which is how it can interleave [SRC].
- Request: `{"command":["name",args...],"request_id":N}`. The reply is `{"request_id":N,"error":"success","data":...}`. If `request_id` is omitted, the reply carries `request_id: 0` [EXP]. Use int64 IDs; non-integer IDs are deprecated.
- **Ordering:** "The only guarantee is that replies to IPC messages are sent in sequence." Events can arrive between a request and its reply, so match replies by `request_id`. Tell lines apart by key: a line with `event` is an event, otherwise it is a reply.
- Error strings seen [EXP]: `"property unavailable"` (e.g. `duration` while idle) and `"invalid parameter"` (unknown command). Treat any value other than `"success"` as an error.
- A line that doesn't start with `{` is an input.conf text command and gets **no reply** [IPC][EXP]. Never send those, or a pending request would wait forever.
- `async: true` replies only when the command completes. We don't need it.
- `observe_property <id> <name>` sends the current value immediately, then one `property-change` event per change: `{"event":"property-change","id":5,"name":"volume","data":55.0}`. **`data` is missing** when the property is unavailable [EXP], e.g. `duration` between files. Decode it as `json.RawMessage` or a pointer and treat a missing value as "unknown". Observations die with the connection, so keep it open.
- Numbers are JSON floats (`100.000000`), even for integer-ish properties like `volume`. `playlist-pos` arrives as an integer [EXP].
- mpv can emit **invalid UTF-8** from broken tags or filenames [IPC]. Go's `encoding/json` replaces invalid UTF-8 with U+FFFD rather than failing, so no special filter is needed.
- Line size: a full `playlist` reply can be large. Use `bufio.Reader.ReadBytes('\n')`, or a `bufio.Scanner` with a raised `Buffer` limit (mpvipc moved to 32 MB in 2026-07 for this reason [SRC]).
- On EOF or a read error, fail every pending request with an error and close the event channel. mpvipc doesn't do this, so its `Call` blocks forever [SRC].

## 3. Properties, commands and events for a player

Observed sequence for "load A, append B, both play out" [EXP, trimmed]:

```
loadfile A replace  -> {"data":{"playlist_entry_id":1},"error":"success"}
loadfile B append   -> {"data":{"playlist_entry_id":2},"error":"success"}
start-file {playlist_entry_id:1}; idle-active=false; file-loaded; duration=2.0; playback-restart; core-idle=false
... end-file {reason:"eof", playlist_entry_id:1}; duration=<unavailable>
start-file {playlist_entry_id:2}; playlist-pos=1; file-loaded; duration=2.0; playback-restart
... end-file {reason:"eof", playlist_entry_id:2}; idle; playlist-pos=-1; idle-active=true
```

| Need | Use | Notes |
|---|---|---|
| play/pause | `set_property pause bool` / observe `pause` | |
| position | observe `time-pos` | Updates often during playback. Throttle UI redraws. |
| length | observe `duration` | Unavailable between files [INPUT][EXP]. |
| volume | `set_property volume N` / observe `volume` | Float 0-100 (above 100 allowed up to `--volume-max`). |
| seek | `seek <sec> absolute` or `relative` | Emits `seek` then `playback-restart`. |
| buffering | observe `core-idle` | True while paused, restarting, or starved by a slow network cache, even when `pause` is false [INPUT]. |
| track change | events `start-file`, `end-file` | `end-file.reason` is one of eof, stop, quit, error, redirect, unknown. On error, `file_error` gives the reason [INPUT]. |
| nothing playing | observe `idle-active`, event `idle` | |
| which entry | `playlist_entry_id` from `loadfile`'s reply and the start/end events | Stable IDs, safer than indexes. `playlist-pos` is -1 when idle [INPUT]. |
| end of queue | `end-file reason=eof` with no next entry, then idle | `eof-reached` is only meaningful with `--keep-open` [INPUT]. |
| stop | `stop` (or `playlist-play-index none`) | |
| quit | `quit` | Replies, then mpv exits 0 [EXP]. Kill it if it hasn't exited after a timeout. |

**`loadfile` version pitfalls** [INPUT]. The signature is `loadfile <url> [<flags> [<index> [<options>]]]`. The `index` argument was added in **0.38.0**, which moved the position of `options`. The `play` flag (`append+play`) arrived in **0.42**; before that, the form is `append-play`, which is now deprecated. Distros ship older mpv, so:
- Use the **named-argument form**, which doesn't depend on argument positions: `{"command":{"name":"loadfile","url":U,"flags":"append"}}` (verified on 0.41 [EXP]).
- Use the plain `replace`/`append` flags, and start playback explicitly with `replace` or `playlist-play-index`, rather than relying on `append-play`/`+play`.
- Read `mpv-version` at connect time, log it, and refuse versions that are too old (the minimum is still to be decided).

## 4. Queue: mpv playlist or app-managed?

What the mpv options give us [OPT]:
- `--gapless-audio=weak` (the default) keeps the audio device open across consecutive playlist entries when formats match. This only works when the next file is **already in mpv's playlist**. A `loadfile replace` issued from `end-file` is not gapless.
- `--prefetch-playlist=yes` (default **no**) opens the next entry's URL once the current one is fully read. It "can't predict whether you go backwards in the playlist, and assumes you won't edit the playlist."

**Recommendation: the app owns the queue; mpv holds a sliding window of current plus next.**
- The domain `Queue` (ordering, shuffle, repeat, visual-mode `dd`/`p`, persistence, and the Subsonic `savePlayQueue` later) stays in Go. That keeps mpv out of the domain model and makes the queue testable without mpv.
- When track *i* starts (`start-file` with that entry's ID), `loadfile <url(i+1)> append` and `playlist-remove` entries before the current one. This gives gapless playback and prefetch while mpv never holds more than two entries.
- When the user edits the queue near the current track (next track removed or reordered), replace the window: `playlist-remove` the stale next entry and append the new one. Jumping to an arbitrary track uses `loadfile replace` and rebuilds the window.
- Mirroring the whole queue into mpv was rejected. Every queue edit would need index-based `playlist-move`/`playlist-remove` bookkeeping against two sources of truth, and the prefetch caveat above applies.
- Calling `loadfile replace` from `end-file` for every track was rejected because it loses gapless playback and prefetch.

## 5. Passing authenticated stream URLs safely

- **No argv.** Process arguments are visible to other local users via `ps` or `/proc/<pid>/cmdline`. Start mpv with no file and send every URL with `loadfile` over the user-only socket or pipe.
- **Token auth in the query string.** Use `u`, `t=md5(password+salt)` and `s`, with a fresh salt per URL (see `research/subsonic-api.md`). Never use `p=`. The token+salt pair can be replayed, but it doesn't reveal the password.
- **Headers aren't a substitute with prefetch.** We loaded A with per-file option `http-header-fields: X-Navigo-Test: one` and B with `two`. With `--prefetch-playlist=yes` the server saw `one` on **both** requests. With prefetch off, each got its own value [EXP]. So per-file header options don't survive prefetching. A global `--http-header-fields` would work, but query-string auth is what Subsonic expects anyway.
- **mpv echoes URLs.** The `playlist`, `path` and `filename` properties return the full URL to any IPC client [EXP], and mpv log output includes it. Don't pass `--log-file`, don't call `request_log_messages`, and redact `t`/`s` from anything navigo itself logs.
- `--ytdl=no` stops the youtube-dl hook from ever receiving our URLs. `--no-config --load-scripts=no` stops user scripts from seeing them (and from changing behaviour). If users need mpv tuning (audio device and similar), offer a navigo YAML passthrough list of mpv flags rather than loading `mpv.conf`.

## 6. Process lifecycle notes

- `--no-terminal` is essential. Without it mpv initialises the TTY and reads keys from stdin, which fights the TUI [OPT]. Also leave `cmd.Stdin` nil and send stdout/stderr to `io.Discard` (or to a ring buffer for diagnostics).
- `--idle=yes` keeps mpv alive with an empty playlist, so one process lasts the whole session.
- If mpv dies, the read loop gets EOF. Fail pending calls, surface "player stopped" to the UI, and respawn lazily on the next play. Re-sending `observe_property` after reconnecting is required.
- On exit, send `quit`, wait about 1 s, then `Process.Kill()`. On unix, remove the temp dir. The Windows pipe disappears with the process.

## Open questions

- The minimum supported mpv version for v1 (0.38 for named `loadfile` args with `index` semantics? Older versions should work with the named form, but that is unverified).
- Windows and macOS were checked by docs, mpv source and cross-compiling only. Run an end-to-end test on real Windows and macOS hosts when the skeleton exists.
