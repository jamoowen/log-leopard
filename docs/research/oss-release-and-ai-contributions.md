# First GitHub Release and AI-Assisted Contribution Policy Research

Research date: 2026-08-01

## Scope and method

This note answers two questions for LogLeopard:

1. How should an untagged Go project publish its first GitHub binary release?
2. What practical, enforceable policy should an open-source repository use for AI-assisted contributions?

Sources are limited to GitHub's official documentation and CLI manual, Go's official documentation, the policy files of the named open-source projects, and LogLeopard's own repository files and AGPL license. Quotations below are direct; recommendations are explicitly identified as recommendations rather than source requirements. No blogs, vendor commentary, or policy summaries were used.

## Executive recommendations

### First release

- Use `v0.1.0` for the first public tag and GitHub release. LogLeopard is useful but explicitly has no packaged release or updater and still lists hardening, accessibility, packaging, and reproducible releases as work in progress. Go describes `v0.x.x` as in development and unstable, with no compatibility guarantee.[^go-version-numbers]
- If the first binaries are intended for testing rather than normal use, first publish `v0.1.0-rc.1` and mark the GitHub release as a prerelease. Promote by creating a new `v0.1.0` tag and release from a reviewed commit; do not move or reuse the release-candidate tag.
- Make the Git tag the sole version input. Require an existing remote tag (`gh release create --verify-tag`) rather than allowing the release command to create a tag from whatever happens to be the default branch head.[^gh-release-create]
- Build self-contained archives for Linux, macOS, and Windows from the tagged commit. Each archive should contain the production-tagged Go executable with the already-built React `web/dist` embedded, plus `LICENSE`, `README.md`, and a short install/readme file if needed.
- Publish `SHA256SUMS` covering every downloadable archive. Verify it before upload, then download the published assets and verify again. GitHub also records a `digest` for release assets, and immutable releases provide cryptographically signed release attestations that can be checked with `gh release verify` and `gh release verify-asset`.[^release-assets-api] [^verify-release]
- Enable GitHub's immutable releases setting before publishing. Create a draft, attach all assets, inspect the notes and asset set, and publish once. GitHub then locks the tag and assets and automatically creates a release attestation.[^immutable-releases]
- Keep the ordinary CI workflow read-only. Give only the release/publish job `contents: write`; give build jobs `contents: read`. Add `id-token: write` and `attestations: write` only if producing separate build-provenance attestations with `actions/attest`, not merely to upload a GitHub release.[^workflow-permissions] [^artifact-attestations-howto]
- Generate GitHub release notes as a starting point, but edit them into a concise user-facing record: what LogLeopard is, supported systems, installation/start instructions, notable changes, known limitations, checksums/verification, and the full comparison link. Generated notes include merged PRs, contributors, and a full changelog link and can be categorized by labels.[^generated-release-notes]

### AI-assisted contributions

- Permit AI assistance, but place responsibility entirely on the human contributor. Require disclosure of the tool and the extent of assistance, understanding sufficient to explain every change without AI, human-authored PR/review discussion, complete testing, and confirmation that the contributor has the right to submit all content under the project's AGPL-3.0 license.
- Prohibit autonomous or bulk-generated issues, PRs, reviews, and security reports; unverified claims; bypassing tests; and submission of credentials, pairing URLs, real cloud logs, project IDs, private vulnerability details, or other non-public data to AI services.
- State that disclosure does not excuse quality, licensing, security, or privacy failures. Maintainers may close low-value, unverifiable, unexplained, or policy-violating submissions without detailed review and may block repeated abuse.
- Enforce the policy at contribution boundaries: normative text in `CONTRIBUTING.md`, required confirmations in the PR template and issue forms, existing CI as a merge requirement, and maintainer requests for explanation or reproduction when understanding is in doubt. Automated AI detection should not be the enforcement mechanism; the enforceable facts are disclosure, test results, provenance certification, and the contributor's ability to explain and maintain the work.

## Brief LogLeopard audit

The repository is currently clean and has two commits but no Git tags. There is one CI workflow and no release workflow.

### Release-relevant shape

