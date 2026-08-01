# LogLeopard

<p align="center">
  <img src="web/public/log-leopard.png" width="220" alt="LogLeopard logo">
</p>

LogLeopard is a local-first browser UI for finding and reading Google Cloud logs. It aims to make focused log investigation fast without copying cloud logs or credentials into another hosted service.

## Goals

- Provide a dense, keyboard-friendly interface for Cloud Run log investigation.
- Keep cloud credentials in a loopback-only Go process and cloud logs in Cloud Logging.
- Preserve useful provider detail while normalizing common fields for reading and filtering.
- Make query scope, pagination, and local persistence explicit and bounded.
- Provide a zero-configuration Cloud Run service-health view that drills directly into the corresponding logs.

## Non-goals

LogLeopard is not a log ingestion platform, database, general-purpose metrics/dashboard builder, alerting system, observability agent, or AI analysis service. Its service-health view uses a fixed, bounded set of built-in Cloud Run metrics. It does not currently support providers other than GCP or resources other than Cloud Run.

## Architecture

The Go backend owns Application Default Credentials (ADC), GCP discovery, fixed Cloud Monitoring queries, Cloud Logging queries, query compilation, validation, normalized responses, opaque cursors, pairing sessions, and local connection profiles. The React/Vite frontend owns presentation and browser-local preferences; it receives neither cloud credentials nor provider credentials. Cloud Logging and Cloud Monitoring remain the sources of truth.

Go API types are authoritative. Huma generates the committed `openapi.json`, and the pinned `openapi-typescript` version generates the committed `web/src/api/schema.d.ts`. Keeping both artifacts in Git makes frontend installs and Node-only builds deterministic. Run `make api` after changing the Go API and `make api-check` to detect drift without modifying either artifact.

Production builds embed `web/dist` in the Go binary. Development Go builds use an empty asset filesystem, so Go tests do not require Node or a frontend build.

## Install

