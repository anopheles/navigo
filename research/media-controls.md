# OS media controls from Go without cgo

Research for [#5](https://github.com/anopheles/navigo/issues/5). Checked 2026-10-19.

## Answer

**v1: implement MPRIS ourselves on Linux with `godbus/dbus/v5` (pure Go). On macOS and Windows, use the media integration already built into mpv, which navigo spawns anyway. Put all of this behind a `MediaControls` interface, with build-tagged files per OS.**

- **Linux / MPRIS:** pure Go, no cgo, no helper. D-Bus is a wire protocol and godbus implements it natively. mpv has no MPRIS support of its own, so navigo has to provide it.
- **Windows / SMTC:** mpv (0.39.0 and later) already drives `SystemMediaTransportControls` itself (`--media-controls=yes` is the default). Doing SMTC in pure Go is possible (go-ole + a hidden window), but there is no Go wrapper for the producer-side API, and the work is large.
- **macOS / Now Playing:** mpv already drives `MPNowPlayingInfoCenter` and `MPRemoteCommandCenter` (enabled by `--input-media-keys=yes`, the default) when it is built with the `macos-media-player` feature. Doing it in pure Go is possible with purego's `objc` package (it supports blocks), but no Go wrapper exists. There is also an unverified risk that macOS won't treat navigo as the "now playing" app, because the audio comes from the mpv process.
- So it is neither "full native support everywhere" nor "MPRIS plus empty stubs". It is **MPRIS native, plus macOS/Windows delegated to mpv**. That gives working media keys and now-playing info on all three OSes in v1, with no cgo.

## Per platform

### Linux: MPRIS over D-Bus (pure Go)

What the spec requires ([MPRIS 2.2](https://specifications.freedesktop.org/mpris/latest/)):

- Request a bus name starting with `org.mpris.MediaPlayer2.` on the session bus, e.g. `org.mpris.MediaPlayer2.navigo`. A second instance appends `.instance<id>`. The id may use only ASCII letters, digits, `_` and `-`, and must not start with a digit.
- Export the object `/org/mpris/MediaPlayer2` with two required interfaces, `org.mpris.MediaPlayer2` and `org.mpris.MediaPlayer2.Player`. `TrackList` and `Playlists` are optional; skip them in v1.
- Player methods ([Player interface](https://specifications.freedesktop.org/mpris/latest/Player_Interface.html)): `Next`, `Previous`, `Pause`, `PlayPause`, `Stop`, `Play`, `Seek(x µs)`, `SetPosition(o trackid, x µs)`, `OpenUri(s)`.
- Properties: `PlaybackStatus`, `LoopStatus`, `Rate`, `Shuffle`, `Metadata (a{sv})`, `Volume`, `Position`, `Min/MaximumRate`, `CanGoNext/Previous`, `CanPlay`, `CanPause`, `CanSeek`, `CanControl`.
  - All of them emit `org.freedesktop.DBus.Properties.PropertiesChanged`, except `Position` and `CanControl`.
  - Position jumps are reported with the separate `Seeked(x)` signal.
- `Metadata` must include `mpris:trackid` (type `o`, a valid D-Bus object path, not under `/org/mpris`) whenever a track is current. Use something like `/navigo/track/<escaped subsonic id>`. `mpris:length` (µs) bounds `Position`. Common xesam keys: `xesam:title`, `xesam:artist` (as), `xesam:album`, and `mpris:artUrl` for a cover URL.

Library:

- **`github.com/godbus/dbus/v5`**: latest **v5.2.4, released 2026-10-13** (from [proxy.golang.org](https://proxy.golang.org/github.com/godbus/dbus/v5/@latest); repo pushed 2026-10-13, about 1.2k stars, BSD-2-Clause, not archived). The README describes it as a "complete native implementation of the D-Bus message protocol" ([README](https://github.com/godbus/dbus)).
  - Its `prop` subpackage exports a property table and emits `PropertiesChanged` for you.
  - Its `introspect` subpackage covers `Introspectable`.
  - Caveat: the README still says the "API is considered unstable", though v5 has been the stable import path for years.
- **`github.com/quarckster/go-mpris-server`**: latest **v1.2.0, released 2026-08-16**. A ready-made MPRIS server on top of godbus, but it is **GPL-3.0**, has 6 stars, and has a single maintainer. **Not recommended:** the licence would force GPL on navigo, and the MPRIS surface is small enough (about 300 lines) to write ourselves.
- `Pauloo27/go-mpris` and `leberKleber/go-mpris` are MPRIS *clients* (they control other players). They don't fit this use.

Effort: small. Map mpv IPC events (`pause`, `time-pos`, `duration`, `playlist-pos`, end-file) onto the property table. Map MPRIS method calls onto navigo player commands.

### Windows: SystemMediaTransportControls

Platform facts:

- A non-UWP desktop process gets SMTC through `ISystemMediaTransportControlsInterop::GetForWindow(HWND, riid, out)`. `appWindow` "must refer to a top-level window that belongs to the calling process" ([Microsoft Learn](https://learn.microsoft.com/en-us/windows/win32/api/systemmediatransportcontrolsinterop/nf-systemmediatransportcontrolsinterop-isystemmediatransportcontrolsinterop-getforwindow)). A console TUI has no window of its own, so it has to create a hidden one and pump its messages.
- The manual-control steps ([Microsoft Learn](https://learn.microsoft.com/en-us/windows/apps/develop/media-playback/system-media-transport-controls)):
  - set `IsPlayEnabled` / `IsPauseEnabled` / `IsNextEnabled` / `IsPreviousEnabled`;
  - handle the `ButtonPressed` event, which is not raised on the UI thread;
  - set `PlaybackStatus`;
  - fill `DisplayUpdater` (`Type = Music`, `MusicProperties.Title/Artist/AlbumArtist`, `Thumbnail`), then call `Update()`;
  - push `UpdateTimelineProperties` about every 5 s.
- mpv's own implementation follows exactly this pattern. It creates a dummy window "to allow SMTC to work also in audio only mode", then calls `GetForWindow` ([osdep/win32/smtc.cpp](https://github.com/mpv-player/mpv/blob/master/osdep/win32/smtc.cpp)).

Pure Go? **Possible, but there's no ready-made wrapper.**

- **`github.com/go-ole/go-ole`**: latest tagged **v1.3.0, released 2023-08-04** (proxy). Untagged commits continue; the last was 2026-10-12. MIT. It provides `RoInitialize`, `RoGetActivationFactory`, `RoActivateInstance` and `HString` ([winrt.go](https://github.com/go-ole/go-ole/blob/master/winrt.go)). That is enough to activate WinRT classes, but each interface's vtable has to be written by hand.
- **`github.com/saltosystems/winrt-go`**: no tags. Latest pseudo-version is **v0.0.0-20261016124014-b298124d7472 (2026-10-16)**. MIT, actively maintained. It is a generated WinRT projection plus generator, and the README says it targets Bluetooth APIs. It contains only the *consumer* `Windows.Media.Control.GlobalSystemMediaTransportControlsSession*` types, which control other apps. It does **not** contain `Windows.Media.SystemMediaTransportControls`, the producer API we need. Its generator could probably produce it; the README says some features are missing. Its generated `TypedEventHandler` delegate code is the hard part we'd need for `ButtonPressed`.
- `zandercodes/gowinrt`: a WinRT generator, GPL-3.0, 0 stars. Not a candidate.
- Hidden window and message loop: `golang.org/x/sys/windows` (latest **v0.49.0, released 2026-10-06**) plus `user32` procs, on a goroutine pinned with `runtime.LockOSThread`.

**mpv already does this.** `--media-controls=<yes|no>` (Windows only): "Enable integration of media control interface SystemMediaTransportControls". The default is yes, except for libmpv. It was added in mpv 0.39.0 ([options.rst](https://github.com/mpv-player/mpv/blob/master/DOCS/man/options.rst), [interface-changes.rst](https://github.com/mpv-player/mpv/blob/master/DOCS/interface-changes.rst)). From [smtc.cpp](https://github.com/mpv-player/mpv/blob/master/osdep/win32/smtc.cpp):

- Title comes from `media-title`, so navigo can set it per file with `force-media-title`.
- Artist, album, album artist and track come from the file's own tags (`metadata/by-key/...`), which Navidrome's stream normally carries.
- Play and Pause set mpv's `pause` property.
- **Next/Previous run mpv's `playlist-next` / `playlist-prev`**. So navigo must keep the next and previous tracks loaded in mpv's internal playlist (appending the next track also helps gapless playback), and follow `playlist-pos` changes back into its own queue.
- Caveat from the manual: Windows may show "Unknown app" or no icon unless mpv is registered (`mpv --register`).
- Pass `--media-controls=yes` explicitly when spawning mpv.

### macOS: MPNowPlayingInfoCenter / MPRemoteCommandCenter

Platform facts (Apple docs, MediaPlayer framework):

- `MPNowPlayingInfoCenter`: available on macOS 10.12.2 and later. It holds a `nowPlayingInfo` dictionary for system Now Playing surfaces ([docs](https://developer.apple.com/documentation/mediaplayer/mpnowplayinginfocenter)).
- `playbackState`: "This property only applies to macOS. You must set this property every time the app begins or halts playback, otherwise remote control functionality may not work as expected." ([docs](https://developer.apple.com/documentation/mediaplayer/mpnowplayinginfocenter/playbackstate))
- `MPRemoteCommandCenter.shared()`: its command objects (play, pause, togglePlayPause, nextTrack, previousTrack, changePlaybackPosition, …) take handlers through `addTarget(handler:)`, which "adds a block to be called when an event is received" ([docs](https://developer.apple.com/documentation/mediaplayer/mpremotecommandcenter), [addTarget](https://developer.apple.com/documentation/mediaplayer/mpremotecommand/addtarget(handler:))). **Handlers are Objective-C blocks.**

Pure Go? **Possible, but there's no ready-made wrapper.**

- **`github.com/ebitengine/purego`**: latest **v0.11.2, released 2026-10-15** (proxy; the GitHub release page shows 2026-10-18). Apache-2.0, about 4k stars, actively maintained, macOS amd64/arm64 are Tier 1.
  - It can `Dlopen` `/System/Library/Frameworks/MediaPlayer.framework` with `CGO_ENABLED=0`.
  - Its `objc` package wraps the Objective-C runtime and implements blocks to Clang's Block ABI (`objc/objc_block_darwin.go`: `NewBlock`, `Block.Copy/Release`), which is what `addTarget(handler:)` needs.
  - Caveats: the README says "This is beta software so expect bugs and potentially API breaking changes". Remote-command delivery generally needs a running main-thread run loop (CFRunLoop), which conflicts with a Bubble Tea style main loop and would need `runtime.LockOSThread` on the main goroutine (inference, not verified).
- **`github.com/progrium/darwinkit`**: latest **v0.5.0, released 2024-07-11**, last push 2025-03-08. It uses cgo, and is stale. Not a candidate.
- **Unverified risk:** macOS may decide which app is "Now Playing" by which process is producing audio. That process is mpv, not navigo. A navigo-side implementation might never be selected. This needs a spike on real hardware before committing to it.

**mpv already does this.** In builds with the `macos-media-player` meson feature (needs Swift), mpv's [remote_command_center.swift](https://github.com/mpv-player/mpv/blob/master/osdep/mac/remote_command_center.swift):

- registers play, pause, togglePlayPause, stop, nextTrack, previousTrack, seek and changePlaybackPosition;
- fills `MPNowPlayingInfoCenter` from `media-title`, `metadata/by-key/artist` and `metadata/by-key/album`;
- is started through `cocoa_init_media_keys` when `--input-media-keys=yes` (the default, "macOS and Windows only").

The commands come in as mpv input keys (`MP_KEY_NEXT`, `MP_KEY_PREV`, …). By default these map to `playlist-next/prev`, and navigo can rebind them over IPC with the `keybind` command (e.g. `keybind NEXT "script-message navigo next"`). Caveat: this depends on the user's mpv build. Homebrew and official builds normally include Swift, but navigo can't guarantee it. Log a hint if the controls don't show up.

## Recommended v1 shape

```go
// internal/mediactl
type Controls interface {
    Update(state PlayerState) // track metadata, status, position, can-next/prev
    Commands() <-chan Command // Play, Pause, Toggle, Next, Prev, Stop, SeekTo
    Close() error
}
```

- `mediactl_linux.go`: godbus MPRIS server (hand-written, `prop` + `introspect` subpackages).
- `mediactl_darwin.go`, `mediactl_windows.go`: no-op `Controls`. mpv itself provides the integration. The player layer:
  - spawns mpv with `--media-controls=yes` (Windows) and `--input-media-keys=yes` (macOS);
  - sets `force-media-title` per file;
  - keeps adjacent queue entries in mpv's playlist, or rebinds `NEXT`/`PREV` via `keybind`, so OS Next/Previous move navigo's queue.
- `mediactl_other.go` (`//go:build !linux && !darwin && !windows`): no-op stub.
- Later, if mpv's integration proves too limited (no cover art on macOS, "Unknown app" on Windows), the darwin/windows files can be swapped for native purego / go-ole implementations behind the same interface. No caller changes needed.

## Sources

- MPRIS spec 2.2: https://specifications.freedesktop.org/mpris/latest/ and https://specifications.freedesktop.org/mpris/latest/Player_Interface.html
- godbus: https://github.com/godbus/dbus, https://proxy.golang.org/github.com/godbus/dbus/v5/@latest
- go-mpris-server: https://github.com/quarckster/go-mpris-server
- Microsoft SMTC: https://learn.microsoft.com/en-us/windows/apps/develop/media-playback/system-media-transport-controls, https://learn.microsoft.com/en-us/windows/apps/develop/media-playback/integrate-with-systemmediatransportcontrols, https://learn.microsoft.com/en-us/windows/win32/api/systemmediatransportcontrolsinterop/nf-systemmediatransportcontrolsinterop-isystemmediatransportcontrolsinterop-getforwindow
- go-ole: https://github.com/go-ole/go-ole (winrt.go); winrt-go: https://github.com/saltosystems/winrt-go
- Apple MediaPlayer: https://developer.apple.com/documentation/mediaplayer/mpnowplayinginfocenter, .../playbackstate, https://developer.apple.com/documentation/mediaplayer/mpremotecommandcenter
- purego: https://github.com/ebitengine/purego (README, objc/objc_block_darwin.go); darwinkit: https://github.com/progrium/darwinkit
- mpv: DOCS/man/options.rst (`--media-controls`, `--input-media-keys`, `--force-media-title`), DOCS/interface-changes.rst, osdep/win32/smtc.cpp, osdep/mac/remote_command_center.swift, osdep/mac/app_bridge.m, meson.build. All at https://github.com/mpv-player/mpv (latest release v0.41.0, 2025-12-21).
- Version and maintenance data: GitHub REST API (`repos/*`, `releases/latest`) and proxy.golang.org `@latest`, queried 2026-10-19.
