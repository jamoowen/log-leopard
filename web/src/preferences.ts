import type { FieldPredicate, QueryMode, Severity } from "./api/types";
import type { PredicateDraft } from "./query-tools";

const KEY = "logleopard.preferences.v1";
const HISTORY_KEY = "logleopard.history.v1";
const SAVED_KEY = "logleopard.saved-queries.v1";
const PINS_KEY = "logleopard.field-pins.v1";
const queryModes: QueryMode[] = ["leopard", "structured", "native"];
const severities: Severity[] = [
  "DEFAULT",
  "DEBUG",
  "INFO",
  "NOTICE",
  "WARNING",
  "ERROR",
  "CRITICAL",
];
const presets = new Set(["5m", "15m", "1h", "6h", "24h", "7d"]);
const predicateOperators = new Set<FieldPredicate["operator"]>([
  "equals",
  "contains",
  "exists",
  "gt",
  "lt",
]);
const safePath = /^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$/;
const blockedPathParts = new Set(["__proto__", "prototype", "constructor"]);
let fallbackDraftId = 0;

export type PollInterval = 0 | 5 | 10 | 30;
export interface SavedQuery {
  id: string;
  name: string;
  profileId: string;
  sources: string[];
  preset: string;
  mode: QueryMode;
  query?: string;
  predicates?: FieldPredicate[];
  severities: Severity[];
  sort: "newest";
  display: Preferences["display"];
}

export interface Preferences {
  theme: "dark" | "light";
  timezone: "local" | "utc";
  display: "compact" | "structured" | "raw";
  profileId: string;
  sources: string[];
  severities: Severity[];
  preset: string;
  queryMode: QueryMode;
  drafts: Record<QueryMode, string>;
  predicateDrafts: PredicateDraft[];
  queryDraftsEnabled: boolean;
  historyEnabled: boolean;
  localRecipesEnabled: boolean;
  polling: PollInterval;
}

export const defaults: Preferences = {
  theme: "dark",
  timezone: "local",
  display: "compact",
  profileId: "",
  sources: [],
  severities: [],
  preset: "15m",
  queryMode: "leopard",
  drafts: {
    leopard: "service:payments severity:error",
    structured: "",
    native: "severity >= ERROR",
  },
  predicateDrafts: [],
  queryDraftsEnabled: false,
  historyEnabled: false,
  localRecipesEnabled: false,
  polling: 0,
};

function isPlainObject(value: unknown): value is Record<string, unknown> {
  if (value === null || typeof value !== "object" || Array.isArray(value))
    return false;
  const prototype = Object.getPrototypeOf(value);
  return prototype === Object.prototype || prototype === null;
}

function enumValue<T extends string>(
  value: unknown,
  values: readonly T[],
  fallback: T,
): T {
  return typeof value === "string" && values.includes(value as T)
    ? (value as T)
    : fallback;
}

function stringArray(
  value: unknown,
  limit: number,
  validate: (item: string) => boolean = () => true,
): string[] {
  if (!Array.isArray(value)) return [];
  return [
    ...new Set(
      value.filter(
        (item): item is string => typeof item === "string" && validate(item),
      ),
    ),
  ].slice(0, limit);
}

function isSafePath(value: string): boolean {
  return (
    safePath.test(value) &&
    value.split(".").every((part) => !blockedPathParts.has(part))
  );
}

function safeSet(storage: Storage, key: string, value: unknown): void {
  try {
    storage.setItem(key, JSON.stringify(value));
  } catch {
    /* local storage is best-effort */
  }
}

function safeRemove(storage: Storage, key: string): void {
  try {
    storage.removeItem(key);
  } catch {
    /* local storage is best-effort */
  }
}

function uniqueDraftId(candidate: string, used: Set<string>): string {
  if (candidate && !used.has(candidate)) return candidate;
  for (let attempt = 0; attempt < 4; attempt++) {
    const generated = globalThis.crypto?.randomUUID?.();
    if (generated && !used.has(generated)) return generated;
  }
  let generated: string;
  do {
    generated = `restored-predicate-${++fallbackDraftId}`;
  } while (used.has(generated));
  return generated;
}