- `go.mod` declares module `github.com/jamoowen/log-leopard` and Go `1.26.5`.
- The shipped product is a Go `main` package under `cmd/log-leopard`, not primarily a library module. Go's tag conventions still provide a familiar, machine-valid release scheme even though the principal artifact is a binary.
- Production builds first run the Vite build and then run `go build -tags production`. `web/embed_production.go` embeds `dist/*` with `//go:embed`; therefore each released executable contains the React UI and should not need a separate web asset at runtime.
- `Makefile` currently writes a host binary named `log-leopard`. There is no cross-platform packaging, checksum generation, release metadata injection, signing, or release upload target.
- The command contains OS-specific browser launching for macOS, Linux, and Windows. This is evidence of intended platform behavior, not proof that every architecture/package has been tested.
- No cgo use was found in project Go source. That makes cross-compilation plausible, but release targets must still be built and smoke-tested before being advertised as supported.
- `README.md` says: "There is no packaged release or updater yet" and calls reproducible cross-platform releases near-term work. It also requires Go 1.26.5+, Node 22.12+, pnpm 11.8.0, and golangci-lint 2.10.1 for source builds.
- Existing CI already does the important pre-release work: pinned action commit SHAs, `permissions: contents: read`, Go race tests, frontend formatting/lint/typecheck/unit tests, API drift checks, a production build, Playwright, `govulncheck`, and production dependency audit.
- Existing Dependabot configuration covers Go, npm, and GitHub Actions, including SHA-pinned actions.

### Contribution-policy shape

- `CONTRIBUTING.md` already requires `make check`, race tests for Go changes, Playwright for browser workflow changes, generated API artifacts when contracts change, synthetic fixtures, and private vulnerability reporting.
- The PR template already asks contributors to confirm verification, absence of credentials/tokens/project IDs/queries/real logs, and generated API updates.
- `AGENTS.md` adds stricter project invariants: cloud credentials stay in Go, all log/query content is hostile, production is loopback-only, cloud operations are read-only, and returned logs, credentials, project IDs, queries, and filters must not be persisted or printed.
- `SECURITY.md` prohibits public vulnerability reports and sensitive material, directs reports to GitHub private vulnerability reporting, and requires synthetic reproduction data.
- The repository is AGPL-3.0. Its license permits conveying object code only when machine-readable Corresponding Source is also made available by an allowed method. For network downloads, section 6(d) requires "equivalent access to the Corresponding Source in the same way through the same place at no further charge" and "clear directions next to the object code saying where to find the Corresponding Source."[^logleopard-license]

These facts favor a small release workflow with no cloud credentials and an AI policy that extends the existing privacy boundary rather than inventing a separate regime.

## Release research

### Tags and semantic versions

GitHub releases are not independent version records: "Releases are based on Git tags, which mark a specific point in your repository's history."[^about-releases] The tag therefore needs to identify exactly the source used to build every attached binary.

Go's official versioning documentation uses semantic versions with a mandatory `v` prefix. It assigns these meanings:[^go-version-numbers]

- `v0.x.x`: "still in development and unstable" with no stability or backward-compatibility guarantees.
- `v1.x.x`: stable major line; later major numbers signal backward-incompatible public API changes.
- `vX.Y.x`: backward-compatible public API additions.
- `vX.Y.Z`: patch changes that do not affect public API or dependencies.
- `vX.Y.Z-beta.2`: a prerelease milestone with no stability guarantees.

Go's publishing guide demonstrates `git tag v0.1.0` and warns: "Don't change a tagged version of a module after publishing it." Go may authenticate module content against an earlier copy and return a security error if the content later differs.[^go-publishing]

#### Recommendation for LogLeopard

- Stable-looking first public release: `v0.1.0`.
- Preview before that release: `v0.1.0-rc.1`, then `v0.1.0-rc.2` if another candidate is needed.
- Subsequent compatible features: `v0.2.0`; fixes to `v0.1.0`: `v0.1.1`.
- Do not infer the binary version from `web/package.json` (`0.1.0`). That package is private and is only one embedded component. The Git tag should identify the whole Go-plus-React product.
- Do not use incomplete tags such as `v0.1`, moving aliases such as `latest`, or date-only tags as release identities. A moving download URL may be offered for convenience, but immutable versioned tags/assets remain authoritative.
- Consider `v1.0.0` only after the CLI/config/API behavior and supported platform contract are intentionally stable. Version zero is not a reason to make arbitrary breaking changes; release notes should still identify them.

Although Go requires `/v2` in a module path for a published module at major version 2 or later, that is a future library/module concern, not a reason to change LogLeopard's current module path for `v0` or `v1`.[^go-version-numbers]

### Prereleases and "Latest"

GitHub provides a prerelease flag specifically "to notify users that the release is not ready for production and may be unstable." If maintainers do not explicitly set a latest release, GitHub assigns the label using semantic versioning.[^managing-releases]

The release candidate should carry both signals:

- Semantic version: `v0.1.0-rc.1`.
- GitHub release metadata: prerelease enabled (`gh release create ... --prerelease`).

