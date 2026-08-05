import { StrictMode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, test, vi } from "vitest";
import { ApiError, type ApiClient } from "./api/types";

const api = vi.hoisted(() => ({
  pair: vi.fn<ApiClient["pair"]>(),
  authStatus: vi.fn<ApiClient["authStatus"]>(),
  startGoogleAuth: vi.fn<ApiClient["startGoogleAuth"]>(),
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
  localStorage.clear();
  sessionStorage.clear();
  api.pair.mockReset();
  api.authStatus.mockReset();
  api.startGoogleAuth.mockReset();
  api.profiles.mockResolvedValue([]);
  api.sources.mockResolvedValue({ sources: [] });
  api.authStatus.mockResolvedValue({
    available: true,
    state: "available",
    message: "Application Default Credentials are available.",
  });
  api.pair
    .mockResolvedValueOnce({ paired: true })
    .mockRejectedValueOnce(new Error("pairing token is invalid or expired"));
  window.history.replaceState(
    null,
    "",
    "/?view=logs#pair=single-use-pairing-token-with-enough-length",
  );
});

test("shows the Google sign-in action from the default fleet view when credentials are unavailable", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.authStatus.mockResolvedValue({
    available: false,
    state: "needs-auth",
    message:
      "Sign in with Google to make Application Default Credentials available.",
  });
  api.startGoogleAuth.mockResolvedValue({
    authorizationUrl: "https://example.invalid/google-auth",
    state: "pending",
    message: "Continue signing in with Google.",
  });

  api.profiles.mockResolvedValue([
    {
      id: "test",
      name: "Test",
      projectId: "synthetic-project",
      provider: "gcp",
      status: "needs-auth",
    },
  ]);
  window.history.replaceState(
    null,
    "",
    "/#pair=single-use-pairing-token-with-enough-length",
  );

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  const button = await screen.findByRole("button", {
    name: "Sign in with Google",
  });
  expect(
    screen.getByRole("button", { name: "Fleet overview" }),
  ).toHaveAttribute("aria-pressed", "true");
  fireEvent.click(button);
  await waitFor(() => expect(api.startGoogleAuth).toHaveBeenCalledTimes(1));
});

test("refreshes after a Google sign-in success return without finishing in the browser", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  window.history.replaceState(
    null,
    "",
    "/?auth=success#pair=single-use-pairing-token-with-enough-length",
  );

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  await waitFor(() => expect(api.authStatus).toHaveBeenCalled());
  expect(window.location.search).toBe("");
});

test("explains a Google sign-in callback failure", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  window.history.replaceState(
    null,
    "",
    "/?auth=failed#pair=single-use-pairing-token-with-enough-length",
  );

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  expect(
    await screen.findByText(
      "Google sign-in was not completed. Start sign-in again.",
    ),
  ).toBeInTheDocument();
  expect(window.location.search).toBe("");
});

test("shows Google sign-in after a query authentication failure", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.profiles.mockResolvedValue([
    {
      id: "test",
      name: "Test",
      projectId: "synthetic-project",
      provider: "gcp",
      status: "ready",
    },
  ]);
  api.query.mockRejectedValue(
    new ApiError(
      "Google Cloud authentication is unavailable or expired. Sign in with Google or verify Application Default Credentials, then retry.",
      424,
    ),
  );
  api.startGoogleAuth.mockResolvedValue({
    authorizationUrl: "https://example.invalid/google-auth",
    state: "pending",
    message: "Continue signing in with Google.",
  });

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  fireEvent.click(await screen.findByRole("button", { name: "Run query" }));
  const button = await screen.findByRole("button", {
    name: "Sign in with Google",
  });
  expect(
    screen.getByText("Google Cloud authentication is unavailable or expired.", {
      exact: false,
    }),
  ).toBeVisible();
  fireEvent.click(button);
  await waitFor(() => expect(api.startGoogleAuth).toHaveBeenCalledTimes(1));
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
