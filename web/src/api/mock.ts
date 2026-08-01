import { fixtureEntries } from "./fixtures";
import type { ApiClient, Profile, ProfileInput, QueryRequest } from "./types";

let profiles: Profile[] = [
  {
    id: "local-demo",
    name: "Production read-only",
    projectId: "synthetic-prod-01",
    provider: "gcp",
    status: "ready",
  },
  {
    id: "staging",
    name: "Staging",
    projectId: "synthetic-stg-01",
    provider: "gcp",
    status: "ready",
  },
];

const pause = (ms = 180) => new Promise((resolve) => setTimeout(resolve, ms));

export const mockClient: ApiClient = {
  async pair() {
    await pause(250);
    return { paired: true };
  },
  async authStatus() {
    await pause(30);
    return { available: true, message: "Synthetic credentials are available." };
  },
  async profiles() {
    await pause(50);
    return profiles;
  },
  async saveProfile(input: ProfileInput, id?: string) {
    await pause();
    const profile: Profile = {
      id: id ?? `profile-${Date.now()}`,
      ...input,
      provider: "gcp",
      status: "ready",
    };
    profiles = id
      ? profiles.map((item) => (item.id === id ? profile : item))
      : [...profiles, profile];
    return profile;
  },
  async sources(profileId) {
    await pause(120);
    if (profileId === "staging")
      return {
        sources: [],
        warning:
          "No concrete Cloud Run services were discovered. Queries will use all Cloud Run logs.",
      };
    return {
      sources: [
        { id: "payments-api", label: "payments-api", kind: "cloud-run" },
        { id: "edge-router", label: "edge-router", kind: "cloud-run" },
        { id: "nightly-worker", label: "nightly-worker", kind: "cloud-run" },
      ],
    };
  },
  async query(input: QueryRequest, signal?: AbortSignal) {
    await pause(420);
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const query = input.query ?? "";
    const selectedSources = input.sources ?? [];
    const selectedSeverities = input.severities ?? [];
    if (query.includes("mock:error"))
      throw new Error("Synthetic provider error. Try removing mock:error.");
    if (query.includes("mock:expired"))
      return {
        entries: [],
        expiresAt: new Date(Date.now() - 1_000).toISOString(),
      };
    const offset = Number(input.cursor ?? 0);
    const terms =
      input.mode === "native"
        ? []
        : query
            .toLowerCase()
            .split(/\s+/)
            .filter((term) => term && !term.includes(":"));
    const start = new Date(input.start).getTime();
    const end = new Date(input.end).getTime();
    const requiresServerError =
      input.mode === "native" &&
      query.trim() === "httpRequest.status >= 500 AND httpRequest.status < 600";
    const filtered = fixtureEntries.filter((entry) => {
      const status = Number(
        (entry.raw as { httpRequest?: { status?: unknown } }).httpRequest
          ?.status,
      );
      return (
        new Date(entry.timestamp).getTime() >= start &&
        new Date(entry.timestamp).getTime() < end &&
        (!selectedSources.length || selectedSources.includes(entry.source)) &&
        (!selectedSeverities.length ||
          selectedSeverities.includes(entry.severity)) &&
        (!requiresServerError || status >= 500) &&
        terms.every((term) => entry.message.toLowerCase().includes(term))
      );
    });
    const entries = filtered.slice(offset, offset + input.limit);
    const next = offset + entries.length;
    return {
      entries,
      ...(next < filtered.length ? { nextCursor: String(next) } : {}),
      expiresAt: new Date(Date.now() + 15 * 60_000).toISOString(),
    };
  },
  async requestContext(input, signal) {
    await pause(240);
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const center = new Date(input.eventTimestamp).getTime();
    return {
      entries: fixtureEntries
        .filter(
          (entry) =>
            Math.abs(new Date(entry.timestamp).getTime() - center) <=
            15 * 60_000,
        )
        .slice(0, 12)
        .sort((a, b) => a.timestamp.localeCompare(b.timestamp)),
    };
  },
  async serviceHealth(input, signal) {
    await pause(260);
    if (signal?.aborted) throw new DOMException("Aborted", "AbortError");
    const start = new Date(input.start).getTime();
    const end = new Date(input.end).getTime();
    const alignmentSeconds = Math.max(
      60,
      Math.ceil((end - start) / 300 / 60_000) * 60,
    );
    const count = Math.min(
      300,
      Math.floor((end - start) / (alignmentSeconds * 1000)),
    );
    const requests = Array.from({ length: count }, (_, index) => ({
      timestamp: new Date(
        start + (index + 1) * alignmentSeconds * 1000,
      ).toISOString(),
      value: 28 + ((index * 7) % 23),
    }));
    const serverErrors = requests.map((point, index) => ({
      timestamp: point.timestamp,
      value:
        index >= Math.floor(count * 0.55) && index <= Math.floor(count * 0.62)
          ? 2 + (index % 4)
          : index % 17 === 0
            ? 1
            : 0,
    }));
    const latencyP95 = requests.map((point, index) => ({
      timestamp: point.timestamp,
      value: 95 + ((index * 19) % 180),
    }));
    return {
      service: input.service,
      start: input.start,
      end: input.end,
      alignmentSeconds,
      series: [
        { name: "request_count", unit: "1", points: requests },
        { name: "server_error_count", unit: "1", points: serverErrors },
        { name: "request_latency_p95", unit: "ms", points: latencyP95 },
      ],
    };
  },
};