This avoids a version that looks final while relying only on a UI badge. A final release is a new `v0.1.0` tag and release, not an edit that removes the prerelease marker from `v0.1.0-rc.1`.

Use a prerelease when the binaries need broad platform testing, packaging is new, or installation behavior may change. If maintainers have already completed that validation and are comfortable recommending normal use within the documented limitations, `v0.1.0` can be the first release without an RC.

### Release creation and immutability

GitHub recommends this sequence for immutable releases: create a draft, attach all assets, then publish.[^immutable-releases] Once published, an immutable release provides these protections:

- The associated tag is locked to its commit and cannot be moved or deleted while the release exists.
- Attached assets cannot be modified or deleted.
- GitHub automatically generates a cryptographically verifiable release attestation containing the tag, commit SHA, and release assets.
- Deleting the release does not allow reuse of the same tag name.

Enable repository release immutability before the first release; it only applies to future releases.[^prevent-release-changes] Treat a publishing error as a reason to issue a new version, not to mutate an already downloaded release.

The official `gh release create` behavior supports the desired safe flow. With assets and immutable releases enabled, it uses separate calls to create a draft, upload assets, and publish. `--verify-tag` aborts unless the remote tag already exists; without that option, the command can automatically create a tag from the default branch head.[^gh-release-create]

Recommended publication shape:

```text
maintainer creates and pushes annotated tag
  -> tag workflow validates tag and source commit
  -> full tests and production build
  -> package each target
  -> generate and verify SHA256SUMS
  -> optionally create build-provenance attestations
  -> create draft release and upload complete asset set
  -> inspect assets and generated/edited notes
  -> publish immutable release
  -> download and verify each published asset
```

For maximum control on a first release, make publication a protected environment job requiring maintainer approval after all artifacts exist. Do not use a privileged `pull_request_target` workflow to build or publish contributor code. GitHub warns that privileged triggers combined with untrusted checkout can expose repository write access or secrets.[^actions-security]

### GitHub Actions permissions

GitHub says to grant `GITHUB_TOKEN` the least required access. Defining any permissions sets unspecified permissions to `none`; `contents: write` permits release creation.[^workflow-permissions] Actions can access `github.token` even when it is not passed explicitly, so permissions matter for every action in a job.[^github-token]

Recommended job-level permissions:

| Job                                                              | Permissions                                                | Reason                                                                                                   |
| ---------------------------------------------------------------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Validate, test, frontend build, target builds, package, checksum | `contents: read`                                           | Checkout/read only. Preserve the current CI default.                                                     |
| Explicit build attestation, if adopted                           | `contents: read`, `id-token: write`, `attestations: write` | These are the permissions GitHub documents for attesting binaries.[^artifact-attestations-howto]         |
| Publish GitHub release                                           | `contents: write`                                          | Required to create the release and upload assets. No other write scope is needed.[^workflow-permissions] |

Prefer `permissions: {}` or `contents: read` at workflow level, then elevate only the publish/attestation jobs. The release job should consume only artifacts produced by jobs from the same trusted tag workflow, and it should not execute scripts from an untrusted PR.

Continue pinning every third-party action to a full commit SHA. GitHub calls a full-length SHA "currently the only way to use an action as an immutable release" and recommends checking that the SHA belongs to the action's repository, not a fork.[^actions-security] LogLeopard already follows this practice.

The release workflow should not need stored secrets: `GITHUB_TOKEN` can create the release, the frontend build is deterministic from public dependencies, and tests are intentionally independent of GCP. Do not add GCP credentials or live log access to release jobs.

### Build and artifact plan

Start with targets that correspond to code paths already present and that can be tested:

| Asset                                   | Executable        | Archive    |
| --------------------------------------- | ----------------- | ---------- |
| `log-leopard_0.1.0_linux_amd64.tar.gz`  | `log-leopard`     | tar + gzip |
| `log-leopard_0.1.0_linux_arm64.tar.gz`  | `log-leopard`     | tar + gzip |
| `log-leopard_0.1.0_darwin_amd64.tar.gz` | `log-leopard`     | tar + gzip |
| `log-leopard_0.1.0_darwin_arm64.tar.gz` | `log-leopard`     | tar + gzip |
| `log-leopard_0.1.0_windows_amd64.zip`   | `log-leopard.exe` | ZIP        |
| `SHA256SUMS`                            | n/a               | plain text |

This is a proposed matrix, not a claim that all five targets are presently verified. It is acceptable to release a smaller tested matrix and add platforms in a later minor release. In particular, macOS binaries may trigger Gatekeeper expectations if they are not signed/notarized; release notes should state the actual signing status and installation steps rather than imply native packaging.

