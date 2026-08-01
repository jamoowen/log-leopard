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

export function validatePredicate(draft: PredicateDraft): string | null {
  const path = draft.path.trim();
  if (
    !pathPattern.test(path) ||
    path.split(".").some((part) => blockedPathParts.has(part))
  )
    return "Use a safe dot path with letters, digits, and underscores.";
  if (draft.operator === "exists")
    return draft.value === "true" || draft.value === "false"
      ? null
      : "Choose true or false.";
  if (draft.operator === "gt" || draft.operator === "lt")
    return draft.value.trim() !== "" && Number.isFinite(Number(draft.value))
      ? null
      : "Enter a number.";
  if (draft.operator === "contains")
    return draft.value.length > 0 ? null : "Enter text to match.";
  return parseEquals(draft.value).valid
    ? null
    : "Enter text, a number, true, or false.";
}

function parseEquals(value: string): {
  valid: boolean;
  value: string | number | boolean;
} {
  const clean = value.trim();
  if (!clean) return { valid: false, value: "" };
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
      path: draft.path.trim(),
      operator: draft.operator,
      value: draft.value === "true",
    };
  if (draft.operator === "gt" || draft.operator === "lt")
    return {
      path: draft.path.trim(),
      operator: draft.operator,
      value: Number(draft.value),
    };
  if (draft.operator === "equals")
    return {
      path: draft.path.trim(),
      operator: draft.operator,
      value: parseEquals(draft.value).value,
    };
  return {
    path: draft.path.trim(),
    operator: draft.operator,
    value: draft.value,
  };
}

export function predicateToDraft(
  predicate: FieldPredicate,
  id = crypto.randomUUID(),
): PredicateDraft {
  return {
    id,
    path: predicate.path,
    operator: predicate.operator,
    value: String(predicate.value),
  };
}

export function modeQuery(mode: QueryMode, drafts: Record<QueryMode, string>) {
  return mode === "structured" ? undefined : drafts[mode];
}