function severityArray(value: unknown): Severity[] {
  return stringArray(value, severities.length, (item) =>
    severities.includes(item as Severity),
  ) as Severity[];
}

function sanitizePredicateDrafts(value: unknown): PredicateDraft[] {
  if (!Array.isArray(value)) return [];
  const ids = new Set<string>();
  return value
    .flatMap((item) => {
      if (
        !isPlainObject(item) ||
        typeof item.id !== "string" ||
        typeof item.path !== "string" ||
        (item.path !== "" && !isSafePath(item.path)) ||
        typeof item.value !== "string" ||
        !predicateOperators.has(item.operator as FieldPredicate["operator"])
      )
        return [];
      const id = uniqueDraftId(item.id, ids);
      ids.add(id);
      return [
        {
          id,
          path: item.path,
          operator: item.operator as PredicateDraft["operator"],
          value: item.value,
        },
      ];
    })
    .slice(0, 50);
}

function sanitizePredicates(value: unknown): FieldPredicate[] {
  if (!Array.isArray(value)) return [];
  return value
    .flatMap((item) => {
      if (
        !isPlainObject(item) ||
        typeof item.path !== "string" ||
        !isSafePath(item.path) ||
        !predicateOperators.has(item.operator as FieldPredicate["operator"])
      )
        return [];
      const operator = item.operator as FieldPredicate["operator"];
      const validValue =
        operator === "exists"
          ? typeof item.value === "boolean"
          : operator === "gt" || operator === "lt"
            ? typeof item.value === "number" && Number.isFinite(item.value)
            : operator === "contains"
              ? typeof item.value === "string"
              : typeof item.value === "string" ||
                typeof item.value === "boolean" ||
                (typeof item.value === "number" && Number.isFinite(item.value));
      return validValue
        ? [
            {
              path: item.path,
              operator,
              value: item.value,
            },
          ]
        : [];
    })
    .slice(0, 50);
}

export function loadPreferences(storage: Storage = localStorage): Preferences {
  try {
    const parsed: unknown = JSON.parse(storage.getItem(KEY) ?? "{}");
    const stored = isPlainObject(parsed) ? parsed : {};
    const drafts = isPlainObject(stored.drafts) ? stored.drafts : {};
    const privacyControlsConfigured =
      typeof stored.queryDraftsEnabled === "boolean";
    if (!privacyControlsConfigured) {
      safeRemove(storage, HISTORY_KEY);
      safeRemove(storage, SAVED_KEY);
    }
    const queryDraftsEnabled = stored.queryDraftsEnabled === true;
    return {
      theme: enumValue(stored.theme, ["dark", "light"], defaults.theme),
      timezone: enumValue(stored.timezone, ["local", "utc"], defaults.timezone),
      display: enumValue(
        stored.display,
        ["compact", "structured", "raw"],
        defaults.display,
      ),
      profileId:
        typeof stored.profileId === "string"
          ? stored.profileId
          : defaults.profileId,
      sources: stringArray(stored.sources, 100, Boolean),
      severities: severityArray(stored.severities),
      preset:
        typeof stored.preset === "string" && presets.has(stored.preset)
          ? stored.preset
          : defaults.preset,
      queryMode: enumValue(stored.queryMode, queryModes, defaults.queryMode),
      drafts: queryDraftsEnabled
        ? {
            leopard:
              typeof drafts.leopard === "string"
                ? drafts.leopard
                : defaults.drafts.leopard,
            structured:
              typeof drafts.structured === "string"
                ? drafts.structured
                : defaults.drafts.structured,
            native:
              typeof drafts.native === "string"
                ? drafts.native
                : defaults.drafts.native,
          }
        : defaults.drafts,
      predicateDrafts: queryDraftsEnabled
        ? sanitizePredicateDrafts(stored.predicateDrafts)
        : [],
      queryDraftsEnabled,
      historyEnabled:
        privacyControlsConfigured && typeof stored.historyEnabled === "boolean"
          ? stored.historyEnabled
          : defaults.historyEnabled,
      localRecipesEnabled:
        privacyControlsConfigured &&
        typeof stored.localRecipesEnabled === "boolean"
          ? stored.localRecipesEnabled
          : defaults.localRecipesEnabled,
      polling:
        typeof stored.polling === "number" &&
        [0, 5, 10, 30].includes(stored.polling)
          ? (stored.polling as PollInterval)
          : defaults.polling,
    };
  } catch {
    return defaults;
  }
}

