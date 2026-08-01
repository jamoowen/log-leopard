import { ApiError } from "./types";
import { requestSourceDiscovery, shouldUseMockApi } from "./client";

describe("mock API selection", () => {
  it("allows explicit mocks only in development builds", () => {
    expect(shouldUseMockApi("?mock=1", undefined, true)).toBe(true);
    expect(shouldUseMockApi("", "true", true)).toBe(true);
    expect(shouldUseMockApi("?mock=1", "true", false)).toBe(false);
  });
});

describe("source discovery client", () => {
  afterEach(() => vi.restoreAllMocks());

  it("returns a successful empty discovery with its backend warning", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify([]), {
        status: 200,
        headers: {
          "Content-Type": "application/json",
          "X-LogLeopard-Warning": "No concrete services found",
        },
      }),
    );

    await expect(requestSourceDiscovery("profile/id")).resolves.toEqual({
      sources: [],
      warning: "No concrete services found",
    });
    expect(fetch).toHaveBeenCalledWith(
      "/api/v1/sources?profileId=profile%2Fid",
      expect.objectContaining({ credentials: "include" }),
    );
  });

  it("rejects failed discovery instead of returning a successful empty result", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response(JSON.stringify({ detail: "Discovery unavailable" }), {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(requestSourceDiscovery("profile")).rejects.toSatisfy(
      (error: unknown) =>
        error instanceof ApiError &&
        error.message === "Discovery unavailable" &&
        error.status === 503,
    );
  });
});
