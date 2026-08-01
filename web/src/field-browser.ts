import type { LogEntry } from "./api/types";

export interface DiscoveredField {
  path: string;
  count: number;
  types: string[];
  sample?: string;
}
const blocked = new Set(["__proto__", "prototype", "constructor"]);

function valueType(value: unknown) {
  if (value === null) return "null";
  if (Array.isArray(value)) return "array";
  return typeof value;
}

export function discoverFields(
  entries: LogEntry[],
  maxNodes = 2000,
): DiscoveredField[] {
  const fields = new Map<
    string,
    { count: number; types: Set<string>; sample?: string }
  >();
  let nodes = 0;
  function visit(
    value: unknown,
    path: string,
    depth: number,
    seen: WeakSet<object>,
  ) {
    if (
      ++nodes > maxNodes ||
      depth > 6 ||
      value === null ||
      typeof value !== "object" ||
      Array.isArray(value)
    )
      return;
    if (seen.has(value)) return;
    seen.add(value);
    for (const [key, child] of Object.entries(
      value as Record<string, unknown>,
    ).slice(0, 100)) {
      if (blocked.has(key) || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(key)) continue;
      const childPath = path ? `${path}.${key}` : key;
      const current = fields.get(childPath) ?? {
        count: 0,
        types: new Set<string>(),
      };
      current.count += 1;
      current.types.add(valueType(child));
      if (!current.sample && typeof child === "string")
        current.sample = child.slice(0, 80);
      fields.set(childPath, current);
      visit(child, childPath, depth + 1, seen);
    }
  }
  for (const entry of entries.slice(0, 500))
    if (entry.structured) visit(entry.structured, "", 0, new WeakSet());
  return [...fields]
    .map(([path, field]) => ({
      path,
      count: field.count,
      types: [...field.types].sort(),
      ...(field.sample ? { sample: field.sample } : {}),
    }))
    .sort((a, b) => b.count - a.count || a.path.localeCompare(b.path));
}

export function valueAtPath(value: unknown, path: string): unknown {
  let current = value;
  for (const key of path.split(".")) {
    if (
      blocked.has(key) ||
      current === null ||
      typeof current !== "object" ||
      Array.isArray(current) ||
      !Object.prototype.hasOwnProperty.call(current, key)
    )
      return undefined;
    current = (current as Record<string, unknown>)[key];
  }
  return current;
}

export function compactValue(value: unknown) {
  if (value === undefined) return "—";
  if (typeof value === "string") return value.slice(0, 80);
  try {
    return JSON.stringify(value).slice(0, 80);
  } catch {
    return "[unavailable]";
  }
}