Build requirements specific to this repository:

1. Check out the exact tag commit with full history/tags available for version validation.
2. Install Go from `go.mod`, pnpm 11.8.0, and Node 22.
3. Install frontend dependencies with the frozen lockfile.
4. Run the same checks as CI, including race, browser, vulnerability, and production-tag tests where feasible. A release workflow must not assume the branch CI ran for the tag, because current `ci.yml` triggers only for pushes to `main` and pull requests.
5. Build `web/dist` once from the tagged source and make that exact directory available to each binary build.
6. Build with `-tags production` so `web/embed_production.go`, rather than the empty development filesystem, is selected.
7. Prefer reproducible build flags such as `-trimpath` and explicit version/commit/date linker values once the application exposes version information. Avoid timestamps unless deliberately sourced from the commit or `SOURCE_DATE_EPOCH`.
8. Package the executable with `LICENSE` and a source link. Because LogLeopard distributes AGPL object code, put clear Corresponding Source directions next to the binary download and retain source availability for the required period.[^logleopard-license]
9. Audit the licenses and notice requirements of statically linked Go dependencies and bundled frontend dependencies before distribution; include required third-party notices. GitHub's automatic tag source archive is useful, but publishing it does not by itself establish that the binary's complete source and notice obligations were reviewed.
10. Smoke-test each native binary on its target OS: it starts in `-fake` mode, binds to numeric loopback, serves the embedded UI, returns OpenAPI, and exits cleanly. Cross-compilation success alone is not a runtime test.

Building on native GitHub-hosted runners is the conservative first-release choice, especially for macOS and Windows. If later builds use `GOOS`/`GOARCH` cross-compilation with `CGO_ENABLED=0`, first confirm every dependency and target behaves correctly and retain native smoke tests.

### Checksums, release digests, and attestations

Use three complementary layers, each answering a different question:

1. **`SHA256SUMS`:** portable verification for users without GitHub CLI. Generate it from the final archives, sort entries deterministically, and publish it as a release asset. Verify all entries before publication and after redownloading assets.
2. **GitHub release asset digest:** the official release-assets API includes a `digest` field for each asset.[^release-assets-api] This provides an API-visible digest but should not replace the easy-to-download checksum manifest.
3. **GitHub attestations:** immutable releases automatically attest the release/tag/commit/assets. `gh release verify TAG` checks that a release exists and is immutable; `gh release verify-asset TAG FILE` validates the local asset digest against the signed release attestation.[^verify-release] [^gh-verify-asset]

Optional explicit build attestations provide a stronger link to the build workflow. GitHub states that attestations identify the workflow, repository, organization, environment, commit SHA, and triggering event; for public repositories their Sigstore bundles are recorded in a public transparency log.[^artifact-attestations] GitHub also cautions that an attestation is not proof that software is secure; it links the artifact to source and build instructions so a consumer can apply a policy.[^artifact-attestations]

If adopted, attest the final archives that users download, not transient test builds. GitHub specifically recommends signing released software, binaries, packages, or manifests of hashes, and says frequent test builds should not be signed.[^artifact-attestations]

### Release notes

GitHub-generated release notes contain merged PRs, contributors, and a full changelog link. `.github/release.yml` can categorize entries by labels and exclude labels/authors.[^generated-release-notes] For an early two-commit repository, generated notes may be noisy or sparse; they are input to editorial review, not a substitute for it.

Recommended first-release note structure:

```markdown
# LogLeopard v0.1.0

First packaged release of the local-first browser UI for querying Google Cloud
Run logs. The UI is embedded in each executable; no separate Node server is
required.

## Highlights

- ...

## Install and run

- Download the archive for the supported OS/architecture.
- Verify it against SHA256SUMS (or use gh release verify-asset).
- Authenticate with Application Default Credentials as documented in README.
- Run log-leopard; cloud access remains read-only.

## Known limitations

- GCP Cloud Logging and Cloud Run only.
- Local browser application; no updater/native installer.
- Exact signing/notarization status.

## Verification

- SHA256SUMS link and commands.
- gh release verify / verify-asset commands.

## Source and license

- Exact tag/source archive link and AGPL-3.0 notice.

**Full changelog**: <comparison URL>
```

For later releases, call out user-visible behavior, security fixes (coordinated with a GitHub security advisory where applicable), configuration/API compatibility, and upgrade actions. GitHub explicitly recommends publishing a repository security advisory when a release fixes a vulnerability.[^about-releases]

