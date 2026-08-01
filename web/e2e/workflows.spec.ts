import { expect, test } from "@playwright/test";

test("desktop query and inspector workflow", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
  await expect(page.getByText("LOGLEOPARD", { exact: true })).toBeVisible();
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

test("identical custom requests refetch and polling does not lock execution", async ({
  page,
}, testInfo) => {
  test.skip(testInfo.project.name !== "desktop");
  await page.goto("/?mock=1");
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
  await expect(page.getByText("Define a query")).toBeVisible();
  await expect(page).not.toHaveURL(/#pair=/);
  await expect(
    page.getByRole("combobox", { name: "Connection" }),
  ).toContainText("Production read-only");
});

test("mobile core workflow remains usable", async ({ page }, testInfo) => {
  test.skip(testInfo.project.name !== "mobile");
  await page.goto("/?mock=1");
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
  await page.getByLabel("Saved query name").fill("Slow requests");
  await page.getByRole("button", { name: "Save current" }).click();
  await expect(page.getByText("Slow requests", { exact: true })).toBeVisible();

  await page.keyboard.press("ControlOrMeta+KeyK");
  await page.getByLabel("Search commands").fill("poll every 10");
  await page.getByRole("button", { name: /Poll every 10 seconds/ }).click();
  await expect(page.getByRole("combobox", { name: "Polling" })).toContainText(
    "Polling every 10s",
  );
});