export function savePreferences(
  value: Preferences,
  storage: Storage = localStorage,
) {
  if (value.queryDraftsEnabled) {
    safeSet(storage, KEY, value);
    return;
  }
  const safe: Partial<Preferences> = { ...value };
  delete safe.drafts;
  delete safe.predicateDrafts;
  safeSet(storage, KEY, safe);
}

export function loadHistory(storage: Storage = localStorage): string[] {
  try {
    return stringArray(JSON.parse(storage.getItem(HISTORY_KEY) ?? "[]"), 50);
  } catch {
    return [];
  }
}

export function addHistory(query: string, storage: Storage = localStorage) {
  const clean = query.trim();
  if (!clean) return;
  safeSet(
    storage,
    HISTORY_KEY,
    [clean, ...loadHistory(storage).filter((item) => item !== clean)].slice(
      0,
      50,
    ),
  );
}

export function clearHistory(storage: Storage = localStorage) {
  safeRemove(storage, HISTORY_KEY);
}

export function loadSavedQueries(
  storage: Storage = localStorage,
): SavedQuery[] {
  try {
    const value: unknown = JSON.parse(storage.getItem(SAVED_KEY) ?? "[]");
    if (!Array.isArray(value)) return [];
    return value
      .flatMap((item): SavedQuery[] => {
        if (
          !isPlainObject(item) ||
          typeof item.id !== "string" ||
          typeof item.name !== "string" ||
          typeof item.profileId !== "string"
        )
          return [];
        const mode = enumValue(item.mode, queryModes, defaults.queryMode);
        return [
          {
            id: item.id,
            name: item.name,
            profileId: item.profileId,
            sources: stringArray(item.sources, 100, Boolean),
            preset:
              typeof item.preset === "string" && presets.has(item.preset)
                ? item.preset
                : defaults.preset,
            mode,
            ...(mode === "structured"
              ? { predicates: sanitizePredicates(item.predicates) }
              : { query: typeof item.query === "string" ? item.query : "" }),
            severities: severityArray(item.severities),
            sort: "newest",
            display: enumValue(
              item.display,
              ["compact", "structured", "raw"],
              defaults.display,
            ),
          },
        ];
      })
      .slice(0, 100);
  } catch {
    return [];
  }
}

export function saveSavedQueries(
  value: SavedQuery[],
  storage: Storage = localStorage,
) {
  safeSet(storage, SAVED_KEY, value.slice(0, 100));
}

export function clearSavedQueries(storage: Storage = localStorage) {
  safeRemove(storage, SAVED_KEY);
}

export function loadFieldPins(
  profileId: string,
  storage: Storage = localStorage,
): string[] {
  if (!profileId) return [];
  try {
    const value: unknown = JSON.parse(storage.getItem(PINS_KEY) ?? "{}");
    return isPlainObject(value)
      ? stringArray(value[profileId], 12, isSafePath)
      : [];
  } catch {
    return [];
  }
}

export function saveFieldPins(
  profileId: string,
  pins: string[],
  storage: Storage = localStorage,
) {
  if (!profileId) return;
  let all: Record<string, string[]> = {};
  try {
    const parsed: unknown = JSON.parse(storage.getItem(PINS_KEY) ?? "{}");
    if (isPlainObject(parsed))
      all = Object.fromEntries(
        Object.entries(parsed).filter(([, value]) => Array.isArray(value)),
      ) as Record<string, string[]>;
  } catch {
    /* replace corrupt pin storage */
  }
  all[profileId] = stringArray(pins, 12, isSafePath);
  safeSet(storage, PINS_KEY, all);
}