## AI-assisted contribution policy research

### Policy spectrum in notable projects

The primary-source sample shows that there is no single OSS rule, but there is strong agreement that a human remains accountable.

#### CPython: quality and responsibility, disclosure appreciated

CPython states: "The person submitting an issue or PR is responsible for its content, regardless of whether AI tools were used in its creation." Authors must review AI work in detail and be able to explain changes in their own words. Disclosure is appreciated but not required. The policy reiterates minimal focused changes, coding style, tests, and compatibility, and permits maintainers to close unproductive work without explanation and block repeat disruption.[^cpython-ai]

Useful LogLeopard lesson: ordinary engineering requirements should not be weakened or duplicated for AI output. LogLeopard should go one step stricter on disclosure because disclosure is cheap, verifiable by contributor attestation, and useful during review.

#### curl: disclosure for reports, verification, licensing burden, tests

curl requires disclosure when AI helped find reported problems, demands that reporters verify findings themselves, warns against pasted AI reports, and immediately bans users who submit fabricated reports. For PRs, contributors bear the burden of ensuring no unlicensed code is submitted; AI-assisted code must still meet coding standards, clarity, documentation, tests, and all normal requirements.[^curl-contribute]

Useful LogLeopard lesson: apply disclosure to issues and security reports, not just code; require a minimal synthetic reproduction and prohibit raw generated reports. Licensing and provenance remain the contributor's obligation.

#### Bitcoin Core: no autonomous agents; human chooses, understands, and communicates

Bitcoin Core allows AI coding tools but requires contributors to know the language, be capable of writing the code, understand surrounding code and effects, and explain changes in their own words. It says: "Pull requests should not be opened or driven by autonomous agents" and permits closure without notice. It also prohibits copying AI responses into reviewer discussions.[^bitcoin-ai]

Useful LogLeopard lesson: distinguish assistance from agency. A named human must choose the work, own the design and maintenance burden, and personally handle review. An agent account or unattended issue/PR generator is not an acceptable contributor.

#### Ghostty: mandatory disclosure and demonstrable understanding

Ghostty requires disclosure of every AI tool and the extent of assistance. The human must understand all code and must review and edit AI-assisted issue/discussion text. Its contribution guide makes understanding the "critical rule" and says contributors must explain interactions with the greater system without AI.[^ghostty-ai] [^ghostty-contributing]

Useful LogLeopard lesson: require a short factual disclosure, not transcripts or performative detail. Understanding is enforced through normal review questions, not a detector.

#### Git: reject slop, unexplained changes, and uncertain provenance

Git connects AI use directly to its Developer Certificate of Origin: contributors must know origin and have the right to submit under the project's license. It says the project will reject content that looks generated, bloated, nonsensical, or not understood/explainable, while recommending careful uses such as guidance, debugging, and pre-submission checking.[^git-submitting]

Useful LogLeopard lesson: define rejection in terms of observable defects and contributor responsibility. "Looks AI-generated" alone is subjective; unexplained behavior, unverified claims, unnecessary churn, missing tests, and unknown provenance are concrete grounds.

#### QEMU: prohibition where DCO compliance is unclear

QEMU currently declines contributions believed to include or derive from generative-AI output because contributors may not be able to certify copyright/license provenance under its DCO. It allows research, static analysis, and debugging when generated output is not included, and allows discussed exceptions. Its sign-off remains responsibility for the entire patch.[^qemu-provenance]

Useful LogLeopard lesson: maintain a fallback rule that content must be rejected whenever provenance or licensing cannot be established. A total prohibition is defensible but not necessary if LogLeopard adopts mandatory human ownership, disclosure, provenance certification, and maintainer discretion.

### Recommended LogLeopard requirements

The following is recommended policy substance, not a claim about current repository rules.

#### 1. Disclosure

Require disclosure in every issue, PR, security report, or substantial review comment materially assisted by generative AI:

- Tool/product name.
- Extent: for example research, debugging, code generation, tests, documentation, translation, or editing.
- Human verification performed.

Example: `AI assistance: Claude Code was used to inspect the query parser and draft tests. I reviewed and edited all changes and ran make check and go test -race ./....`

Do not require prompts, transcripts, model output, or private account details. They create noise and may themselves contain sensitive information. Failure to disclose may result in closure; intentional/repeated failure may result in blocking.

#### 2. Human understanding and ownership

Require that the submitter:

- Chose the problem and proposed scope.
- Understands every submitted code and documentation change, including interactions with the Go backend, embedded React UI, API schema generation, and security/privacy invariants.
- Can explain the design, tradeoffs, failure behavior, and tests in their own words without asking an AI tool to answer maintainers.
- Personally responds to review and remains the author responsible for correcting and maintaining the contribution.
- Does not open or drive contributions through an autonomous agent.

