import type { LogEntry, Severity } from "./types";

const messages = [
  "Request completed in 42ms",
  "Worker instance started",
  "Cache miss for /v1/catalog",
  "Deadline exceeded while contacting upstream",
  "Health check passed",
  'User payload: <img src=x onerror=alert(1)> & "quoted"',
  "Revision traffic allocation updated",
  "Database connection pool at 78%",
];
const severities: Severity[] = [
  "INFO",
  "NOTICE",
  "DEBUG",
  "ERROR",
  "INFO",
  "WARNING",
  "NOTICE",
  "CRITICAL",
];

export const fixtureEntries: LogEntry[] = Array.from(
  { length: 140 },
  (_, index) => {
    const message = messages[index % messages.length]!;
    const severity = severities[index % severities.length]!;
    const source =
      index % 3 === 0
        ? "payments-api"
        : index % 3 === 1
          ? "edge-router"
          : "nightly-worker";
    return {
      id: `entry-${index}`,
      timestamp: new Date(Date.now() - index * 17_000).toISOString(),
      severity,
      severityOriginal: severity,
      source,
      sourceOriginal: `projects/synthetic/logs/${source}`,
      message,
      messageSource: "jsonPayload.message",
      labels: {
        revision: `${source}-000${(index % 4) + 1}`,
        region: "europe-west1",
      },
      structured: {
        message,
        requestId: `req_${String(index).padStart(5, "0")}`,
        latencyMs: 12 + index,
        ok: severity !== "ERROR",
      },
      requestId: `req_${String(index).padStart(5, "0")}`,
      httpRequest: {
        method: "GET",
        url: `https://synthetic.example.test/v1/items/${index}`,
        status: severity === "ERROR" || severity === "CRITICAL" ? 500 : 200,
        latency: `0.${String(12 + index).padStart(3, "0")}s`,
      },
      ...(index % 2 === 0
        ? {
            trace: `projects/synthetic-prod-01/traces/trace-${Math.floor(index / 2)}`,
          }
        : {}),
      raw: {
        insertId: `synthetic-${index}`,
        severity,
        textPayload: message,
        httpRequest: {
          status: severity === "ERROR" || severity === "CRITICAL" ? 500 : 200,
        },
        resource: {
          type: "cloud_run_revision",
          labels: { service_name: source },
        },
      },
    };
  },
);
