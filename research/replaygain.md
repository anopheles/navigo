# What the server and mpv give navigo for ReplayGain

Ticket: anopheles/navigo#49. Researched 2026-10-20.

## Answer in brief

- **The server exposes gain and peak per song, not per album.** OpenSubsonic defines a `replayGain` object on `Child` (a song) with `trackGain`, `albumGain`, `trackPeak`, `albumPeak`, `baseGain` and `fallbackGain` [OS-RG][OS-CHILD]. Gains are in dB, peaks are linear and positive. It is not an extension: `getOpenSubsonicExtensions` has no entry for it, and Navidrome 0.64.2 doesn't list one [ND-EXT][EXP-ND]. `AlbumID3` has no such object [OS-ALBUM][EXP-ND].
- **Navidrome fills it from file tags only.** It maps `replaygain_{track,album}_{gain,peak}` and, for Opus, `R128_{TRACK,ALBUM}_GAIN` (Q7.8 divided by 256, plus 5 dB) [ND-MAP]. It sends only the four track/album fields, never `baseGain` or `fallbackGain` [ND-HELP]. Untagged songs get an empty object `{}`; Opus songs get gains but no peaks [EXP-ND]. I found no audio analysis in Navidrome and it doesn't change the audio; the only gain handling is in its web UI [ND-CONF].
- **mpv reads the tags from the stream itself and needs no help from the server.** It decodes `REPLAYGAIN_*`, `REPLAYGAIN_GAIN/PEAK` and `R128_*` tags, plus libavformat's ReplayGain side data [MPV-DEMUX][MPV-LAVF]. It worked for direct FLAC, MP3 and Opus over HTTP, and for chunked, non-seekable transcoded MP3 and Opus [EXP-MPV][EXP-ND]. **ADTS AAC has no tags, so a transcode to `aac` carries no gain** [ND-FF][EXP-ND].
- **`--replaygain=no|track|album`** (default `no`), **`--replaygain-preamp=<dB>`** (default 0), **`--replaygain-clip=yes|no`** (default `no`: lower the gain to `1/peak`) and **`--replaygain-fallback=<dB>`** (default 0 = off). The logic and docs are identical in v0.38.0 and v0.41.0 [MPV-AUDIO][MPV-OPT]. Surprise: the fallback also applies when `--replaygain=no`, even to tagged files [EXP-MPV].
- **The mode can be changed at runtime with `set_property`, with no restart and no gap** (tested for `replaygain` and `replaygain-preamp`; `-clip` and `-fallback` carry the same update flag [MPV-AUDIO]). No `end-file`, `start-file`, `file-loaded` or `playback-restart` event fired, `playlist-pos` stayed, and a two-track gapless queue came out as exactly 16.00 s with a 0 ms silent run. A change applies to the already-queued next track too [EXP-MPV]. The new gain is audible after mpv's output buffer drains (about 1 to 1.5 s with the `pcm` output used here; a real audio output buffers less).
- **The gain stacks with navigo's volume as a plain multiplier.** mpv's gain is `(volume/100)^3 × replaygain × 10^(volume-gain/20)`, and mute gives 0 [MPV-AUDIO]. navigo's volume stays in 0 to 100 [NAVIGO], so at 100 the ReplayGain factor is the whole gain, and it can exceed 1 (a boost) when a track's gain is positive. ReplayGain never changes the `volume` property.
- **navigo today starts mpv with `--no-config`**, so a user's `mpv.conf` is ignored, but `player.mpv_args` is appended after the base args, so `--replaygain=track` already works there [NAVIGO]. navigo's Subsonic client has no `replayGain` field yet [NAVIGO].

## Sources

