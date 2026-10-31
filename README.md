# navigo

A keyboard-driven terminal client for [Navidrome](https://www.navidrome.org/)
and other Subsonic servers, with vim keys. The queue is edited like a vim
buffer (`dd`, `yy`, `p`, `V`, `u`), the library is cached locally so
browsing and search work at once and offline, and playback goes through
mpv, gaplessly.

![navigo browsing albums, playing a track with its cover, seeking and finding](docs/media/demo.gif)

*Recorded against a made-up library, with `make demo`; the covers are generated.*

## What you need

- A Navidrome server, or another Subsonic server (API 1.16.1). navigo is
  tested against Navidrome.
- [mpv](https://mpv.io) 0.38 or newer, on your `PATH` or set as
  `player.mpv_path`. navigo starts it itself. Without it you can still
  browse, and navigo tells you how to install it.
- Linux, macOS or Windows, on amd64 or arm64.

## Install

On macOS, with [Homebrew](https://brew.sh) (this also installs mpv):

```sh
brew install anopheles/tap/navigo
```

Otherwise, download the archive for your OS and CPU from
[Releases](https://github.com/anopheles/navigo/releases), unpack it and
put `navigo` on your `PATH`. On macOS, the binary isn't notarized: run
`xattr -d com.apple.quarantine navigo` once after unpacking.

Or, with Go 1.26 or newer:

```sh
go install github.com/anopheles/navigo/cmd/navigo@latest
```

`navigo --version` says which version you have.

## First run

Run `navigo`. The first time, a form asks for your server's URL,
username and password. It checks them against the server and saves the
password in your OS keyring, never in a file unless you say so. Then the
library syncs once; Home's lists work meanwhile, and a Mix asked for
then starts when the sync is done. After that it opens at once from the local cache and
checks the server in the background.

navigo starts on Home: recently added, most and recently played, random
and starred albums.

## Keys

Press `?` for the keys that work where the cursor is, and `'?` (or
`<F1>`) for all of them. When you type a command after `:`, navigo lists
the matching commands with their keys, and next time tells you the key.

| | |
|---|---|
| Move | `j` `k`, `gg` `G`, `<C-d>` `<C-u>`, `/` to find in a list, `n` `N` |
| Open, go back | `l` or `<CR>`, `h`, `<C-o>` back to where you were |
| Sections | `'h` Home, `'a` Artists, `'b` Albums, `'p` Playlists, `'s` Search, `'q` the queue |
| Play | `<CR>` plays from here, `<Space>` play/pause, `>` `<` next/previous, `.` `,` seek |
| Queue | `A` add to the end, `a` play next, `V` select, `dd` remove, `yy` copy, `p` paste, `J` `K` move, `u` undo |
| Other | `S` shuffle, `R` repeat, `=` `-` volume, `*` star, `i` info, `ga` `gr` go to album/artist |
| Quit | `:q` |

## Configuration

Everything is optional. The config file is `navigo/config.yaml` in your
config directory (`~/.config` on Linux, `~/Library/Application Support`
on macOS, `%AppData%` on Windows), or wherever `--config` points.

```yaml
server:
  url: https://music.example.com
  user: ann
  password_command: pass show navidrome   # or nothing: the keyring

player:
  mpv_path: /opt/mpv/bin/mpv               # when mpv isn't on PATH

keys:
  leader: ','
  normal:
    global:
      '<C-n>': down
      '<leader>x': 'seek +60'
    queue:
      'D': queue-clear
```

Keys can be rebound per mode and per scope, with vim key notation.
`navigo check-config` checks the file and lists any problems with their
line and column. [docs/spec.md](docs/spec.md) has every setting.

## Where navigo keeps things

- **The library cache:** `navigo/` in your user cache directory. It holds
  metadata only, never audio, and the server always wins.
- **The log:** `navigo.log` next to the cache. Run with `--debug` (or
  `NAVIGO_DEBUG=1`) for more detail.
- **Search and `:` history:** `navigo/` in `$XDG_STATE_HOME` (or
  `~/.local/state`) on Linux, and in the cache directory elsewhere.

`NO_COLOR` turns colours off; navigo stays fully usable without them.

## Media keys

On Linux, navigo appears on the session bus as an MPRIS player, so media
keys, desktop widgets and `playerctl` work. On macOS and Windows, mpv's
own media integration does this (mpv 0.39 or newer).

## Summon it with a key

`navigo --search-on-focus` opens search whenever its terminal window
gains focus, with the last query selected. Show navigo with a key, type,
add the track with `A` (or `a` to play it next) and hide navigo again.
A prompt, popup, Visual selection or half-typed key is left alone. Under
tmux it needs `focus-events on`.

On Hyprland with the Lua config, this keeps navigo in a hidden workspace,
starts it with the session and toggles it with `SUPER+N`:

```lua
local cmd = "kitty --class navigo -e navigo --search-on-focus"

hl.window_rule({ name = "navigo", match = { class = "^(navigo)$" },
    float = true, size = "monitor_w*0.7 monitor_h*0.75", center = true })

hl.bind("SUPER + N", function()
    for _, w in ipairs(hl.get_windows()) do
        if w.class == "navigo" then
            return hl.dispatch(hl.dsp.workspace.toggle_special("music"))
        end
    end
    hl.exec_cmd(cmd, { workspace = "special:music" })
end)

hl.on("hyprland.start", function()
    hl.exec_cmd(cmd, { workspace = "special:music silent" })
end)
```

To keep the music going when the window is closed, run navigo in a tmux
session instead: `tmux new-session -A -s music navigo --search-on-focus`.

## Contributing

Contributions are welcome, from people and coding agents alike. Every
change has to come with evidence that it works: a test that fails
without it, the gate's output, and screens for anything visible.
[CONTRIBUTING.md](CONTRIBUTING.md) says what counts.

`docs/spec.md` is the specification, `GLOSSARY.md` the terms, and
`docs/adr/` the decisions. `tools/ci/local.sh` runs the same checks as CI;
`tools/e2e/run.sh` runs the end-to-end tests against a throwaway
Navidrome.

## Licence

[MIT](LICENSE).
