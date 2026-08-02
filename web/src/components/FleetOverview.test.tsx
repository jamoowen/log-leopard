import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, expect, test, vi } from "vitest";
import type { ApiClient, Source } from "../api/types";
import type { HealthWindow } from "../preferences";

const fleetOverview = vi.hoisted(() => vi.fn<ApiClient["fleetOverview"]>());

vi.mock("../api/client", () => ({ api: { fleetOverview } }));

import { FleetOverview } from "./FleetOverview";

const sources: Source[] = [
  { id: "worker", label: "worker", kind: "cloud-run" },
  { id: "api", label: "api", kind: "cloud-run" },
];

type OpenService = (service: string, window: HealthWindow) => void;

function renderFleet(
  onOpenService: OpenService = vi.fn<OpenService>(),
  discoveryWarning?: string,
) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <FleetOverview
        profile={{
          id: "staging",
          name: "Staging",
          projectId: "sample-project",
          provider: "gcp",
          status: "ready",
        }}
        sources={sources}
        discoveryPending={false}
        {...(discoveryWarning ? { discoveryWarning } : {})}
        sessionReady
        onOpenService={onOpenService}
      />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  localStorage.clear();
  fleetOverview.mockReset();
  fleetOverview.mockImplementation(async (input) => ({
    start: input.start,
    end: input.end,
    services: [
      {
        service: "api",
        requestCount: 100,
        serverErrorCount: 1,
        requestLatencyP95Ms: 240,
      },
      {
        service: "worker",
        requestCount: 8,
        serverErrorCount: 2,
        requestLatencyP95Ms: null,
      },
    ],
  }));
});

test("summarizes fixed fleet metrics and opens a service", async () => {
  const openService = vi.fn<OpenService>();
  const user = userEvent.setup();
  renderFleet(openService);

  expect(await screen.findByRole("table", { name: /last 1h/ })).toBeVisible();
  expect(screen.getByText("Unavailable")).toBeVisible();
  expect(screen.getByText("25.00%")).toBeVisible();
  expect(fleetOverview).toHaveBeenCalledWith(
    expect.objectContaining({
      profileId: "staging",
      services: ["api", "worker"],
    }),
    expect.anything(),
  );

  await user.click(screen.getByRole("button", { name: /worker/ }));
  expect(openService).toHaveBeenCalledWith("worker", "1h");
});

test("changes the bounded metric window", async () => {
  const user = userEvent.setup();
  renderFleet();
  await screen.findByRole("table");

  await user.click(screen.getByRole("button", { name: "24h" }));
  await waitFor(() => expect(fleetOverview).toHaveBeenCalledTimes(2));
  const request = fleetOverview.mock.calls.at(-1)![0];
  expect(
    new Date(request.end).getTime() - new Date(request.start).getTime(),
  ).toBe(24 * 60 * 60_000);
});

test("refreshes fleet metrics explicitly", async () => {
  const user = userEvent.setup();
  renderFleet();
  await screen.findByRole("table");

  await user.click(screen.getByRole("button", { name: "Refresh metrics" }));

  await waitFor(() => expect(fleetOverview).toHaveBeenCalledTimes(2));
});

test("filters services locally and clears the filter", async () => {
  const user = userEvent.setup();
  renderFleet();
  await screen.findByRole("table");
  const filter = screen.getByRole("searchbox", {
    name: "Filter fleet services",
  });

  await user.type(filter, "work");
  expect(screen.getByText("1 OF 2")).toBeVisible();
  expect(screen.getByRole("button", { name: /worker/ })).toBeVisible();
  expect(screen.queryByRole("button", { name: /api/ })).toBeNull();
  expect(fleetOverview).toHaveBeenCalledTimes(1);

  await user.clear(filter);
  await user.type(filter, "missing");
  expect(screen.getByText("No services match “missing”.")).toBeVisible();

  await user.click(
    screen.getByRole("button", { name: "Clear service filter" }),
  );
  expect(screen.getByText("2 OF 2")).toBeVisible();
  expect(screen.getByRole("button", { name: /api/ })).toBeVisible();
});

test("sorts services locally and keeps missing latency last", async () => {
  const user = userEvent.setup();
  renderFleet();
  await screen.findByRole("table");
  const sort = screen.getByRole("combobox", { name: "Sort fleet services" });

  expect(screen.getAllByRole("row")[1]).toHaveTextContent("worker");
  await user.selectOptions(sort, "requests");
  expect(screen.getAllByRole("row")[1]).toHaveTextContent("api");
  await user.selectOptions(sort, "latency");
  expect(screen.getAllByRole("row")[1]).toHaveTextContent("api");
  expect(screen.getAllByRole("row")[2]).toHaveTextContent("worker");
  expect(fleetOverview).toHaveBeenCalledTimes(1);
});

test("shows a bounded discovery warning without hiding available services", async () => {
  renderFleet(
    undefined,
    "Cloud Run discovery is limited to 100 unique service names; additional services were omitted.",
  );

  expect(await screen.findByRole("table")).toBeVisible();
  expect(screen.getByText(/limited to 100 unique service names/)).toBeVisible();
});