| Tag | Source |
|---|---|
| [OS-RG] | OpenSubsonic docs, `content/en/docs/Responses/ReplayGain.md` (`opensubsonic/open-subsonic-api`, `main`, fetched 2026-10-20) |
| [OS-CHILD] | Same repo, `content/en/docs/Responses/Child.md` (field `replayGain`, listed among the OpenSubsonic additions; "if a server supports a field it must return it with an empty / default value") |
| [OS-ALBUM] | Same repo, `content/en/docs/Responses/AlbumID3.md` (no `replayGain` field) |
| [OS-STREAM] | Same repo, `content/en/docs/Endpoints/stream.md` (`format`, `maxBitRate`, `format=raw`) |
| [ND-EXT] | Navidrome `server/subsonic/opensubsonic.go` at tag v0.64.2 (commit 101145742f4762164202cb4858c9126258bdc463): the extension list |
| [ND-MAP] | Same tag: `model/tag.go`, `model/metadata/map_mediafile.go` (`mapGain`, R128 conversion), `model/mediafile.go`, `db/migrations/20260719005427_add_album_replaygain.go` |
| [ND-HELP] | Same tag: `server/subsonic/helpers.go` (`osChildFromMediaFile`; returns nothing for clients in `Subsonic.LegacyClients`, default `DSub`), `server/subsonic/responses/responses.go` (`ReplayGain` struct) |
| [ND-FF] | Same tag: `core/ffmpeg/ffmpeg.go` (`buildDynamicArgs`: `-map_metadata 0 -map_metadata 0:s:a:0`, and the comment that ADTS output cannot hold tags); `core/stream/legacy_client.go` (`ResolveRequest`). A search of `core/ffmpeg` and `core/stream` for `replaygain`, `volume` and `loudnorm` found nothing. |
| [ND-CONF] | Same tag: `conf/configuration.go` and `server/serve_index.go` (`EnableReplayGain` is only passed to the web UI config) |
| [MPV-OPT] | mpv manual, `DOCS/man/options.rst` at `master`, `v0.38.0` and `v0.41.0` (diff of the `--replaygain*` entries: identical) |
| [MPV-AUDIO] | mpv `player/audio.c` at `master`, `v0.38.0`, `v0.41.0`: `compute_replaygain`, `audio_get_gain` / `audio_update_volume`; `options/options.c` (`replaygain*` flagged `UPDATE_VOL`, fallback range -200 to 60) |
| [MPV-DEMUX] | mpv `demux/demux.c`: `decode_rgain`, `demux_update_replaygain` (identical in v0.38.0 and v0.41.0) |
| [MPV-LAVF] | mpv `demux/demux_lavf.c`: `export_replaygain` (libavformat `AV_PKT_DATA_REPLAYGAIN` side data) |
| [NAVIGO] | This repo at `4e207ff`: `internal/mpv/process.go` (`baseArgs`, `Options.Args`), `internal/app/playback.go` (volume clamp 0 to 100), `internal/player/player.go` (`SetVolume`), `internal/subsonic/subsonic.go` (`StreamURL` sends only `id` and auth) |
| [EXP-ND] | Experiment: `deluan/navidrome:0.64.2` (the image `tools/navidrome/compose.yaml` pins) on a four-song library generated with ffmpeg 8.0.1: FLAC and MP3 with full ReplayGain tags, Opus with R128 tags, FLAC with no tags. Queried `search3`, `getAlbum`, `getAlbumList2`, `getSong`, `getOpenSubsonicExtensions`; fetched `stream` raw and with `format=` mp3, opus, aac, flac and read the tags with ffprobe. Also the 547 songs recorded in `internal/subsonic/subsonictest/testdata` (from the same server version, synthetic untagged library): all have `"replayGain": {}`. |
| [EXP-MPV] | Experiment: local mpv v0.41.0, `--no-config --ao=pcm`, the output measured with ffmpeg `volumedetect` or by reading the PCM; a local HTTP server for direct files and for a close-delimited stream with no `Content-Length` and no ranges. Runtime tests drove mpv over JSON IPC and paced the PCM output through a FIFO read at real time. |

## 1. What the server sends

Real response from Navidrome 0.64.2 (`search3`, same library, abridged) [EXP-ND]:

| Song | Tags in the file | `replayGain` |
|---|---|---|
| FLAC, full tags | track -6.00 dB / 0.5, album -3.00 dB / 0.8 | `{trackGain:-6, albumGain:-3, trackPeak:0.5, albumPeak:0.8}` |
| MP3, full tags | track -2.00 dB / 0.5, album -3.00 dB / 0.8 | same four fields, `trackGain:-2` |
| Opus | `R128_TRACK_GAIN=-1280`, `R128_ALBUM_GAIN=-768` | `{trackGain:0, albumGain:2}` (-1280/256+5 and -768/256+5), no peaks |
| FLAC, no tags | none | `{}` |