If the contributor cannot explain a change after a reasonable question, maintainers may close it. This is direct, observable enforcement used by Bitcoin Core, Ghostty, Git, and CPython.[^bitcoin-ai] [^ghostty-ai] [^git-submitting] [^cpython-ai]

#### 3. Testing and verification

AI-assisted work must meet exactly the same or higher verification bar:

- Run `make check`.
- Run `go test -race ./...` when Go behavior or concurrency changes.
- Run `pnpm --dir web run test:e2e` when browser workflows change.
- Add focused regression tests for bug fixes and behavior tests for features.
- Use synthetic fixtures only.
- State what was run and any test not run, with a reason.
- Never alter, weaken, skip, or delete a valid test merely to make generated code pass.

CPython explicitly calls bypassing tests an unacceptable non-fix; curl requires tests for features or an exact explanation of other verification.[^cpython-ai] [^curl-contribute]

#### 4. Licensing, authorship, and provenance

Require contributors to certify:

- They have the right to submit the entire contribution for distribution under AGPL-3.0.
- AI use does not transfer provenance risk to maintainers or change the project's license.
- They did not paste third-party code, documentation, tests, media, or data unless its source, authorship, and compatible license are identified and preserved as required.
- They will identify generated files and include their authoritative source, consistent with LogLeopard's existing OpenAPI/schema rules.
- If origin or license status is uncertain, the material must be rewritten from known sources or omitted.

This can be implemented as a contribution certification in `CONTRIBUTING.md` and a PR checkbox. A DCO/sign-off bot is an optional stronger mechanism if the project wants commit-level attestations; it is not required merely because AI is allowed. QEMU and Git demonstrate why provenance certification matters, while curl puts the burden to avoid unlicensed content on the contributor.[^qemu-provenance] [^git-submitting] [^curl-contribute]

Do not require an AI system to be listed as an author or co-author. The human submitter is the accountable author; Bitcoin Core explicitly rejects agent co-authorship for this reason.[^bitcoin-ai]

#### 5. Generated spam and maintainer burden

Prohibit:

- Bulk or autonomous issue/PR creation.
- Speculative bug or security reports without human reproduction.
- Generated reviews that restate a diff, add no project-specific evidence, or have not been checked.
- Large mechanical rewrites, style churn, or dependency changes without an accepted need.
- Pasted AI prose that is verbose, fabricated, or does not answer the template.
- Multiple near-duplicate submissions after closure.

Maintainers may close these without detailed review or explanation and may block repeat offenders. This follows CPython's closure/block discretion, curl's response to fabricated reports, Bitcoin Core's closure-without-notice rule, and Ghostty's protection of maintainer time.[^cpython-ai] [^curl-contribute] [^bitcoin-ai] [^ghostty-ai]

The policy should avoid insults or public denouncement. Closure, labels, moderation, and blocking are sufficient and easier to apply consistently.

#### 6. Security and privacy

LogLeopard needs stricter prompt-data rules than a generic library because its domain is cloud logs and local credentials. Require contributors not to provide any AI service with:

- Credentials, tokens, pairing URLs, cookies, or authentication material.
- Real cloud logs, query text from real investigations, provider filters, request/trace IDs, project IDs, screenshots containing them, or local profile files.
- Private vulnerability reports, unpatched exploit details, embargoed fixes, or security-advisory content unless the AI service is explicitly approved for that confidential use by the maintainer.
- Any non-public source, user data, or third-party confidential material they lack permission to disclose.

Use synthetic minimal reproductions, just as current `CONTRIBUTING.md` and `SECURITY.md` already require for repository submissions. AI-assisted security findings must be disclosed, reproduced by the human, and reported only through the private security channel. A generated claim without a working synthetic reproduction may be closed as unverified. curl's policy is a direct precedent for disclosure and human validation of AI-found security issues.[^curl-contribute]

This requirement concerns what contributors disclose to external tools, regardless of whether a provider claims not to train on prompts. It is intentionally enforceable as a contributor representation and as grounds for removing leaked material; maintainers cannot technically audit every private prompt.

#### 7. Maintainer discretion and equal standards

State explicitly:

