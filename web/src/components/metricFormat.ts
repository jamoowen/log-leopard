export function formatMetricValue(value: number, unit: "count" | "ms") {
  if (unit === "count") return formatCount(value);
  return value >= 1_000
    ? `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 2)}s`
    : `${value.toFixed(value >= 100 ? 0 : 1)}ms`;
}

export function formatCount(value: number) {
  return value >= 1_000
    ? `${(value / 1_000).toFixed(value >= 10_000 ? 0 : 1)}k`
    : value.toFixed(0);
}
