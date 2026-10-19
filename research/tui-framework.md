# Which Go TUI framework fits a modal vim keymap?

Research for [#3](https://github.com/anopheles/navigo/issues/3) (map: [#1](https://github.com/anopheles/navigo/issues/1)). Checked 2026-10-19 against the frameworks' repos, the Go module proxy and their issue trackers.

## Answer

**Use Bubble Tea v2 (`charm.land/bubbletea/v2` v2.1.0) with Lip Gloss v2 for styling. Write navigo's own keymap engine and its own virtualized list. Take only small pieces from Bubbles v2 (textinput for the `:` line and `/` prompt). Don't use `bubbles/list` or `bubbles/key` for the main views.**

Why:

- **Input layer.** Every key arrives as a `tea.KeyPressMsg` in one `Update` function. No widget gets to handle a key before navigo's keymap engine sees it. Counts, `gg`/`dd`, operator-pending state, a `:` line and visual mode all become plain state in the model. tview's built-in widgets handle `g`/`G`/`j`/`k`/`h`/`l` themselves, so navigo would be working against them.
- **Testability.** `View()` is a pure function of the model. Golden-frame tests can call `Update` and `View` directly, with no terminal. teatest v2 adds tests that run the full program.
- **Windows.** Bubble Tea v2 reads Windows Console input records, including resize events, through ultraviolet. Windows Terminal and conhost on Windows 10 1703+ are covered.
- **Maintenance.** The most active project of the three: v2.0.0 shipped 2026-02-24 and there were monthly releases after it. tview's last release is v0.42.0 (2025-08-27), and it is still on tcell v2.

Raw tcell v3 is a solid second choice if Bubble Tea's renderer ever becomes the bottleneck. You would have to write your own layout and widgets, and since tcell v3 removed `SimulationScreen`, you would also have to build your own golden-frame harness.

## Versions checked (Go module proxy, 2026-10-19)

| Module (note the major-version path) | Latest | Released | Source |
|---|---|---|---|
| `charm.land/bubbletea/v2` | v2.1.0 | 2026-10-08 | [proxy](https://proxy.golang.org/charm.land/bubbletea/v2/@latest), [release](https://github.com/charmbracelet/bubbletea/releases/tag/v2.1.0) |
| `charm.land/bubbles/v2` | v2.2.1 | 2026-08-24 | [proxy](https://proxy.golang.org/charm.land/bubbles/v2/@latest) |
| `charm.land/lipgloss/v2` | v2.0.6 | 2026-08-11 | [proxy](https://proxy.golang.org/charm.land/lipgloss/v2/@latest) |
| `github.com/charmbracelet/x/exp/teatest/v2` | pseudo-version only (`v2.0.0-20261018003744-cb897cb4ed72`), no tag | 2026-10-18 | [proxy](https://proxy.golang.org/github.com/charmbracelet/x/exp/teatest/v2/@latest) |
| `github.com/charmbracelet/x/exp/golden` | v0.1.0 | 2026-09-24 | [proxy](https://proxy.golang.org/github.com/charmbracelet/x/exp/golden/@latest) |
| `github.com/rivo/tview` | v0.42.0 (still v0.x, no `/vN`) | 2025-08-27 | [proxy](https://proxy.golang.org/github.com/rivo/tview/@latest) |
| `github.com/gdamore/tcell/v3` | v3.6.0 | 2026-10-10 | [proxy](https://proxy.golang.org/github.com/gdamore/tcell/v3/@latest) |
| `github.com/gdamore/tcell/v2` | v2.13.10 | 2026-05-06 | [proxy](https://proxy.golang.org/github.com/gdamore/tcell/v2/@latest) |

Notes:

- Bubble Tea v2 moved to the vanity path `charm.land/bubbletea/v2`; see its [go.mod](https://github.com/charmbracelet/bubbletea/blob/main/go.mod). The old `github.com/charmbracelet/bubbletea` path is v1, which stopped at v1.3.10 (2025-09-17). Use the v2 path.
- Bubble Tea's go.mod declares `go 1.26.0`, and so does tcell v3's ([go.mod](https://github.com/gdamore/tcell/blob/main/go.mod)). navigo needs Go ≥ 1.26 with either.
- teatest v2 lives under `x/exp`, which means experimental. It has no tagged release, and its go.mod still pins `charm.land/bubbletea/v2 v2.0.0-rc.1` ([go.mod](https://github.com/charmbracelet/x/blob/main/exp/teatest/v2/go.mod)). Pin a pseudo-version, and expect its API to change.

## Comparison

### 1. How much it fights a custom modal input layer

**Bubble Tea v2.**
- Elm-style loop: every key press is delivered to the root model's `Update` as `tea.KeyPressMsg` ([key.go](https://github.com/charmbracelet/bubbletea/blob/main/key.go)). There is no focus system or widget-level key consumption unless you forward messages to child components yourself. navigo's keymap engine can sit at the root and decide which component, if any, gets a key.
- `Key.Keystroke()` / `String()` produce strings like `ctrl+d`, which map naturally onto a YAML binding schema.
- v2 adds key releases, repeat detection and Kitty keyboard-protocol disambiguation. `IsRepeat` works only with the Kitty protocol or the Windows Console ([UPGRADE_GUIDE_V2.md](https://github.com/charmbracelet/bubbletea/blob/main/UPGRADE_GUIDE_V2.md)).
- `bubbles/key.Binding` only matches a single keystroke: `Matches` compares `k.String()` with each binding's key list ([key/key.go](https://github.com/charmbracelet/bubbles/blob/main/key/key.go)). Sequences (`gg`, `dd`), counts and modes are not supported, so navigo needs its own engine (a trie of key sequences per mode, plus a count accumulator and a pending-operator state). That work is the same under every framework. Bubble Tea just doesn't get in the way.
- Timeouts for ambiguous prefixes (`g` vs `gg`) can be done with `tea.Tick` commands.

**tview.**
- Focus-based widget tree. Widgets handle keys in their own `InputHandler`. `Table` hard-codes vim-ish keys: `g`, `G`, `j`, `k`, `h`, `l`, and `Ctrl-F`/`Ctrl-B` ([table.go](https://github.com/rivo/tview/blob/master/table.go) InputHandler).
- You can intercept keys globally with `Application.SetInputCapture` or per widget with `Box.SetInputCapture` ([application.go](https://github.com/rivo/tview/blob/master/application.go), [box.go](https://github.com/rivo/tview/blob/master/box.go)). For full rebinding, though, navigo would have to swallow nearly every key and drive widgets through their setters. At that point the widgets' built-in navigation is a liability: unbound keys could leak through to hard-coded behaviour.
- The upstream owner has said he hesitates to move to tcell v3 because it would break users ([rivo/tview#1145](https://github.com/rivo/tview/issues/1145), closed 2026-07). navigo would be tied to tcell v2.

**Raw tcell v3.**
- No widgets, so nothing to fight. You read `EventKey`s from the `EventQ` channel, which makes it easy to `select` on a key-sequence timeout ([CHANGESv3.md](https://github.com/gdamore/tcell/blob/main/CHANGESv3.md)).
- v3 also cleans up key events: `KeyRune` carries a string, Ctrl-letter keys carry the rune plus `ModCtrl`, `KeyBackspace2` is folded into `KeyBackspace`, and Ctrl-Space is a rune with `ModCtrl`.
- The cost is that layout, scrolling, text input, styling and redraw scheduling all have to be built by hand.

### 2. Scrolling through lists of 10k+ items

- **Bubbles `list`:** not suited for this. An open issue reports noticeable scroll delay at about 8,000 items ([charmbracelet/bubbles#810](https://github.com/charmbracelet/bubbles/issues/810)). It also bundles fuzzy filtering and pagination that don't match vim `/` + `n`/`N` semantics. navigo should write its own virtualized list: keep the cursor and offset in the model and render only the visible rows. That is roughly 100 lines and fits the vim motions directly (counts, `ctrl-d`/`ctrl-u`, `gg`/`G`, visual ranges).
- **Bubble Tea v2 renderer:** the "cursed" renderer is built on ultraviolet's cell buffer and `TerminalRenderer` and diffs cells between frames ([cursed_renderer.go](https://github.com/charmbracelet/bubbletea/blob/main/cursed_renderer.go)). As long as `View()` only builds the visible rows, frame cost depends on terminal size, not list length.
- **tview `Table`:** supports virtual data through the `TableContent` interface (`Table.SetContent`) ([table.go](https://github.com/rivo/tview/blob/master/table.go)), so 10k+ rows are fine. Its scrolling keys are hard-coded, as noted above.
- **Raw tcell:** you draw only what's visible. As fast as you make it.

### 3. Resize handling

- **Bubble Tea:** delivers `tea.WindowSizeMsg`. On Windows, ultraviolet reads `WINDOW_BUFFER_SIZE_EVENT` through `ReadConsoleInput` ([terminal_reader_windows.go](https://github.com/charmbracelet/ultraviolet/blob/main/terminal_reader_windows.go)). v2.1.0 fixed a fallback for 0x0 terminal sizes ([release notes](https://github.com/charmbracelet/bubbletea/releases/tag/v2.1.0)).
- **tcell:** `EventResize`. On Windows it uses `ReadConsoleInput` for resize notifications even in VT input mode ([README-windows.md](https://github.com/gdamore/tcell/blob/main/README-windows.md)). tview redraws on resize automatically.

### 4. Windows Terminal / conhost

- **Bubble Tea v2:**
  - Puts the console in raw mode and enables `ENABLE_VIRTUAL_TERMINAL_INPUT` / `ENABLE_VIRTUAL_TERMINAL_PROCESSING` ([tty_windows.go](https://github.com/charmbracelet/bubbletea/blob/main/tty_windows.go)), then reads console input records through ultraviolet, which covers both conhost and Windows Terminal.
  - Suspend (ctrl-z) is unsupported on Windows (`suspendSupported = false`), which doesn't matter for navigo.
  - Open Windows key issues: Ctrl-Space not detected in plain PowerShell/conhost ([#1495](https://github.com/charmbracelet/bubbletea/issues/1495)); garbled clipboard paste ([#1712](https://github.com/charmbracelet/bubbletea/issues/1712)); a bare Ctrl key-down delivered as a NUL rune, reported against v1's `key_windows.go` ([#1865](https://github.com/charmbracelet/bubbletea/issues/1865)). The keymap engine should ignore NUL or empty-text key events, and default bindings should avoid Ctrl-Space.
- **tcell v3:** Windows 10 1703+ only. v3 dropped the legacy console API (`NewConsoleScreen`) and uses VT mode with win32-input-mode for advanced key reporting. Its main Windows test target is Windows Terminal. Over SSH from Windows, rich key reporting degrades, so Ctrl-I and Tab can't be told apart ([README-windows.md](https://github.com/gdamore/tcell/blob/main/README-windows.md), [CHANGESv3.md](https://github.com/gdamore/tcell/blob/main/CHANGESv3.md)).
- **tview:** inherits tcell v2's Windows support, including the legacy console API.

### 5. Maturity and maintenance (as of 2026-10-19)

| | Latest | Cadence | Activity |
|---|---|---|---|
| Bubble Tea | v2.1.0 (2026-10-08) | v2.0.0 on 2026-02-24, then roughly monthly patch releases (v2.0.7 to v2.0.10, Jun–Sep 2026) | ~45k stars, 267 open issues, pushed 2026-10-08 |
| Bubbles | v2.2.1 (2026-08-24) | v2.0.0 2026-02-24, v2.1.0 2026-03, v2.2.0 2026-08 | pushed 2026-10-04 |
| Lip Gloss | v2.0.6 (2026-08-11) | monthly patches | pushed 2026-10-11 |
| tcell | v3.6.0 (2026-10-10) | monthly minors (v3.4.0 May to v3.6.0 Oct) | 8 open issues, pushed 2026-10-19 |
| tview | v0.42.0 (2025-08-27) | no release in over a year; only master commits (last 2026-08-11) | ~14k stars, 104 open issues, still on tcell v2 |

(Repo stats from the GitHub API; release dates from the GitHub releases pages and the module proxy.)

Bubble Tea v2 is a fresh major version, and some v2 rough edges are still open, e.g. [#1848](https://github.com/charmbracelet/bubbletea/issues/1848) (a panic leaves the terminal in alt-screen) and [#1850](https://github.com/charmbracelet/bubbletea/issues/1850) (some error paths leave the tty in raw mode). navigo should recover panics and restore the terminal in `main`.

### 6. Testability (golden-frame tests)

- **Bubble Tea:** the best of the three.
  - `Update(msg) (Model, Cmd)` and `View()` are pure, so a test can feed a sequence of `KeyPressMsg`s and compare `View().Content` against a golden file with `x/exp/golden` (v0.1.0). No PTY or goroutines are involved. This suits keymap tests like "`5j` then `dd` removes queue item 6".
  - For whole-program tests, `teatest/v2` provides `NewTestModel`, `Send`, `Type`, `WaitFor`, `FinalModel` and `RequireEqualOutput` ([teatest.go](https://github.com/charmbracelet/x/blob/main/exp/teatest/v2/teatest.go)). It is experimental and untagged (see above).
- **tview:** tests need a `tcell.Screen` passed to `Application.SetScreen`, normally tcell v2's `SimulationScreen`. The app runs its own event loop goroutine, so tests must inject events and wait for draws, and widget state is spread across mutable widget objects.
- **Raw tcell v3:** `SimulationScreen` is removed. Its replacements, `MockScreen`/`MockTerm`, are explicitly not public API ([CHANGESv3.md](https://github.com/gdamore/tcell/blob/main/CHANGESv3.md)). navigo would have to put its own rendering behind an interface to make golden tests work.

## Recommendation for navigo

1. Depend on `charm.land/bubbletea/v2` v2.1.0 and `charm.land/lipgloss/v2` v2.0.6. Add `charm.land/bubbles/v2` v2.2.1 only for `textinput` (`:` command line, `/` search prompt) and maybe `spinner`. Set `go 1.26` in go.mod.
2. Build a **keymap engine** package that doesn't depend on Bubble Tea: a per-mode trie from key sequences to actions, a count prefix, operator-pending state and timeouts. Its input should be a normalized keystroke string (from `KeyPressMsg.Keystroke()`), and it should drop NUL or empty keys (Windows quirk). It can then be unit-tested without a terminal, and the YAML rebinding schema maps onto it directly.
3. Build a **virtualized list** component (cursor, offset, visual-selection range) that renders only visible rows. Don't use `bubbles/list`.
4. Tests: pure `Update`/`View` golden tests with `x/exp/golden` as the main layer, plus a few `teatest/v2` program tests pinned to a pseudo-version. This feeds the "E2E testing strategy" item still open on the map.
5. Avoid Ctrl-Space as a default binding because of the Windows conhost issue.