- AI disclosure neither favors nor disqualifies a contribution.
- Compliance does not guarantee review or acceptance.
- Maintainers decide whether scope, quality, security, provenance, and review cost justify merging.
- Maintainers may request a smaller human-written reproduction, explanation, test, or rewrite.
- Maintainers may close or reject work when use is undisclosed, understanding is inadequate, claims cannot be reproduced, provenance is uncertain, sensitive data is exposed, or review cost exceeds project value.
- Repeated or intentional violations may lead to blocking.

This preserves discretion while tying decisions to published, observable criteria.

### Recommended policy text

The following compact text could be adapted into `CONTRIBUTING.md` later:

> AI-assisted contributions are allowed, but a human contributor must own the
> work. Disclose the AI tool and the extent of its use in issues, pull requests,
> and security reports. You must review and edit the result, understand and be
> able to explain every submitted change in your own words, personally respond
> to review, and complete the same tests and documentation required for any
> contribution. Autonomous or bulk-generated submissions are not accepted.
>
> By submitting, you certify that you have the right to contribute the complete
> work for distribution under AGPL-3.0 and that any reused material has known,
> compatible provenance. Do not submit output whose origin or license you cannot
> establish. Do not identify an AI tool as an author or transfer responsibility
> for the contribution to it.
>
> Never provide AI services with credentials, pairing URLs, real cloud logs,
> project identifiers, real investigation queries or filters, private
> vulnerability information, or other non-public data. Use synthetic fixtures
> and report vulnerabilities through the private process in `SECURITY.md`.
>
> Maintainers may close without detailed review any submission that is
> unexplained, unverified, unnecessarily broad, fabricated, unsafe, of uncertain
> provenance, or burdensome relative to its project value. Repeated or
> intentional violations may result in blocking. Disclosure does not guarantee
> acceptance and does not relax any contribution requirement.

### Practical enforcement points

If the policy is adopted in a later change, use all of these low-cost controls:

1. Add the normative policy and examples to `CONTRIBUTING.md`; link it from issue forms and the PR template.
2. Add required PR confirmations for AI disclosure (including `None`), human understanding/ownership, license/provenance, no sensitive data supplied to AI, and exact tests run.
3. Add issue/security-report prompts asking whether AI helped identify or draft the report and requiring a human-verified synthetic reproduction.
4. Keep existing CI required for merge. Add no "AI detector" check; enforce objective failures through tests, privacy review, explanation, provenance, and maintainer judgment.
5. Close autonomous, duplicate, fabricated, or non-responsive submissions with a short link to the policy. Escalate repeated intentional violations to blocking.
6. Treat an inability to explain a material change as a failed review, not as a mentoring obligation imposed on maintainers. Contributors can return with a smaller, understood change.
7. Keep all requirements applicable to maintainers where practical. If maintainers reserve exceptions, state them transparently; otherwise the policy risks appearing to regulate only outsiders.

## Suggested implementation order

This research did not modify release or contribution files. If implemented later, the smallest safe sequence is:

1. Add AI policy text to `CONTRIBUTING.md` and confirmations to the existing PR template.
2. Decide first-release status: `v0.1.0-rc.1` for public packaging validation or final `v0.1.0` after private validation.
3. Add version reporting to the binary so users can map an executable to tag and commit.
4. Add deterministic packaging/checksum scripts locally and test all advertised targets.
5. Add a tag-triggered release workflow with read-only defaults, a narrowly privileged publish job, pinned actions, complete CI gates, and draft publication.
6. Enable immutable releases before the first publication.
7. Publish, then independently download and verify all assets, checksums, embedded UI startup, and release attestations.

## Primary sources

All web sources were accessed on 2026-08-01.