- **Fields.** Per the spec all six are optional numbers and "if the data is not present the field must be omitted", but `replayGain` itself must always be on `Child` [OS-RG]. `trackPeak` and `albumPeak` must be positive. `baseGain` is an extra output gain (the spec's example is the Ogg Opus output gain) and `fallbackGain` is "an optional fallback gain that clients should apply when the corresponding gain value is missing" [OS-RG]. Navidrome's `ReplayGain` struct has both, but `osChildFromMediaFile` sets only the four tag fields [ND-HELP], and the 0.64.2 responses above contain neither.
- **Only songs.** `getAlbum`, `getAlbumList2` and `AlbumID3` carry no gain [OS-ALBUM][EXP-ND]. Navidrome stores an album gain internally (the most frequent value among the album's tracks) but doesn't send it [ND-MAP]. For album mode, the per-song `albumGain` is all there is.
- **No feature flag.** Navidrome lists seven extensions (`transcodeOffset`, `formPost`, `songLyrics`, `indexBasedQueue`, `transcoding`, `playbackReport`, `topSongsByArtistId`) and none is for ReplayGain [ND-EXT][EXP-ND]. A client detects support by the key being on a song. Navidrome omits all OpenSubsonic song fields for clients named in `Subsonic.LegacyClients` (default `DSub`) [ND-HELP].
- **Typical libraries.** The values exist only if the files carry tags (or Opus R128 tags); I found no server-side analysis [ND-MAP]. Untagged files give `{}`. Tagging can also be partial (track without album); Navidrome then returns just the fields it has [ND-HELP][EXP-ND]. I did not survey real-world libraries; the share of tagged files is unmeasured.
- **Server-side gain.** The stream bytes are not normalised. The only gain handling is `EnableReplayGain`, which just goes to the web UI's config [ND-CONF].

## 2. What mpv does with it

### Where mpv gets the tags

mpv never asks the server. When the demuxer reports metadata, `demux_update_replaygain` looks at each audio stream's tags, then the file's, for [MPV-DEMUX]:

1. `REPLAYGAIN_TRACK_GAIN` + `REPLAYGAIN_TRACK_PEAK`, with `REPLAYGAIN_ALBUM_*` (album copies track if either album tag is missing);
2. else `REPLAYGAIN_GAIN` + `REPLAYGAIN_PEAK` (album copies track);
3. else `R128_TRACK_GAIN` (+ optional `R128_ALBUM_GAIN`): `/256`, then `+5` dB; peak stays 0.

A missing peak tag defaults to 1.0; a peak that is not above 0 invalidates that tag set. libavformat side data is used first when present [MPV-LAVF]. Because the tag-based path runs on the metadata event, it works on streams: I saw correct gains for [EXP-MPV][EXP-ND]:

| Source (served over HTTP) | `--replaygain=track` | `--replaygain=album` |
|---|---|---|
| FLAC (Content-Length) | -6.0 dB | -3.0 dB |
| MP3 (ID3v2 `TXXX`) | -6.0 dB | -3.0 dB |
| Opus (R128) | 0.0 dB | +2.0 dB |
| FLAC with track tags only | -6.0 dB | -6.0 dB (falls back to track) |
| FLAC with no tags | none | none |
| MP3 transcode, chunked, no Content-Length, no ranges | -6.0 dB | -3.0 dB |
| Opus transcode, same transport | -6.0 dB | n/a (not run) |
| AAC (ADTS) transcode | **none** | **none** |

The same results came from the real Navidrome 0.64.2 for `stream` raw, `format=mp3`, `format=opus` (-6 dB) and `format=aac` (no gain) [EXP-ND][EXP-MPV].

### Transcoded streams

- Navidrome builds the ffmpeg command with `-map_metadata 0 -map_metadata 0:s:a:0`, and the code comment says ADTS (`aac`) output cannot hold tags [ND-FF]. ffprobe on the real responses agrees: `mp3` (ID3), `opus` and `flac` keep all four tags, `aac` has no tags at all [EXP-ND].
- The tags still describe the source file and the transcode doesn't change the level, so the gain stays valid.
- navigo sends no `format` or `maxBitRate` [NAVIGO], so whether Navidrome transcodes is up to the server (`ResolveRequest` also looks at the player's configured max bitrate) [ND-FF]. I did not trace that path further. With the `transcoding` extension (`getTranscodeDecision`/`getTranscodeStream`), which navigo doesn't use, I did not test what tags survive.

### Options

All of these are in the manual for v0.38.0 and v0.41.0 with identical text [MPV-OPT], and the code in `compute_replaygain` is the same in both [MPV-AUDIO]. I didn't diff v0.39 and v0.40.

| Option | Meaning | Tested |
|---|---|---|
| `--replaygain=no\|track\|album` | `no` (default) does nothing; `track` uses track gain; `album` uses album gain, which equals track gain when the file has no album tags | yes |
| `--replaygain-preamp=<dB>` | Added to the chosen gain before the clip cap. Range -150 to 150 | yes (+6 dB: -6 becomes 0 dB) |
| `--replaygain-clip=yes\|no` | `no` (default): factor limited to `1/peak`; `yes`: no cap. With peak 0.5, preamp +18 (gain +12 dB): capped to 2.0 vs 3.98 with `yes`. Opus R128 has no peak, so no cap ever applies | yes |
| `--replaygain-fallback=<dB>` | Gain used when the file has no ReplayGain data, **or when `--replaygain=no`**; then no other ReplayGain option is applied. 0 means off. Range -200 to 60 | yes |

The fallback detail comes from `if (rgain_mode && rg) { ... } else if (rgain_fallback) { ... }`: with `--replaygain=no --replaygain-fallback=-8` a tagged file is played at -8 dB [MPV-AUDIO][EXP-MPV]. So a non-zero fallback must be set together with a mode, or it acts as a flat volume offset.

## 3. Changing the mode at runtime

- `replaygain`, `replaygain-preamp`, `replaygain-clip` and `replaygain-fallback` are normal options flagged `UPDATE_VOL`, so `set_property` over IPC re-runs `audio_update_volume`, which recomputes the gain from the current track's tags and calls `ao_set_gain` [MPV-AUDIO]. Values are `"no"`, `"track"`, `"album"` (string) and a number for the others [EXP-MPV].
- Tests (all with mpv v0.41.0, via IPC, `--prefetch-playlist=yes`) [EXP-MPV]:
  - One file, mode `no` -> `track` -> `album` -> preamp +3: the level moved -6, -3 and 0 dB in steps. Events during the changes: none; the only events were the initial `start-file`, `file-loaded`, `playback-restart` and the final `end-file`.
  - Two files queued with `loadfile ... append`, `track` from the start: total 16.00 s, longest silence 0 ms, the second track at its own -2 dB gain (its first one-second window was already at -2 dB).
  - Same queue, switched to `album` while the first track played: the second track (already queued and prefetched) came out at its album gain (-1 dB), still 16.00 s and 0 ms silent.
  - Same queue, switched to `no` near the track boundary: the second track played with no gain, no gap, no extra events.
- The change is heard after mpv's already-buffered audio (about 1 to 1.5 s with the `pcm` output). I did not measure this with a real audio output.
- `loadfile` per-file options give another route that doesn't depend on tags: `volume-gain=<dB>` on each entry. It took effect per entry, including on the prefetched next entry, with 16.00 s and 0 ms silence for `volume-gain=-6` then `-12` [EXP-MPV]. It skips peak-based clip prevention and is bounded by `volume-gain-max` (default 12 dB) [MPV-OPT]; `replaygain` must stay `no` or the gains would add.

## 4. Interaction with navigo's volume

- mpv computes `gain = (volume/100)^3 × replaygain_factor × 10^(volume-gain/20)`, and 0 if muted [MPV-AUDIO] (v0.38.0 has the same expression inline in `audio_update_volume`). The cubic curve is applied to `volume` only; ReplayGain multiplies after it.
- Measured: `volume=50` alone gave -18.1 dB (0.5^3); `volume=50` with a -6 dB track gave -24.1 dB; `volume=100` with `volume-gain=-6` plus -6 dB gave -12.0 dB; `volume=130` with a -6 dB track gave +0.84 dB [EXP-MPV].
- navigo clamps its volume to 0 to 100 [NAVIGO], so volume 100 is unity gain and ReplayGain is the only thing that can go above it. Negative gains (the common case) only lower the level. Positive gains boost, up to `1/peak` when the file has a peak and `--replaygain-clip=no`; there is no cap otherwise.
- The `volume` property doesn't change with ReplayGain [MPV-AUDIO], so navigo's volume display and the MPRIS volume stay as they are. mpv's `volume-max` (default 130) limits only the `volume` property, not the ReplayGain factor, and `volume-gain` is a separate stage [MPV-AUDIO][MPV-OPT].

## Open points

- Real-world share of tagged files and the partial-tag cases in actual libraries: not measured.
- Which tags survive through the OpenSubsonic `transcoding` extension: not tested.
- Output latency of a mode change with a real audio output (PulseAudio, CoreAudio, WASAPI): not measured.
- mpv v0.39 and v0.40 were not diffed, only v0.38.0 and v0.41.0 (identical) and `master`.
