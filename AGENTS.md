# Agent Notes

## Product

- LogLeopard is a local-first browser UI for querying and reading cloud logs.
- GCP Cloud Logging and Cloud Run are the only providers implemented initially.
- Cloud logs remain the source of truth. Do not add ingestion, a log database, metrics, alerts, AI analysis, or AWS support without an explicit requirement.

## Architecture

- The Go backend owns credentials, provider calls, query compilation, normalization, local profiles, and API validation.
- The React app under `web/` owns presentation and browser-local preferences. It must never receive cloud credentials.
- Keep provider seams narrow: discovery, query, opaque cursor, normalized entry, raw entry, and capabilities.
- GCP-specific semantics belong under `internal/provider/gcp`; do not force native provider filters into a universal query model.
- Go API types are authoritative. Huma emits OpenAPI; `openapi-typescript` generates `web/src/api/schema.d.ts`. Never edit generated files manually.

## Security And Privacy

- Bind production only to numeric loopback addresses. Never add permissive CORS.
- Require the local pairing session for every data and configuration endpoint.
- Treat all log contents and query text as hostile input. Render text, never injected HTML.
- Keep the CSP restrictive and allowlist external link destinations.
- Cloud operations are read-only. Local profile configuration may be written to the OS config directory.
- Never persist returned log entries or print credentials, query text, project IDs, filters, or log contents in application logs.
- Do not accept service-account key uploads.

## Go

- Use idiomatic Go 1.26.5+, `context.Context`, `log/slog`, and standard library facilities where practical.
- Keep handlers thin and errors explicit. Wrap errors with context and expose sanitized problem responses.
- Define interfaces where consumed. Prefer concrete types until an interface is needed for provider tests.
- Keep `golangci-lint` at v2.10.1 locally to match CI. Do not add development tools to the application module graph.
- Run `golangci-lint run`, `go vet ./...`, and `go test ./...` after Go changes; use `go test -race ./...` for concurrency-sensitive or final verification.

## Frontend

- Use React, TypeScript, Vite, Tailwind CSS, Radix primitives, and TanStack Query.
- Preserve the precision-tool visual language: dense, calm, typography-led, dark-first, and keyboard accessible.
- Desktop is primary, but core workflows must remain functional on narrow screens.
- Do not introduce stock component-library themes or unsafe HTML rendering.
- Prefer `async`/`await` for sequential asynchronous control flow. Use promise combinators when work is genuinely concurrent or compositional.
- Keep TypeScript strict and do not weaken lint, compiler, accessibility, or formatting rules to avoid fixing a finding. Document narrow exceptions inline.
- Use pnpm 11.8.0 through the `packageManager` declaration. Do not add npm or Bun lockfiles.
- Run `pnpm run format:check`, `pnpm run lint`, `pnpm run typecheck`, `pnpm run test`, and `pnpm run build` from `web/` after frontend changes.

## Testing

- Use synthetic fixtures only. Never commit production logs, identifiers, credentials, or tokens.
- Keep CI independent of GCP. Authenticated smoke tests must be explicit, local, read-only, and must not record responses.
- Test query escaping, fixed pagination windows, cursor binding, normalization, hostile payload rendering, and API rejection paths.

## Documentation And Git

- Keep `README.md` accurate when setup, commands, architecture, security, or behavior changes.
- Follow `CONTRIBUTING.md` for AI-assisted work and `RELEASING.md` for release preparation.
- `CLAUDE.md` imports this file; keep shared instructions here rather than duplicating them.
- Do not edit generated OpenAPI or TypeScript schema output manually.
- Run `make check` before handing work back. CI additionally runs the race detector, Playwright, and vulnerability scans.
- Do not commit, push, publish, or create releases unless explicitly requested.
