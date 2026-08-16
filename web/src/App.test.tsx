import { StrictMode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, expect, test, vi } from "vitest";
import { ApiError, type ApiClient } from "./api/types";

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
  localStorage.clear();
  sessionStorage.clear();
  api.pair.mockReset();
  api.authStatus.mockReset();
  api.profiles.mockReset();
  api.saveProfile.mockReset();
  api.sources.mockReset();
  api.query.mockReset();
  api.requestContext.mockReset();
  api.serviceHealth.mockReset();
  api.fleetOverview.mockReset();
  api.profiles.mockResolvedValue([]);
  api.sources.mockResolvedValue({ sources: [] });
  api.query.mockResolvedValue({
    entries: [],
    expiresAt: "2099-01-01T00:00:00.000Z",
  });
  api.authStatus.mockResolvedValue({
    available: true,
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

test("defaults the primary view and canonical URL to Logs", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  window.history.replaceState(null, "", "/?view=unexpected");

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  expect(await screen.findByRole("textbox", { name: "Query" })).toBeVisible();
  expect(screen.getByRole("button", { name: "Logs" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  expect(await screen.findByText("Connect a project")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "New connection" }));
  expect(screen.getByRole("dialog", { name: "New connection" })).toBeVisible();
  await waitFor(() => expect(window.location.search).toBe(""));
});

test("explains how to configure ADC when credentials are unavailable", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.authStatus.mockResolvedValue({
    available: false,
    message:
      "Application Default Credentials are unavailable. Run `gcloud auth application-default login` or set GOOGLE_APPLICATION_CREDENTIALS to a service-account key file.",
  });

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

  expect(
    await screen.findByText("Application Default Credentials required"),
  ).toBeInTheDocument();
  expect(
    screen.getByRole("button", { name: "Check credentials again" }),
  ).toBeVisible();
  expect(screen.getByRole("button", { name: "New connection" })).toBeVisible();
  fireEvent.click(
    screen.getByRole("button", { name: "Check credentials again" }),
  );
  await waitFor(() => expect(api.authStatus).toHaveBeenCalledTimes(2));
  expect(api.sources).not.toHaveBeenCalled();
});

test("recovers credential, profile, and source data without reloading", async () => {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  api.authStatus
    .mockResolvedValueOnce({ available: false, message: "ADC unavailable" })
    .mockResolvedValue({ available: true, message: "ADC available" });
  api.profiles
    .mockResolvedValueOnce([
      {
        id: "test",
        name: "Test",
        projectId: "synthetic-project",
        provider: "gcp",
        status: "needs-auth",
      },
    ])
    .mockResolvedValue([
      {
        id: "test",
        name: "Test",
        projectId: "synthetic-project",
        provider: "gcp",
        status: "ready",
      },
    ]);

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );
  await screen.findByText("Application Default Credentials required");
  fireEvent.click(
    screen.getByRole("button", { name: "Check credentials again" }),
  );

  expect(await screen.findByText("Define a query")).toBeVisible();
  expect(api.authStatus).toHaveBeenCalledTimes(2);
  expect(api.profiles.mock.calls.length).toBeGreaterThanOrEqual(2);
  await waitFor(() =>
    expect(api.sources.mock.calls.length).toBeGreaterThanOrEqual(2),
  );
});

test("Escape closes the custom range UI without changing the selected range", async () => {
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

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  await screen.findByText("Define a query");
  const custom = await screen.findByRole("button", { name: "Custom" });
  fireEvent.click(custom);
  fireEvent.change(screen.getByLabelText("Custom start"), {
    target: { value: "2026-08-01T10:00" },
  });
  fireEvent.change(screen.getByLabelText("Custom end"), {
    target: { value: "2026-08-01T10:15" },
  });
  fireEvent.keyDown(window, { key: "Escape" });

  expect(screen.queryByLabelText("Custom start")).not.toBeInTheDocument();
  expect(custom).toHaveAttribute("aria-pressed", "true");
  fireEvent.click(screen.getByRole("button", { name: "Run query" }));
  await waitFor(() => expect(api.query).toHaveBeenCalledTimes(1));
  expect(api.query.mock.calls[0]?.[0]).toMatchObject({
    start: new Date("2026-08-01T10:00").toISOString(),
    end: new Date("2026-08-01T10:15").toISOString(),
  });
});

test("manual source Add is idempotent and selected sources can be removed", async () => {
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

  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  await screen.findByText("Define a query");
  fireEvent.click(await screen.findByRole("button", { name: "All sources" }));
  const input = screen.getByLabelText("Manual service name");
  fireEvent.change(input, { target: { value: "manual-api" } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));
  fireEvent.change(input, { target: { value: "manual-api" } });
  fireEvent.click(screen.getByRole("button", { name: "Add" }));

  expect(screen.getByRole("button", { name: "1 source" })).toBeVisible();
  expect(screen.getAllByText("manual-api")).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "Remove manual-api" }));
  expect(
    screen.getAllByRole("button", { name: "All sources" })[0],
  ).toBeVisible();
});

test("explains ADC after a query authentication failure", async () => {
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
      "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry.",
      424,
    ),
  );
  render(
    <QueryClientProvider client={client}>
      <App />
    </QueryClientProvider>,
  );

  const run = await screen.findByRole("button", { name: "Run query" });
  await waitFor(() => expect(run).toBeEnabled());
  fireEvent.click(run);
  expect(
    await screen.findByText("Application Default Credentials required"),
  ).toBeVisible();
  expect(screen.queryByRole("button", { name: /sign in/i })).toBeNull();

  await waitFor(() => expect(api.query).toHaveBeenCalledTimes(1));
  api.query.mockResolvedValue({ entries: [] });
  fireEvent.click(
    screen.getByRole("button", { name: "Check credentials again" }),
  );
  await waitFor(() => expect(api.query).toHaveBeenCalledTimes(2));
  expect(
    screen.queryByText("Application Default Credentials required"),
  ).not.toBeInTheDocument();
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
