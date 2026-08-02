import { expect, test } from "@playwright/test";

async function openLogs(page: import("@playwright/test").Page) {
  await page.getByRole("button", { name: "Logs", exact: true }).click();
}

test("fleet overview opens a selected service", async ({ page }) => {
  await page.goto("/?mock=1");
  await expect(
    page.getByRole("table", { name: /Cloud Run fleet metrics/ }),
  ).toBeVisible();
  await expect(page.getByText("Unavailable", { exact: true })).toBeVisible();
  await page
    .getByRole("combobox", { name: "Sort fleet services" })
    .selectOption("latency");
  await expect(page.getByRole("row").nth(1)).toContainText("payments-api");
  const filter = page.getByRole("searchbox", {
    name: "Filter fleet services",
  });
  await filter.fill("edge");
  await expect(page.getByText("1 OF 3", { exact: true })).toBeVisible();
  await expect(page.getByRole("button", { name: /payments-api/ })).toBeHidden();
  await page.getByRole("button", { name: /edge-router/ }).click();
  await expect(
    page.getByRole("button", { name: "Service health" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByLabel("Health service")).toHaveValue("edge-router");
  await expect(page.getByText("REQUEST HEALTH", { exact: true })).toBeVisible();
});

test("service health drills into the exact log interval", async ({
  page,
}, testInfo) => {
  await page.goto("/?mock=1");
  await page.getByRole("button", { name: "Service health" }).click();
  await page.getByLabel("Health service").selectOption("payments-api");
  await expect(page.getByText("REQUEST HEALTH", { exact: true })).toBeVisible();
  await page.reload();
  await expect(page.getByLabel("Health service")).toHaveValue("payments-api");
  await expect(page.getByText("REQUEST HEALTH", { exact: true })).toBeVisible();
  const chart = page.locator("button.service-health-chart").first();
  await expect(chart.locator(".service-health-scale.request")).toContainText(
    "Requests",
  );
  await expect(chart.locator(".service-health-scale.errors")).toContainText(
    "5xx",
  );
  const latencyChart = page
    .getByRole("slider", {
      name: "P95 latency metric interval",
    })
    .first();
  await expect(latencyChart).toBeVisible();
  await latencyChart.hover({ position: { x: 120, y: 80 } });
  await expect(page.getByRole("tooltip")).toContainText("ms");
  await chart.focus();
  await chart.press("Home");
  const firstBucket = await chart.getAttribute("aria-label");
  await chart.press("ArrowRight");
  await expect(chart).not.toHaveAttribute("aria-label", firstBucket!);
  const keyboardClock = (await chart.getAttribute("aria-label"))?.match(
    /\d{2}:\d{2}/,
  )?.[0];
  await chart.press("Enter");
  await expect(page.locator(".service-health-timeline h1")).toContainText(
    keyboardClock!,
  );
  await page.getByRole("button", { name: "Workbench" }).click();
  await expect(page.getByText("REQUEST HEALTH", { exact: true })).toBeVisible();
  await chart.hover({ position: { x: 120, y: 100 } });
  const tooltip = page.getByRole("tooltip");
  await expect(tooltip).toBeVisible();
  await expect(tooltip).toContainText("Requests");
  await expect(tooltip).toContainText("5xx");
  const hoveredClock = (await tooltip.locator("strong").textContent())?.slice(
    -5,
  );
  if (testInfo.project.name === "mobile") {
    await chart.tap({ position: { x: 120, y: 100 } });
  } else {
    await chart.click({ position: { x: 120, y: 100 } });
  }
  await expect(page.getByText("SELECTED METRIC INTERVAL")).toBeVisible();
  await expect(page.locator(".service-health-timeline h1")).toContainText(
    hoveredClock!,
  );
  await page.getByRole("button", { name: /Open interval in logs/ }).click();
  await expect(
    page.getByRole("button", { name: "Logs", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page.getByRole("textbox", { name: "Query" })).toHaveValue(
    "httpRequest.status >= 500 AND httpRequest.status < 600",
  );
  await expect(page.getByRole("button", { name: "Custom" })).toHaveAttribute(
    "aria-pressed",
    "true",
  );
  await expect(page.getByRole("button", { name: /1 source/ })).toBeVisible();
  const start = await page.getByLabel("Custom start").inputValue();
  const end = await page.getByLabel("Custom end").inputValue();
  expect(new Date(end).getTime() - new Date(start).getTime()).toBe(60_000);
  await expect(page.getByText(/entries loaded/)).toBeVisible();
});

test("primary view survives reload and invalid values fall back safely", async ({
  page,
}) => {
  await page.goto("/?mock=1&view=logs");
  await expect(
    page.getByRole("button", { name: "Logs", exact: true }),
  ).toHaveAttribute("aria-pressed", "true");
  await page.reload();
  await expect(page.getByRole("textbox", { name: "Query" })).toBeVisible();

  await page.goto("/?mock=1&view=unexpected");
  await expect(
    page.getByRole("button", { name: "Fleet overview" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page).not.toHaveURL(/view=/);
});

test("command palette navigates primary views", async ({ page }) => {
  await page.goto("/?mock=1");

  await page.keyboard.press("Control+k");
  await page.getByLabel("Search commands").fill("service health");
  await page.getByLabel("Search commands").press("Enter");
  await expect(
    page.getByRole("button", { name: "Service health" }),
  ).toHaveAttribute("aria-pressed", "true");
  await expect(page).toHaveURL(/view=health/);

  await page.keyboard.press("Control+k");
  await page.getByLabel("Search commands").fill("open");
  await page.getByLabel("Search commands").press("End");
  await page.getByLabel("Search commands").press("Enter");
  await expect(page.getByRole("textbox", { name: "Query" })).toBeVisible();
  await expect(page).toHaveURL(/view=logs/);

  await page.keyboard.press("Control+k");
  await page.getByLabel("Search commands").fill("fleet overview");
  await page.getByRole("option", { name: /Open Fleet overview/ }).click();
  await page.keyboard.press("Control+k");
  await page.getByLabel("Search commands").fill("run query");
  await expect(page.getByText("No matching command.")).toBeVisible();
  await page.keyboard.press("Escape");

  const trigger = page.getByRole("button", { name: "Open command palette" });
  await trigger.click();
  await expect(page.getByLabel("Search commands")).toBeFocused();
  await page.keyboard.press("Escape");
  await expect(trigger).toBeFocused();
});

test("desktop query and inspector workflow", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);
  await expect(page.getByText("Log Leopard", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: /All sources/ }).click();
  await expect(page.getByText(/discovery fallback/)).toBeHidden();
  await page.getByText("payments-api", { exact: true }).click();
  await expect(page.getByRole("button", { name: /1 source/ })).toBeVisible();
  await page.getByRole("button", { name: "All sources", exact: true }).click();
  await expect(
    page.getByRole("button", { name: /All sources/ }).first(),
  ).toBeVisible();
  await page.getByText("payments-api", { exact: true }).click();
  await page.getByRole("button", { name: "ERROR", exact: true }).click();
  await page.getByRole("textbox", { name: "Query" }).fill("Deadline");
  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.getByText(/entries loaded/)).toBeVisible();
  await page.locator(".log-row").first().click();
  await expect(page.getByText("ENTRY", { exact: true })).toBeVisible();
  await page.getByRole("tab", { name: "Raw" }).click();
  await expect(page.getByTestId("json-text")).toContainText("insertId");
});

test("load more appends the next result page", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);
  await page.getByRole("button", { name: "1 hour" }).click();
  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.getByText("80 entries loaded")).toBeVisible();

  await page.getByRole("button", { name: "Load more" }).click();
  await expect(page.getByText("140 entries loaded")).toBeVisible();
  await expect(page.getByRole("button", { name: "Load more" })).toBeHidden();
});

