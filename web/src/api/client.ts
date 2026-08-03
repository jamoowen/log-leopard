import {
  ApiError,
  type ApiClient,
  type FleetOverviewRequest,
  type Problem,
  type ProfileInput,
  type QueryRequest,
  type RequestContextRequest,
  type ServiceHealthRequest,
  type SourceDiscovery,
  type SourceListResponse,
} from "./types";
import { mockClient } from "./mock";

async function requestWithResponse<T>(
  path: string,
  init?: RequestInit,
): Promise<{ data: T; response: Response }> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Content-Type"))
    headers.set("Content-Type", "application/json");
  const response = await fetch(`/api/v1${path}`, {
    ...init,
    credentials: "include",
    headers,
  });
  if (!response.ok) {
    let problem: Partial<Problem> = {};
    try {
      problem = (await response.json()) as Partial<Problem>;
    } catch {
      // Error responses are not guaranteed to contain a JSON problem body.
    }
    throw new ApiError(
      problem.detail ?? problem.title ?? `Request failed (${response.status})`,
      response.status,
    );
  }
  const data =
    response.status === 204 ? (undefined as T) : ((await response.json()) as T);
  return { data, response };
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return (await requestWithResponse<T>(path, init)).data;
}

export async function requestSourceDiscovery(
  profileId: string,
): Promise<SourceDiscovery> {
  const { data, response } = await requestWithResponse<SourceListResponse>(
    `/sources?profileId=${encodeURIComponent(profileId)}`,
  );
  const warning = response.headers.get("X-LogLeopard-Warning")?.trim();
  const warningCode = response.headers.get("X-LogLeopard-Warning-Code")?.trim();
  return {
    sources: data,
    ...(warning ? { warning } : {}),
    ...(warningCode === "authentication" ? { warningCode } : {}),
  };
}

const httpClient: ApiClient = {
  pair: (token) =>
    request("/session/pair", {
      method: "POST",
      body: JSON.stringify({ token }),
    }),
  authStatus: () => request("/auth/status"),
  profiles: () => request("/profiles"),
  saveProfile: (input: ProfileInput, id?: string) =>
    request(`/profiles${id ? `/${encodeURIComponent(id)}` : ""}`, {
      method: id ? "PUT" : "POST",
      body: JSON.stringify(input),
    }),
  sources: requestSourceDiscovery,
  query: (input: QueryRequest, signal) =>
    request("/query", {
      method: "POST",
      body: JSON.stringify(input),
      ...(signal ? { signal } : {}),
    }),
  requestContext: (input: RequestContextRequest, signal) =>
    request("/request-context", {
      method: "POST",
      body: JSON.stringify(input),
      ...(signal ? { signal } : {}),
    }),
  serviceHealth: (input: ServiceHealthRequest, signal) =>
    request("/service-health", {
      method: "POST",
      body: JSON.stringify(input),
      ...(signal ? { signal } : {}),
    }),
  fleetOverview: (input: FleetOverviewRequest, signal) =>
    request("/fleet-overview", {
      method: "POST",
      body: JSON.stringify(input),
      ...(signal ? { signal } : {}),
    }),
};

export function shouldUseMockApi(
  search: string,
  environmentEnabled: string | undefined,
  development: boolean,
) {
  return (
    development &&
    (new URLSearchParams(search).get("mock") === "1" ||
      environmentEnabled === "true")
  );
}

export const api = shouldUseMockApi(
  window.location.search,
  import.meta.env.VITE_MOCK_API,
  import.meta.env.DEV,
)
  ? mockClient
  : httpClient;
