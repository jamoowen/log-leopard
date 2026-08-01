# Releasing LogLeopard

Releases use semantic versions with a `v` prefix. While interfaces and packaging are still settling, use `v0.x.y`. Start with `v0.1.0-rc.1` if the packages need public testing; publish `v0.1.0` only when that candidate is ready for general use within the documented limitations.

Pushing a matching tag runs the `Draft release` workflow. It verifies the source, builds the embedded web UI, packages unsigned binaries for macOS, Linux, and Windows on `amd64` and `arm64`, generates `SHA256SUMS`, and creates a GitHub draft. It never publishes automatically.

## One-time repository setup

Before the first tag:

1. In **Settings > General > Releases**, enable release immutability. It applies only to releases published after it is enabled.
2. In **Settings > Environments**, create a `release` environment. Restrict deployment branches and tags to `v*`; optionally require maintainer approval.
3. Require the normal CI checks on `main` through a branch ruleset.
4. Review the licenses and notice requirements of the bundled Go and frontend dependencies. Add any required third-party notices before distributing binaries.

Release binaries are not currently code-signed or notarized. State this in the release notes, particularly for macOS users.

## Create the first candidate

Start from the exact reviewed commit on `main`, with a clean working tree:

```sh
git switch main
git pull --ff-only origin main
make check
go test -race ./...
pnpm --dir web run test:e2e
git tag -a v0.1.0-rc.1 -m "LogLeopard v0.1.0-rc.1"
git push origin v0.1.0-rc.1
```

The tag is the sole release version. Never move or reuse a pushed release tag; fix a failed candidate and create `v0.1.0-rc.2` instead.

## Review and publish the draft

1. Confirm the tag's `Draft release` workflow succeeded.
2. Open the draft under **Releases** and verify that all six platform archives and `SHA256SUMS` are present.
3. Download the archive for your platform and verify its checksum. On macOS:

   ```sh
   shasum -a 256 -c SHA256SUMS --ignore-missing
   ./log-leopard -version
   ./log-leopard -fake
   ```

4. Edit the generated notes. Include what this release is, highlights, install/run steps, known limitations, unsigned/unnotarized status, checksum instructions, and a link to the tag source.
5. Keep an `-rc.N` release marked as a prerelease. Publish the draft only after its notes and assets have been reviewed.
6. After publication, verify immutability with `gh release verify v0.1.0-rc.1` and verify a downloaded archive with `gh release verify-asset v0.1.0-rc.1 PATH_TO_ARCHIVE`.

When the candidate is accepted, repeat the process from the chosen reviewed commit using a new `v0.1.0` tag. Do not rename the candidate or remove its prerelease status to turn it into the final release.

## Local packaging check

GoReleaser is pinned in CI. To validate the configuration without creating a tag or GitHub release:

```sh
go run github.com/goreleaser/goreleaser/v2@v2.17.1 release --snapshot --clean
```

Snapshot artifacts are written to ignored `dist/`. This cross-compilation check does not replace testing downloaded release binaries on their target operating systems.