test("identical custom requests refetch and polling does not lock execution", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);
  await page.getByRole("button", { name: "Custom" }).click();
  await page.getByLabel("Custom start").fill("2026-07-01T10:00");
  await page.getByLabel("Custom end").fill("2026-07-01T10:15");

  const run = page.getByRole("button", { name: /Run query|Running/ });
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(page.getByText(/entries loaded/)).toBeVisible();
  await page.keyboard.press("ControlOrMeta+Enter");
  await expect(run).toContainText("Running");
  await expect(page.getByText(/entries loaded/)).toBeVisible();

  await page.getByRole("combobox", { name: "Polling" }).click();
  await page.getByRole("option", { name: "Every 5 seconds" }).click();
  await expect
    .poll(() => run.textContent(), { timeout: 6_500, intervals: [50] })
    .toContain("Running");
  await expect(page.getByText(/entries loaded/)).toBeVisible();
});

test("changing connection clears profile-bound results and inspector state", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);
  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.getByText(/entries loaded/)).toBeVisible();
  await page.locator(".log-row").first().click();
  await page.getByRole("tab", { name: "Context" }).click();
  await expect(page.locator(".context-list button").first()).toBeVisible();

  await page.getByRole("combobox", { name: "Connection" }).click();
  await page.getByRole("option", { name: "Staging" }).click();
  await expect(page.getByText("Define a query")).toBeVisible();
  await expect(page.getByText("ENTRY", { exact: true })).toBeHidden();
  await expect(page.locator(".log-row")).toHaveCount(0);
  await page.getByRole("button", { name: "All sources" }).click();
  await expect(
    page.getByText(
      "No concrete Cloud Run services were discovered. Queries will use all Cloud Run logs.",
    ),
  ).toBeVisible();
  await page.keyboard.press("Escape");

  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.getByText(/entries loaded/)).toBeVisible();
  await page.locator(".log-row").first().click();
  await page.getByRole("tab", { name: "Context" }).click();
  await expect(page.locator(".context-list button").first()).toBeVisible();
});

