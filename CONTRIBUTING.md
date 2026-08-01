# Contributing

LogLeopard accepts focused bug fixes, security improvements, tests, and changes that strengthen its local-first Cloud Run log workflow. Discuss broad product changes before implementation; ingestion, hosted credentials, additional providers, metrics, alerts, and AI analysis are intentionally outside the current scope.

## Development

Install Go 1.26, Node.js 22, pnpm 11.8.0, `golangci-lint` v2.10.1, and the locked frontend dependencies:

```sh
pnpm --dir web install --frozen-lockfile
```

Run the complete local gate before opening a pull request:

```sh
make check
go test -race ./...
```

Run `pnpm --dir web run test:e2e` when browser workflows change. Playwright requires Chromium installed through `pnpm --dir web exec playwright install chromium`.

## Generated API files

Go API types are authoritative. If an API type or route changes, run `make api` and include both `openapi.json` and `web/src/api/schema.d.ts`. Do not edit either generated file manually.

## Test data and privacy

Use synthetic fixtures only. Never include credentials, pairing tokens, project IDs, query text from real investigations, provider filters containing identifiers, or returned cloud logs in code, tests, screenshots, issues, or pull requests.

Report security vulnerabilities privately as described in `SECURITY.md`.
