import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import { ApiError, type ApiClient, type Source } from "../api/types";

const serviceHealth = vi.hoisted(() => vi.fn<ApiClient["serviceHealth"]>());

vi.mock("../api/client", () => ({ api: { serviceHealth } }));

import { ServiceHealth } from "./ServiceHealth";

const sources: Source[] = [
  { id: "worker", label: "worker", kind: "cloud-run" },
  { id: "api", label: "api", kind: "cloud-run" },
];

function renderHealth({
  availableSources = sources,
  discoveryError,
  discoveryWarning,
  discoveryWarningCode,
  profileStatus = "ready",
}: {
  availableSources?: Source[];
  discoveryError?: string;
  discoveryWarning?: string;
  discoveryWarningCode?: "authentication";
  profileStatus?: "ready" | "needs-auth";
} = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const rendered = render(
    <QueryClientProvider client={client}>
      <ServiceHealth
        profile={{
          id: "staging",
          name: "Staging",
          projectId: "sample-project",
          provider: "gcp",
          status: profileStatus,
        }}
        sources={availableSources}
        discoveryPending={false}
        {...(discoveryError ? { discoveryError } : {})}
        {...(discoveryWarning ? { discoveryWarning } : {})}
        {...(discoveryWarningCode ? { discoveryWarningCode } : {})}
        sessionReady
        onOpenLogs={() => undefined}
      />
    </QueryClientProvider>,
  );
  return { ...rendered, client };
}

beforeEach(() => {
  localStorage.clear();
  serviceHealth.mockReset();
});

test("waits for an explicit target and can broaden an empty window", async () => {
  serviceHealth.mockImplementation(async (input) => ({
    service: input.service,
    start: input.start,
    end: input.end,
    alignmentSeconds: 60,
    series: [
      { name: "request_count", unit: "1", points: [] },
      { name: "server_error_count", unit: "1", points: [] },
    ],
  }));
  const user = userEvent.setup();
  renderHealth();

  expect(screen.getByText("Choose a Cloud Run service")).toBeVisible();
  expect(serviceHealth).not.toHaveBeenCalled();

  await user.selectOptions(screen.getByLabelText("Health service"), "api");
  await screen.findByText("No traffic observed");
  expect(serviceHealth).toHaveBeenCalledTimes(1);

  await user.click(screen.getByRole("button", { name: "Try 24h" }));
  await waitFor(() => expect(serviceHealth).toHaveBeenCalledTimes(2));
  const request = serviceHealth.mock.calls.at(-1)![0];
  expect(
    new Date(request.end).getTime() - new Date(request.start).getTime(),
  ).toBe(24 * 60 * 60_000);
  expect(
    JSON.parse(localStorage.getItem("logleopard.health-preferences.v1")!),
  ).toMatchObject({ staging: { target: "api", window: "24h" } });
});

test("recalculates the rolling window when metrics refetch", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  try {
    vi.setSystemTime(new Date("2026-08-02T10:15:30Z"));
    localStorage.setItem(
      "logleopard.health-preferences.v1",
      JSON.stringify({ staging: { target: "api", window: "1h" } }),
    );
    serviceHealth.mockImplementation(async (input) => ({
      service: input.service,
      start: input.start,
      end: input.end,
      alignmentSeconds: 60,
      series: [
        { name: "request_count", unit: "1", points: [] },
        { name: "server_error_count", unit: "1", points: [] },
      ],
    }));
    const { client } = renderHealth();
    await waitFor(() => expect(serviceHealth).toHaveBeenCalledTimes(1));
    expect(serviceHealth.mock.calls[0]![0].end).toBe(
      "2026-08-02T10:15:00.000Z",
    );

    vi.setSystemTime(new Date("2026-08-02T10:27:30Z"));
    await client.refetchQueries({ queryKey: ["service-health"] });

    expect(serviceHealth).toHaveBeenCalledTimes(2);
    expect(serviceHealth.mock.calls[1]![0].end).toBe(
      "2026-08-02T10:27:00.000Z",
    );
  } finally {
    vi.useRealTimers();
  }
});

test("refreshes the selected service explicitly", async () => {
  localStorage.setItem(
    "logleopard.health-preferences.v1",
    JSON.stringify({ staging: { target: "api", window: "1h" } }),
  );
  serviceHealth.mockImplementation(async (input) => ({
    service: input.service,
    start: input.start,
    end: input.end,
    alignmentSeconds: 60,
    series: [
      { name: "request_count", unit: "1", points: [] },
      { name: "server_error_count", unit: "1", points: [] },
    ],
  }));
  const user = userEvent.setup();
  renderHealth();
  await screen.findByText("No traffic observed");

  await user.click(screen.getByRole("button", { name: "Refresh metrics" }));

  await waitFor(() => expect(serviceHealth).toHaveBeenCalledTimes(2));
});