test("hostile log text remains inert outside JSON views", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);
  await page.getByRole("textbox", { name: "Query" }).fill("User payload");
  await page.getByRole("button", { name: /Run query/ }).click();
  const row = page.locator(".log-row").first();
  await expect(row).toContainText("<img src=x onerror=alert(1)>");
  await expect(row.locator("img")).toHaveCount(0);
  await row.click();
  await expect(page.locator(".message-detail")).toContainText(
    "<img src=x onerror=alert(1)>",
  );
  await expect(page.locator(".message-detail img")).toHaveCount(0);
});

test("pairing resolves before protected data loads and clears the fragment", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1#pair=synthetic-pairing-token");
  await expect(page.getByText("Pairing session…")).toBeVisible();
  await openLogs(page);
  await expect(page.getByText("Define a query")).toBeVisible();
  await expect(page).not.toHaveURL(/#pair=/);
  await expect(
    page.getByRole("combobox", { name: "Connection" }),
  ).toContainText("Production read-only");
});

test("mobile core workflow remains usable", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "mobile");
  await page.goto("/?mock=1");
  await page.getByRole("button", { name: "Service health" }).click();
  await expect(page.getByText("Choose a Cloud Run service")).toBeVisible();
  await openLogs(page);
  await page.getByRole("textbox", { name: "Query" }).fill("Request");
  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.locator(".log-row").first()).toBeVisible();
  await page.locator(".log-row").first().click();
  await expect(page.getByRole("tab", { name: "Overview" })).toBeVisible();
  await page.getByLabel("Close inspector").click();
  await expect(page.getByRole("textbox", { name: "Query" })).toBeVisible();
});

test("structured builder, field tools, context, recipes, and commands work together", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await openLogs(page);

  await page.getByRole("button", { name: "Structured builder" }).click();
  await expect(
    page.getByRole("button", { name: "Structured builder" }),
  ).toHaveAttribute("aria-pressed", "true");
  await page
    .getByRole("button", { name: /Add the first structured condition/ })
    .click();
  await page.getByLabel("Condition 1 path").fill("latencyMs");
  await page.getByLabel("Condition 1 operator").selectOption("gt");
  await page.getByLabel("Condition 1 value").fill("20");
  await page.getByRole("button", { name: /Run query/ }).click();
  await expect(page.getByText(/entries loaded/)).toBeVisible();

  await page.locator(".log-row").first().click();
  await page.getByRole("tab", { name: "Fields" }).click();
  await expect(page.getByText("Loaded fields")).toBeVisible();
  await page.getByLabel("Pin requestId").click();
  await page.getByLabel("Close inspector").click();
  await expect(page.locator(".pinned-values").first()).toContainText(
    "requestId",
  );

  await page.locator(".log-row").first().click();
  await page.getByRole("tab", { name: "Context" }).click();
  await expect(
    page.getByText(/active source and query filters are not applied/),
  ).toBeVisible();
  await expect(page.locator(".context-list button").first()).toBeVisible();

  await page.getByLabel("Close inspector").click();
  await page.getByLabel("Saved queries").click();
  await page.getByLabel("Enable local recipe storage").check();
  await page.getByLabel("Saved query name").fill("Slow requests");
  await page.getByRole("button", { name: "Save current" }).click();
  await expect(page.getByText("Slow requests", { exact: true })).toBeVisible();

  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.getByLabel("Search commands").fill("poll every 10");
  await page.getByRole("option", { name: /Poll every 10 seconds/ }).click();
  await expect(page.getByRole("combobox", { name: "Polling" })).toContainText(
    "Polling every 10s",
  );
});
