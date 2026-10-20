# Releasing

A release is a `vX.Y.Z` tag. Pushing it runs
`.github/workflows/release.yml`, which runs all of CI (check, e2e and the
build matrix) and then GoReleaser (`.goreleaser.yaml`). GoReleaser builds
the six targets, packs each with `LICENSE` and `README.md`, writes
`checksums.txt`, and drafts a GitHub release with notes made from the
commit subjects since the last tag. The archives get build-provenance
attestations. Nothing is public until you publish the draft.

## Versions

SemVer, starting at v0.1.0. Below 1.0, a minor bump (v0.2.0) may change
the config or key format; say so in the notes. Patch releases (v0.1.1)
only fix things. A tag with a suffix (v0.2.0-rc.1) is marked as a
pre-release.

## Steps

1. On an up-to-date `main` with CI green, run the gate:
   `tools/ci/local.sh`.
2. Tag and push:

   ```sh
   git tag -a v0.1.0 -m v0.1.0
   git push origin v0.1.0
   ```

3. Watch the Release workflow under Actions. If CI fails, nothing is
   drafted: fix on `main`, delete the tag
   (`git push origin :v0.1.0 && git tag -d v0.1.0`), and tag again.
4. Open the draft under Releases. Edit the notes: group the commit list
   into what users notice, call out anything that needs them to act (config
   changes, a newer mpv), and drop what only matters to development.
5. Publish. Watchers who follow releases get notified.

To try GoReleaser locally without publishing:
`goreleaser release --snapshot --clean` (output in `dist/`).

## Verifying a download

```sh
sha256sum -c checksums.txt --ignore-missing
gh attestation verify navigo_0.1.0_linux_amd64.tar.gz --repo anopheles/navigo
```

## macOS

The binaries are not notarized, so macOS quarantines a downloaded one.
After unpacking: `xattr -d com.apple.quarantine navigo`.