[^about-releases]: GitHub Docs, [About releases](https://docs.github.com/en/repositories/releasing-projects-on-github/about-releases): releases are based on Git tags; releases package notes and binary files; security fixes should have a repository security advisory.

[^managing-releases]: GitHub Docs, [Managing releases in a repository](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository): tags, assets, prerelease and latest controls, drafts, and immutable-release workflow.

[^generated-release-notes]: GitHub Docs, [Automatically generated release notes](https://docs.github.com/en/repositories/releasing-projects-on-github/automatically-generated-release-notes): generated PR/contributor/changelog content and `.github/release.yml` categories and exclusions.

[^immutable-releases]: GitHub Docs, [Immutable releases](https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases): locked tags/assets, automatic release attestations, and draft-then-publish best practice.

[^prevent-release-changes]: GitHub Docs, [Preventing changes to your releases](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/establish-provenance-and-integrity/prevent-release-changes): repository/organization enablement and prospective-only effect.

[^verify-release]: GitHub Docs, [Verifying the integrity of a release](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/verify-release-integrity): `gh release verify` and `gh release verify-asset`.

[^release-assets-api]: GitHub Docs, [REST API endpoints for release assets](https://docs.github.com/en/rest/releases/assets): release asset response schema includes `name`, `size`, and `digest`.

[^workflow-permissions]: GitHub Docs, [Workflow syntax: `permissions`](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions#permissions): minimum permissions, unspecified permissions become `none`, and `contents: write` allows release creation.

[^github-token]: GitHub Docs, [Use `GITHUB_TOKEN` for authentication in workflows](https://docs.github.com/en/actions/security-for-github-actions/security-guides/automatic-token-authentication): actions can access `github.token`; grant least required access.

[^actions-security]: GitHub Docs, [Secure use reference](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions): least privilege, untrusted checkout risks, and full-SHA action pinning.

[^artifact-attestations]: GitHub Docs, [Artifact attestations](https://docs.github.com/en/actions/concepts/security/artifact-attestations): provenance contents, Sigstore behavior, recommended subjects, verification, and limitations.

[^artifact-attestations-howto]: GitHub Docs, [Using artifact attestations to establish provenance for builds](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations): binary attestation permissions, `actions/attest`, and CLI verification.

[^gh-release-create]: GitHub CLI manual, [`gh release create`](https://cli.github.com/manual/gh_release_create): `--verify-tag`, generated notes, prerelease, draft, asset upload, and immutable-release behavior.

[^gh-verify-asset]: GitHub CLI manual, [`gh release verify-asset`](https://cli.github.com/manual/gh_release_verify-asset): signed release attestation and digest verification.

[^go-version-numbers]: Go Documentation, [Module version numbering](https://go.dev/doc/modules/version-numbers): semantic version meanings, `v` prefix, prereleases, and major-version module paths.

[^go-publishing]: Go Documentation, [Publishing a module](https://go.dev/doc/modules/publishing): final tidy/test, `v0.1.0` tag example, and warning not to change published tags.

[^cpython-ai]: Python Developer's Guide source, [Guidelines for using AI tools](https://github.com/python/devguide/blob/main/getting-started/ai-tools.rst): responsibility, review, explanation, tests, closure, and blocking.

[^curl-contribute]: curl repository, [`docs/CONTRIBUTE.md`](https://github.com/curl/curl/blob/master/docs/CONTRIBUTE.md), especially "On AI use in curl," "License and copyright," and "Test Cases": disclosure and verification of AI-found issues, fabricated-report sanctions, licensing burden, and unchanged quality/test requirements.

[^bitcoin-ai]: Bitcoin Core repository, [`doc/AI_POLICY.md`](https://github.com/bitcoin/bitcoin/blob/master/doc/AI_POLICY.md): human understanding, human-authored communication, no autonomous agents, no AI co-authors, and closure discretion. See also [`CONTRIBUTING.md`](https://github.com/bitcoin/bitcoin/blob/master/CONTRIBUTING.md) for explanation, testing, and maintainer discretion.

[^ghostty-ai]: Ghostty repository, [`AI_POLICY.md`](https://github.com/ghostty-org/ghostty/blob/main/AI_POLICY.md): mandatory tool/extent disclosure, complete human understanding, human review/editing, and maintainer-time rationale.

[^ghostty-contributing]: Ghostty repository, [`CONTRIBUTING.md`](https://github.com/ghostty-org/ghostty/blob/main/CONTRIBUTING.md): the critical understanding rule and first-contributor enforcement.

[^git-submitting]: Git repository, [`Documentation/SubmittingPatches`](https://github.com/git/git/blob/master/Documentation/SubmittingPatches), "Use of Artificial Intelligence (AI)" and DCO sections: origin/right-to-submit certification, rejection of slop or unexplained output, and responsible uses.

[^qemu-provenance]: QEMU repository, [`docs/devel/code-provenance.rst`](https://github.com/qemu/qemu/blob/master/docs/devel/code-provenance.rst), "Use of AI-generated content," DCO, and generated-file sections: current prohibition, rationale, non-content exceptions, and responsibility for accepted exceptions.

[^logleopard-license]: LogLeopard, [`LICENSE`](../../LICENSE), AGPL-3.0 sections 1, 6, and 13; especially lines 235-274 in the audited working tree for conveying object code and Corresponding Source. See also [`README.md`](../../README.md), [`CONTRIBUTING.md`](../../CONTRIBUTING.md), [`SECURITY.md`](../../SECURITY.md), [`AGENTS.md`](../../AGENTS.md), [`Makefile`](../../Makefile), [`web/embed_production.go`](../../web/embed_production.go), and [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) for repository-specific findings.
