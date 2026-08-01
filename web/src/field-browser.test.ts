import { discoverFields, valueAtPath } from "./field-browser";
import type { LogEntry } from "./api/types";

const entry = (structured: Record<string, unknown>): LogEntry => ({
  id: "1",
  labels: {},
  message: "",
  messageSource: "",
  raw: {},
  severity: "INFO",
  severityOriginal: "INFO",
  source: "x",
  sourceOriginal: "x",
  structured,
  timestamp: new Date().toISOString(),
});

it("discovers safe structured paths with counts, types, and samples", () => {
  const fields = discoverFields([
    entry({ request: { id: "abc", latency: 4 } }),
    entry({ request: { id: "def" } }),
  ]);
  expect(fields.find((field) => field.path === "request.id")).toMatchObject({
    count: 2,
    types: ["string"],
    sample: "abc",
  });
  expect(
    fields.find((field) => field.path === "request.latency"),
  ).toMatchObject({ count: 1, types: ["number"] });
});

it("reads only own safe path properties", () => {
  expect(valueAtPath({ request: { id: "abc" } }, "request.id")).toBe("abc");
  expect(valueAtPath({}, "__proto__.polluted")).toBeUndefined();
});
