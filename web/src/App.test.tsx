import { StrictMode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, test, vi } from "vitest";
import type { ApiClient } from "./api/types";

const api = vi.hoisted(() => ({
  pair: vi.fn<ApiClient["pair"]>(),
  authStatus: vi.fn<ApiClient["authStatus"]>(),
  profiles: vi.fn<ApiClient["profiles"]>(),
  saveProfile: vi.fn<ApiClient["saveProfile"]>(),
  sources: vi.fn<ApiClient["sources"]>(),
  query: vi.fn<ApiClient["query"]>(),
  requestContext: vi.fn<ApiClient["requestContext"]>(),
  serviceHealth: vi.fn<ApiClient["serviceHealth"]>(),
  fleetOverview: vi.fn<ApiClient["fleetOverview"]>(),
}));

vi.mock("./api/client", () => ({
  api,
}));

import App from "./App";

beforeEach(() => {
  api.pair.mockReset();
  api.profiles.mockResolvedValue([]);
  api.pair
    .mockResolvedValueOnce({ paired: true })
    .mockRejectedValueOnce(new Error("pairing token is invalid or expired"));
  window.history.replaceState(null, "", "/#pair=single-use-token");
});

test("exchanges a single-use pairing token once in development StrictMode", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });

  render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <App />
      </QueryClientProvider>
    </StrictMode>,
  );

  await waitFor(() => expect(api.pair).toHaveBeenCalledTimes(1));
  expect(
    screen.queryByText("Local browser pairing required"),
  ).not.toBeInTheDocument();
});
