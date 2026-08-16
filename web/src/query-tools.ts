import type { FieldPredicate, QueryMode } from "./api/types";

export type PredicateOperator = FieldPredicate["operator"];
export interface PredicateDraft {
  id: string;
  path: string;
  operator: PredicateOperator;
  value: string;
}

const pathPattern = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/;
const blockedPathParts = new Set(["__proto__", "prototype", "constructor"]);
const maxPathLength = 256;
const maxPathSegments = 20;
const maxStringValueLength = 2048;

function byteLength(value: string) {
  return new TextEncoder().encode(value).length;
}

export function normalizePredicatePath(value: string) {
  const path = value.trim();
  return path.startsWith("jsonPayload.")
    ? path.slice("jsonPayload.".length)
    : path;
}

export function validatePredicate(draft: PredicateDraft): string | null {
  const path = normalizePredicatePath(draft.path);
  if (byteLength(path) > maxPathLength)
    return "Path cannot exceed 256 characters.";
  if (path.split(".").length > maxPathSegments)
    return "Path cannot contain more than 20 segments.";
  if (
    !pathPattern.test(path) ||
    path.split(".").some((part) => blockedPathParts.has(part))
  )
    return "Use a safe path like level or jsonPayload.level.";
  if (draft.operator === "exists")
    return draft.value === "true" || draft.value === "false"
      ? null
      : "Choose true or false.";
  if (draft.operator === "gt" || draft.operator === "lt")
    return draft.value.trim() !== "" && Number.isFinite(Number(draft.value))
      ? null
      : "Enter a number.";
  if (draft.operator === "contains")
    return draft.value.length === 0
      ? "Enter text to match."
      : byteLength(draft.value) > maxStringValueLength
        ? "Text cannot exceed 2048 characters."
        : null;
  const parsed = parseEquals(draft.value);
  if (!parsed.valid) return "Enter text, a number, true, or false.";
  return typeof parsed.value === "string" &&
    byteLength(parsed.value) > maxStringValueLength
    ? "Text cannot exceed 2048 characters."
    : null;
}

function parseEquals(value: string): {
  valid: boolean;
  value: string | number | boolean;
} {
  const clean = value.trim();
  if (!clean) return { valid: false, value: "" };
  if (clean.startsWith('"') || clean.endsWith('"')) {
    try {
      const parsed: unknown = JSON.parse(clean);
      return typeof parsed === "string"
        ? { valid: true, value: parsed }
        : { valid: false, value: "" };
    } catch {
      return { valid: false, value: "" };
    }
  }
  if (clean === "true" || clean === "false")
    return { valid: true, value: clean === "true" };
  if (/^-?(?:\d+\.?\d*|\.\d+)$/.test(clean) && Number.isFinite(Number(clean)))
    return { valid: true, value: Number(clean) };
  return { valid: true, value };
}

export function toPredicate(draft: PredicateDraft): FieldPredicate | null {
  if (validatePredicate(draft)) return null;
  if (draft.operator === "exists")
    return {
      path: normalizePredicatePath(draft.path),
      operator: draft.operator,
      value: draft.value === "true",
    };
  if (draft.operator === "gt" || draft.operator === "lt")
    return {
      path: normalizePredicatePath(draft.path),
      operator: draft.operator,
      value: Number(draft.value),
    };
  if (draft.operator === "equals")
    return {
      path: normalizePredicatePath(draft.path),
      operator: draft.operator,
      value: parseEquals(draft.value).value,
    };
  return {
    path: normalizePredicatePath(draft.path),
    operator: draft.operator,
    value: draft.value,
  };
}

export function predicateToDraft(
  predicate: FieldPredicate,
  id: string = crypto.randomUUID(),
): PredicateDraft {
  return {
    id,
    path: predicate.path,
    operator: predicate.operator,
    value:
      predicate.operator === "equals" && typeof predicate.value === "string"
        ? JSON.stringify(predicate.value)
        : String(predicate.value),
  };
}

export function modeQuery(mode: QueryMode, drafts: Record<QueryMode, string>) {
  return mode === "structured" ? undefined : drafts[mode];
}

export function validateCustomRange(start: string, end: string) {
  if (!start || !end) return "Choose both a start and end time";
  const startTime = new Date(start).getTime();
  const endTime = new Date(end).getTime();
  if (!Number.isFinite(startTime) || !Number.isFinite(endTime))
    return "Enter valid start and end times";
  if (startTime >= endTime) return "Start time must be before end time";
  if (endTime - startTime > 604_800_000)
    return "Range exceeds the 7 day maximum";
  return null;
}
