# Contributing to navigo

Contributions are welcome, from people and from coding agents alike.
Both are held to the same rule:

**A change comes with evidence that it works.** Saying it does, or that
it should, is not evidence. A pull request without evidence isn't merged,
however good the code looks.

## What counts as evidence

Give the evidence that fits the change, in the pull request:

- **A bug fix:** a test that fails without the fix and passes with it.
  Say how you saw it fail: paste the failing output, or name the commit
  you ran it on.
- **New behaviour:** tests at the layer the behaviour lives in (see
  [Testing](docs/spec.md#testing)):
  - the core (`internal/app`): unit tests on the effects `Apply` returns;
  - the keymap and command parser: unit tests;
  - the screen and keys: layer A tests in `internal/ui`, which drive the
    real model against the fake server;
  - the real binary, mpv and Navidrome together: layer B in `e2e/`.
- **Anything you can see on screen:** the golden frames that changed,
  with the diff shown in the pull request (`go test ./internal/ui
  -update` rewrites them), or a terminal capture before and after.
- **Speed or memory:** measurements before and after, and how you took
  them.
- **Always:** the output of `tools/ci/local.sh` ending in `gate: green`.
  If the change touches playback, the server or startup, also the output
  of `tools/e2e/run.sh`.

If something can't be tested automatically, say what you did by hand,
what you saw, and on which OS, terminal and mpv version.

## Before you start

- **Read the spec.** [docs/spec.md](docs/spec.md) says what navigo does,
  [GLOSSARY.md](GLOSSARY.md) names things, and [docs/adr/](docs/adr/)
  records the decisions. A change that alters behaviour updates the spec
  in the same pull request.
- **Bigger changes start as an issue,** so the design is agreed before
  the code is written.
- **Keep a pull request to one change** that leaves the gate green.

## Rules that are easy to miss

- **Licences:** navigo is MIT. Linked modules must be MIT, BSD, Apache-2.0,
  ISC or MPL-2.0; the gate checks this. No code derived from GPL
  software, not even translated: GPL clients may be looked at as a user
  would, never used as a source.
- **New modules:** check the latest released version and use it. Explain
  why the module is worth its weight.
- **Test data:** fixtures are recorded from the synthetic fixture library
  (`tools/fixtures.sh`), never from a real server, because the repository
  is public. Re-record rather than editing fixtures by hand.
- **Secrets:** tokens, passwords and stream URLs are never logged,
  printed or committed.
- **Style:** match the code around you: its naming, its comments and how
  much it says. The gate runs golangci-lint with the linters and
  formatters in `.golangci.yml` (gofmt, goimports, and golines at 100
  columns), and `go vet`; `golangci-lint fmt` fixes the formatting.

## Running the checks

```sh
tools/ci/local.sh   # golangci-lint, vet, tests, licences: what CI runs
tools/e2e/run.sh    # layer B against a throwaway Navidrome (needs Docker)
```

`go test -tags e2e ./e2e` runs layer B against your own server instead,
from `.env.e2e` (`NAVIGO_E2E_URL`, `NAVIGO_E2E_USER`,
`NAVIGO_E2E_PASSWORD`). It plays tracks there, so they count as played
for that account.

## Recording the README demo

`make demo` records `docs/media/demo.gif`: a throwaway Navidrome serves the
synthetic fixture library with generated covers, and [VHS](https://github.com/charmbracelet/vhs)
runs navigo against it in Docker (`tools/demo/`). It needs Docker and
Go. The script is `tools/demo/demo.tape`; change it when the interface
changes and record again. Never record a real library: the repository is
public.
