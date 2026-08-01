# Releasing LogLeopard

Releases use semantic versions with a `v` prefix. While interfaces and packaging are still settling, use `v0.x.y`. Start with `v0.1.0-rc.1` if the packages need public testing; publish `v0.1.0` only when that candidate is ready for general use within the documented limitations.

Pushing a matching tag runs the `Draft release` workflow. It verifies the source, builds the embedded web UI, packages unsigned binaries for macOS, Linux, and Windows on `amd64` and `arm64`, generates `SHA256SUMS`, and creates a GitHub draft. It never publishes automatically.

## One-time repository setup

Before the first tag:

1. In **Settings > General > Releases**, enable release immutability. It applies only to releases published after it is enabled.
2. In **Settings > Environments**, click **New environment**, name it exactly `release`, and select **Configure environment**. Under **Deployment branches and tags**, choose **Selected branches and tags**, add a **Tag** rule matching `v*.*.*`, and do not add a branch rule. Optionally add yourself under **Required reviewers** to create a manual pause; while you are the only maintainer, leave **Prevent self-review** disabled or you will be unable to approve your own release job. No environment secrets are required.
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
2. From a macOS or Linux checkout, run one command to verify that all expected assets exist, download the archive for the current machine, check its SHA-256 checksum and embedded version, and start it briefly with synthetic data:

   ```sh
   make verify-release TAG=v0.1.0-rc.1
   ```

3. Edit the generated notes. Include what this release is, highlights, install/run steps, known limitations, unsigned/unnotarized status, checksum instructions, and a link to the tag source.
4. Keep an `-rc.N` release marked as a prerelease. Publish the draft only after its notes and assets have been reviewed.
5. After publication, verify immutability with `gh release verify v0.1.0-rc.1`. The pre-publication command already verifies the downloaded archive against `SHA256SUMS`; `gh release verify-asset` is an optional second check against GitHub's release attestation.

When the candidate is accepted, repeat the process from the chosen reviewed commit using a new `v0.1.0` tag. Do not rename the candidate or remove its prerelease status to turn it into the final release.

## Local packaging check

GoReleaser is pinned in CI. To validate the configuration without creating a tag or GitHub release:

```sh
go run github.com/goreleaser/goreleaser/v2@v2.17.1 release --snapshot --clean
```

Snapshot artifacts are written to ignored `dist/`. This cross-compilation check does not replace testing downloaded release binaries on their target operating systems.
