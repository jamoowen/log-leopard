import type { components, paths } from "./schema";

type JsonResponse<
  Path extends keyof paths,
  Method extends keyof paths[Path],
> = paths[Path][Method] extends {
  responses: { 200: { content: { "application/json": infer Body } } };
}
  ? Body
  : never;

export type Profile = components["schemas"]["ProfileResponse"];
export type ProfileInput = components["schemas"]["ProfileBody"];
export type ProfileListResponse = JsonResponse<"/api/v1/profiles", "get">;
export type Source = components["schemas"]["SourceResponse"];
export type SourceListResponse = JsonResponse<"/api/v1/sources", "get">;
export interface SourceDiscovery {
  sources: SourceListResponse;
  warning?: string;
  warningCode?: "authentication";
}
export type QueryRequest = components["schemas"]["QueryInputBody"];
export type FieldPredicate = components["schemas"]["FieldPredicate"];
export type QueryResponseDto = components["schemas"]["QueryOutputBody"];
export type RequestContextRequest =
  components["schemas"]["RequestContextInputBody"];
export type RequestContextResponse =
  components["schemas"]["RequestContextOutputBody"];
export type ServiceHealthRequest =
  components["schemas"]["ServiceHealthInputBody"];
export type ServiceHealthResponse =
  components["schemas"]["ServiceHealthOutputBody"];
export type HealthPoint = components["schemas"]["HealthPoint"];
export type FleetOverviewRequest =
  components["schemas"]["FleetOverviewInputBody"];
export type FleetOverviewResponse =
  components["schemas"]["FleetOverviewOutputBody"];
export type FleetOverviewSummary =
  components["schemas"]["FleetOverviewSummary"];
export type PairResponse = components["schemas"]["CookieOutputBody"];
export type AuthStatus = components["schemas"]["AuthStatusOutputBody"];
export type Problem = components["schemas"]["ErrorModel"];

export type LogEntry = components["schemas"]["Entry"];

export type QueryResponse = Omit<QueryResponseDto, "entries"> & {
  entries: LogEntry[] | null;
};

export type QueryMode = QueryRequest["mode"];
export type Severity =
  | "DEFAULT"
  | "DEBUG"
  | "INFO"
  | "NOTICE"
  | "WARNING"
  | "ERROR"
  | "CRITICAL"
  | "ALERT"
  | "EMERGENCY";

export interface ApiClient {
  pair(token: string): Promise<PairResponse>;
  authStatus(): Promise<AuthStatus>;
  profiles(): Promise<ProfileListResponse>;
  saveProfile(input: ProfileInput, id?: string): Promise<Profile>;
  sources(profileId: string): Promise<SourceDiscovery>;
  query(input: QueryRequest, signal?: AbortSignal): Promise<QueryResponse>;
  requestContext(
    input: RequestContextRequest,
    signal?: AbortSignal,
  ): Promise<RequestContextResponse>;
  serviceHealth(
    input: ServiceHealthRequest,
    signal?: AbortSignal,
  ): Promise<ServiceHealthResponse>;
  fleetOverview(
    input: FleetOverviewRequest,
    signal?: AbortSignal,
  ): Promise<FleetOverviewResponse>;
}

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
  }
}