Until the first package is published, build from source using the requirements below. Tagged releases provide self-contained archives for macOS, Linux, and Windows from the [GitHub Releases page](https://github.com/jamoowen/log-leopard/releases); the executable contains the web UI and needs no Node.js runtime. Release binaries are not currently code-signed or notarized.

After extracting an archive, authenticate with ADC as described below and run `./log-leopard` (`log-leopard.exe` on Windows). Use `./log-leopard -version` to identify a packaged build. Verify downloaded archives against the attached `SHA256SUMS` before running them.

## Requirements

- Go 1.26.5 or newer in the 1.26 release line
- Node.js 22.12+ and pnpm 11.8.0
- Google Cloud CLI for live GCP authentication
- `golangci-lint` v2.10.1 for the complete local quality gate

Install frontend dependencies once:

```sh
pnpm --dir web install --frozen-lockfile
```

## GCP access

LogLeopard uses ADC. ADC supports user credentials, service-account impersonation, and service-account key files referenced through `GOOGLE_APPLICATION_CREDENTIALS`. LogLeopard does not accept key uploads through the browser or copy private keys into its own configuration. For user credentials:

```sh
gcloud auth application-default login
```

An existing service-account key file also works through ADC without entering the browser or LogLeopard configuration:

```sh
GOOGLE_APPLICATION_CREDENTIALS=/path/to/read-only-service-account.json make dev
```

Prefer impersonation where possible because service-account keys are long-lived credentials that must be separately protected, rotated, and revoked.

Grant the authenticated principal these minimum project-level roles on every project it will query:

- `roles/logging.viewer` to discover and read log entries.
- `roles/run.viewer` to discover Cloud Run services.
- `roles/monitoring.viewer` to read the built-in service-health metrics.

Organization policies or custom roles can require additional permissions. If ADC warns about quota, set a quota project with `gcloud auth application-default set-quota-project PROJECT_ID`; that may require `serviceusage.services.use`, included in `roles/serviceusage.serviceUsageConsumer`, on the quota project.

For keyless read-only impersonation, grant the service account the two viewer roles above and grant the developer `roles/iam.serviceAccountTokenCreator` on that service account, then create impersonated ADC:

```sh
gcloud auth application-default login \
  --impersonate-service-account=log-leopard-reader@PROJECT_ID.iam.gserviceaccount.com
```

Impersonation avoids long-lived service-account keys. The impersonated account should have no write roles.

## Run locally

For the packaged-style app, the backend prints a loopback pairing URL to copy into any browser. These commands run the embedded production UI without a separate frontend process:

```sh
make run                          # live GCP through ADC
make run-fake                     # synthetic data, no GCP access
go run -tags production ./cmd/log-leopard -open  # open after make web-build
```

For frontend development, one command builds and supervises the backend and Vite, waits for both, prints the correctly paired Vite URL, and stops both processes on exit:

```sh
make dev       # live GCP through ADC
make dev-fake  # synthetic data, no GCP access
make dev BROWSER=brave  # open the pairing URL in Brave
```

`BROWSER` accepts `brave`, `chrome`, `firefox`, or `safari` on macOS. Without it, LogLeopard prints the URL for you to open in any browser.

The root Vite config proxies `/api` and `/openapi.json` to `http://127.0.0.1:8787`. The lower-level `make web-dev` command remains available when intentionally managing the backend separately; set `LOG_LEOPARD_DEV_BACKEND` to change its proxy target.

Local browser pairing happens before and independently of Google Cloud authentication. Pairing links are single-use and expire after ten minutes. If pairing fails, stop the running command, run `make dev` again, and open only the newest URL it prints; ADC or permission problems appear separately after pairing succeeds.

Connection profiles store a display name and GCP project ID, never credentials.

## Query language

Queries require absolute `start` and `end` timestamps and are limited to seven days. Page sizes are 1 to 200, normalized responses are capped at 4 MiB, and opaque short-lived cursors are bound to the connection, compiled filter, time window, and page size.

The search syntax supports:

- Free or quoted text: `timeout "connection reset"`
- Implicit `AND`: `timeout service:checkout`
- Aliases: `message`/`msg`, `service`/`source`, `severity`/`level`, `status`/`http.status_code`, and `trace`
- Field negation: `-@http.method:POST`
- JSON paths: `@request.user.id:123` or `json.request.user.id:123`

Explicit `AND`, `OR`, parentheses, and negated free text are rejected. Prefix a JSON path with `@` or `json.` to bypass aliases. Every query enforces `resource.type="cloud_run_revision"` and the requested time window. Native mode accepts a complete GCP Logging filter, validates balanced quotes and parentheses, and still encloses it within those fixed constraints.

Structured mode uses a provider-neutral `predicates` array instead of `query`. Each predicate has `path`, `operator`, and `value`; paths are relative to `jsonPayload` and contain up to 20 dot-separated identifier segments. Up to 50 predicates are combined with `AND`. The exact operators and value types are:

- `equals`: string, finite number, or boolean
- `contains`: non-empty string
- `exists`: boolean (`false` means the field must not exist)
- `gt` and `lt`: finite number

`query` is accepted only in `leopard` and `native` modes, while `predicates` is accepted only in `structured` mode. Sources, severities, and the fixed Cloud Run/time constraints apply in all three modes.

The authenticated request-context API is `POST /api/v1/request-context` with `profileId`, `eventTimestamp`, and at least one of `requestId` or `traceId`. It exact-matches only the backend's known request/trace locations, across all Cloud Run revisions and all services in the profile project, from 15 minutes before through 15 minutes after the selected timestamp. It returns `{ "entries": [...] }` with at most 200 entries in ascending timestamp order and the same 4 MiB response bound as normal queries. Active source, severity, and query filters are deliberately not applied.

Message normalization checks common JSON message fields before `textPayload` and compact JSON fallback. Responses retain the selected message path, normalized and original severity, common HTTP/trace/request metadata, and the complete raw entry.

## Daily-use workflow

- Compact, structured, and raw result modes share a virtualized message-led grid and detailed entry inspector.
- The structured builder creates validated `jsonPayload` predicates without requiring GCP filter syntax.
- Loaded structured payloads feed a bounded field browser. Fields can be promoted into filters or pinned into compact rows per connection.
- Named saved-query recipes retain a connection, relative time preset, sources, filters, query mode, and display mode. Absolute timestamps and results are never saved.
- Optional 5, 10, or 30 second polling submits a fresh absolute query window on each tick. Polling pauses while the tab is hidden or an entry is open for inspection.
- Entries with a request or trace identifier expose project-wide request context in chronological order.
- `Cmd/Ctrl+K` opens the command palette; `/` focuses the text query and `Cmd/Ctrl+Enter` runs it.
- Service Health shows fixed request counts, observed 5xx counts, and service-wide p95 container request latency for an explicitly selected Cloud Run service over a one-hour, six-hour, 24-hour, or seven-day window. The selected target and window are remembered locally per connection; metric results are queried on demand, are not persisted, and can be delayed by approximately two minutes.
- Selecting the highest-error interval opens a correlated timeline, then prepares an exact service/time/HTTP 5xx query in Logs.

## Build and test

```sh
make api                # regenerate OpenAPI from Go, then TypeScript types
make api-check          # check both committed contracts without updating them
make fmt                # format Go and frontend source
make check              # format, lint, vet, tests, frontend build, and API drift
go test -race ./...     # optional Go race check
make build              # development Go binary
make build-production   # frontend build plus production-tagged embedded binary
pnpm --dir web run test:e2e  # optional Playwright suite; requires installed browser
```

`make check` deliberately excludes Playwright because browser installation is an additional environment requirement. CI runs the race detector, desktop and mobile Playwright workflows, Go vulnerability analysis, and a production dependency audit in addition to the local gate. The live schema is available from the backend at `GET /openapi.json`.

## Privacy and security

- The server binds to a numeric loopback address and does not enable permissive CORS.
- A random ten-minute, single-use URL-fragment token creates a twelve-hour `HttpOnly`, `SameSite=Strict` session. Protected APIs require that session, and mutations also require the exact expected `Origin`.
- Profiles are atomically stored with mode `0600` under the OS user config directory (`LogLeopard/connections.json`) unless `-config` overrides it.
- Query results remain in memory and are never persisted. Display preferences and per-connection field pins are stored locally. Query history, named recipes, and current draft restoration are separate opt-in controls that default off; disabling history or recipes deletes their stored data, and the UI can clear all local query data at once.
- Application logs exclude credentials, project IDs, query text, provider filters, and returned log contents.
- Log content is untrusted data and is rendered as text rather than injected HTML.

## Current limitations and roadmap

The current vertical slice supports one local user, GCP Cloud Logging, a bounded Cloud Run request-health view, Cloud Run discovery, bounded text/structured/native querying, saved recipes, polling, field discovery, normalized/raw entry inspection, request context, and cursor pagination. Discovery can fall back to an all-logs source, but health requires one concrete service and log queries remain constrained to Cloud Run revisions. P95 request latency merges distributions across revisions before percentile conversion; CPU, memory, instance, and startup metrics remain deferred until their aggregation semantics can be represented correctly. There is no updater, installer, or signed/notarized package yet, and broad accessibility, operating-system packaging, and authenticated live-GCP smoke coverage remain works in progress.

Near-term work is to harden the Cloud Run workflow, expand query and rendering tests, improve keyboard/accessibility behavior, and produce reproducible cross-platform releases. Additional providers or resource types should be added only after the provider boundary and user need are proven.

## Contributing and releases

See [CONTRIBUTING.md](CONTRIBUTING.md) for development expectations, privacy rules, and the AI-assisted contribution policy. Maintainers should follow [RELEASING.md](RELEASING.md); tagged releases are built as drafts and require human review before publication.

## License and naming

LogLeopard is free software licensed under the [GNU Affero General Public License v3.0](LICENSE). If you modify it and make that version available to users over a network, the AGPL requires offering those users the corresponding source; see the license for the complete terms.

“LogLeopard” is a project name, not an affiliation or endorsement. Google Cloud, Cloud Logging, Cloud Run, and related marks are trademarks of Google LLC. This project is not affiliated with or endorsed by Google.