test("distinguishes missing metrics permission from no traffic", async () => {
  serviceHealth.mockRejectedValue(
    new ApiError("Grant roles/monitoring.viewer, then retry.", 403),
  );
  const user = userEvent.setup();
  renderHealth();

  await user.selectOptions(screen.getByLabelText("Health service"), "api");

  expect(await screen.findByText("Metrics access required")).toBeVisible();
  expect(screen.queryByText("No traffic observed")).not.toBeInTheDocument();
});

test("distinguishes failed discovery from a project without targets", () => {
  renderHealth({
    availableSources: [],
    discoveryError: "Cloud Run discovery request failed.",
  });

  expect(screen.getByText("Service discovery unavailable")).toBeVisible();
  expect(screen.queryByText("No Cloud Run services discovered")).toBeNull();
});

test("explains when Google Cloud authentication is required", () => {
  renderHealth({ availableSources: [], profileStatus: "needs-auth" });

  expect(
    screen.getByText("Google Cloud authentication required"),
  ).toBeVisible();
  expect(
    screen.getByText(/gcloud auth application-default login/),
  ).toBeVisible();
  expect(screen.queryByText("No Cloud Run services discovered")).toBeNull();
});

test("explains an authentication failure reported by discovery", () => {
  renderHealth({
    availableSources: [],
    discoveryWarning:
      "Google Cloud authentication is unavailable or expired. Refresh ADC with gcloud auth application-default login, or verify GOOGLE_APPLICATION_CREDENTIALS, then retry.",
    discoveryWarningCode: "authentication",
  });

  expect(
    screen.getByText("Google Cloud authentication required"),
  ).toBeVisible();
});

test("does not present a missing latency observation as zero", async () => {
  serviceHealth.mockImplementation(async (input) => {
    const timestamp = new Date(
      new Date(input.start).getTime() + 60_000,
    ).toISOString();
    return {
      service: input.service,
      start: input.start,
      end: input.end,
      alignmentSeconds: 60,
      series: [
        { name: "request_count", unit: "1", points: [{ timestamp, value: 4 }] },
        {
          name: "server_error_count",
          unit: "1",
          points: [{ timestamp, value: 0 }],
        },
      ],
    };
  });
  const user = userEvent.setup();
  renderHealth();

  await user.selectOptions(screen.getByLabelText("Health service"), "api");

  expect(await screen.findByText("Unavailable")).toBeVisible();
  expect(
    screen.queryByRole("slider", { name: "P95 latency metric interval" }),
  ).toBeNull();
  expect(screen.queryByText("0ms")).toBeNull();
});

test("joins sparse samples with an explicit dashed gap", async () => {
  serviceHealth.mockImplementation(async (input) => {
    const start = new Date(input.start).getTime();
    const timestamps = [1, 2, 8].map((minute) =>
      new Date(start + minute * 60_000).toISOString(),
    );
    return {
      service: input.service,
      start: input.start,
      end: input.end,
      alignmentSeconds: 60,
      series: [
        {
          name: "request_count",
          unit: "1",
          points: timestamps.map((timestamp, index) => ({
            timestamp,
            value: 10 + index,
          })),
        },
        {
          name: "server_error_count",
          unit: "1",
          points: timestamps.map((timestamp, index) => ({
            timestamp,
            value: index === 1 ? 1 : 0,
          })),
        },
        {
          name: "request_latency_p95",
          unit: "ms",
          points: timestamps.map((timestamp, index) => ({
            timestamp,
            value: 100 + index * 20,
          })),
        },
      ],
    };
  });
  const user = userEvent.setup();
  const { container } = renderHealth();

  await user.selectOptions(screen.getByLabelText("Health service"), "api");
  await screen.findByText("REQUEST HEALTH");

  expect(
    container.querySelector(".service-health-request-line.request"),
  ).toHaveAttribute("d", "M6.7 46.7 L20.0 27.3");
  expect(
    container.querySelector(".service-health-gap-line.request"),
  ).toHaveAttribute("d", "M20.0 27.3 L100.0 8.0");
  expect(
    container.querySelector(".service-health-secondary-line"),
  ).toHaveAttribute("d", "M6.7 240.0 L20.0 8.0");
  expect(
    container.querySelector(".service-health-scale.request"),
  ).toHaveTextContent("12Requests0");
  expect(
    container.querySelector(".service-health-scale.errors"),
  ).toHaveTextContent("15xx0");
  expect(container.querySelector(".service-health-chart rect")).toBeNull();
  expect(container.querySelector(".service-health-chart circle")).toBeNull();
});
