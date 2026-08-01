# Contributing

LogLeopard accepts focused bug fixes, security improvements, tests, and changes that strengthen its local-first Cloud Run log workflow. Discuss broad product changes before implementation; ingestion, hosted credentials, additional providers, metrics, alerts, and AI analysis are intentionally outside the current scope.

Open an issue before investing in a broad feature, dependency change, or architectural rewrite. Keep pull requests focused, explain the user or security need, and add regression tests for fixes or behavior tests for features.

## Development

Install Go 1.26.5+, Node.js 22, pnpm 11.8.0, `golangci-lint` v2.10.1, and the locked frontend dependencies:

```sh
pnpm --dir web install --frozen-lockfile
```

Run the complete local gate before opening a pull request:

```sh
make check
go test -race ./...
```

Run `pnpm --dir web run test:e2e` when browser workflows change. Playwright requires Chromium installed through `pnpm --dir web exec playwright install chromium`.

Pull requests run four independent CI jobs: Go checks, frontend and API contract checks, browser workflows, and dependency vulnerability checks. Separate jobs run in parallel and make failures easier to diagnose. Dependabot also checks Go modules, frontend packages, and GitHub Actions weekly; each update pull request then runs the same CI as any other contribution, so several runs can appear together when updates are first enabled.

## Generated API files

Go API types are authoritative. If an API type or route changes, run `make api` and include both `openapi.json` and `web/src/api/schema.d.ts`. Do not edit either generated file manually.

## Test data and privacy

Use synthetic fixtures only. Never include credentials, pairing tokens, project IDs, query text from real investigations, provider filters containing identifiers, or returned cloud logs in code, tests, screenshots, issues, or pull requests.

Report security vulnerabilities privately as described in `SECURITY.md`.

## AI-assisted contributions

AI assistance is allowed, but a human contributor must own the work. In every materially AI-assisted issue, pull request, security report, or substantial review, disclose the tool and how it was used. `AI assistance: none` is sufficient when none was used. Do not include prompts or transcripts.

The contributor must review and edit the result, understand and be able to explain every submitted change in their own words, personally respond to review, and complete the same tests and documentation required for any contribution. Autonomous or bulk-generated issues, pull requests, reviews, and security reports are not accepted. Do not use an AI tool as an author or co-author.

By submitting a contribution, you certify that you have the right to contribute the complete work for distribution under AGPL-3.0. Identify the source and compatible license of reused material. Do not submit generated or copied content whose origin or license you cannot establish.

Never provide an AI service with credentials, pairing URLs, cookies, real cloud logs, project identifiers, investigation queries or filters, private vulnerability information, non-public source, or other confidential data. Use synthetic fixtures and the private reporting process in `SECURITY.md`.

Maintainers may close without detailed review work that is unexplained, unverified, unnecessarily broad, fabricated, unsafe, of uncertain provenance, or disproportionately costly to review. Repeated or intentional violations may result in blocking. Disclosure does not guarantee acceptance or relax any contribution requirement.
