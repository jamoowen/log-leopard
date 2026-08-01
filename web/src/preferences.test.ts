import {
  addHistory,
  clearHistory,
  clearSavedQueries,
  defaults,
  loadFieldPins,
  loadHistory,
  loadPreferences,
  loadSavedQueries,
  saveFieldPins,
  savePreferences,
  saveSavedQueries,
} from "./preferences";

describe("preference persistence", () => {
  beforeEach(() => localStorage.clear());

  it("round trips non-result preferences", () => {
    savePreferences({
      ...defaults,
      theme: "light",
      sources: ["api"],
      queryDraftsEnabled: true,
      drafts: { ...defaults.drafts, leopard: "severity:error" },
    });
    expect(loadPreferences()).toMatchObject({
      theme: "light",
      sources: ["api"],
      drafts: { leopard: "severity:error" },
    });
    expect(localStorage.getItem("logleopard.preferences.v1")).not.toContain(
      "entries",
    );
  });

  it("does not persist query drafts without explicit opt-in", () => {
    savePreferences({
      ...defaults,
      drafts: { ...defaults.drafts, leopard: "customer-secret" },
      predicateDrafts: [
        { id: "one", path: "customer.id", operator: "equals", value: "42" },
      ],
    });
    const stored = localStorage.getItem("logleopard.preferences.v1") ?? "";
    expect(stored).not.toContain("customer-secret");
    expect(stored).not.toContain("customer.id");
    expect(loadPreferences().drafts).toEqual(defaults.drafts);
  });

  it("deduplicates and bounds history and can clear it", () => {
    for (let index = 0; index < 55; index++) addHistory(`query-${index}`);
    addHistory("query-54");
    expect(loadHistory()).toHaveLength(50);
    expect(loadHistory()[0]).toBe("query-54");
    clearHistory();
    expect(loadHistory()).toEqual([]);
  });

  it("restores query drafts only after explicit opt-in", () => {
    localStorage.setItem(
      "logleopard.preferences.v1",
      JSON.stringify({
        drafts: { leopard: "old query" },
        queryDraftsEnabled: true,
        theme: "light",
      }),
    );
    expect(loadPreferences().drafts).toEqual({
      leopard: "old query",
      structured: "",
      native: "severity >= ERROR",
    });
    expect(loadPreferences().predicateDrafts).toEqual([]);
  });

  it("clears query data saved under the legacy default-on policy", () => {
    localStorage.setItem(
      "logleopard.preferences.v1",
      JSON.stringify({ historyEnabled: true, localRecipesEnabled: true }),
    );
    localStorage.setItem("logleopard.history.v1", JSON.stringify(["secret"]));
    localStorage.setItem(
      "logleopard.saved-queries.v1",
      JSON.stringify([{ name: "secret" }]),
    );

    const preferences = loadPreferences();
    expect(preferences.historyEnabled).toBe(false);
    expect(preferences.localRecipesEnabled).toBe(false);
    expect(loadHistory()).toEqual([]);
    expect(loadSavedQueries()).toEqual([]);
  });

  it("sanitizes every preference member from hostile storage", () => {
    localStorage.setItem(
      "logleopard.preferences.v1",
      JSON.stringify({
        theme: [],
        timezone: "elsewhere",
        display: null,
        profileId: 4,
        sources: ["api", null, "api", ""],
        severities: ["ERROR", "NOPE", {}],
        preset: "forever",
        queryMode: "sql",
        drafts: { leopard: 7, structured: "kept", native: null },
        predicateDrafts: [
          null,
          { id: "ok", path: "request.id", operator: "equals", value: "x" },
          { id: "bad", path: [], operator: "boom", value: {} },
        ],
        historyEnabled: "yes",
        localRecipesEnabled: 1,
        queryDraftsEnabled: "yes",
        polling: 1,
      }),
    );
    expect(loadPreferences()).toEqual({
      ...defaults,
      sources: ["api"],
      severities: ["ERROR"],
      drafts: defaults.drafts,
      predicateDrafts: [],
    });
  });

  it("regenerates empty and duplicate predicate draft IDs", () => {
    localStorage.setItem(
      "logleopard.preferences.v1",
      JSON.stringify({
        queryDraftsEnabled: true,
        predicateDrafts: [
          { id: "same", path: "one", operator: "equals", value: "1" },
          { id: "same", path: "two", operator: "equals", value: "2" },
          { id: "", path: "three", operator: "equals", value: "3" },
        ],
      }),
    );
    const ids = loadPreferences().predicateDrafts.map((draft) => draft.id);
    expect(ids[0]).toBe("same");
    expect(ids.every(Boolean)).toBe(true);
    expect(new Set(ids)).toHaveProperty("size", 3);
  });

  it("keeps only string history members", () => {
    localStorage.setItem(
      "logleopard.history.v1",
      JSON.stringify(["query", null, 4, {}, "query"]),
    );
    expect(loadHistory()).toEqual(["query"]);
  });

  it("stores named recipes without results or absolute timestamps", () => {
    saveSavedQueries([
      {
        id: "one",
        name: "Errors",
        profileId: "profile",
        sources: [],
        preset: "15m",
        mode: "leopard",
        query: "severity:error",
        severities: ["ERROR"],
        sort: "newest",
        display: "compact",
      },
    ]);
    expect(loadSavedQueries()).toHaveLength(1);
    expect(localStorage.getItem("logleopard.saved-queries.v1")).not.toMatch(
      /entries|start|end/,
    );
  });

  it("migrates complete safe recipe shapes and rejects hostile predicates", () => {
    localStorage.setItem(
      "logleopard.saved-queries.v1",
      JSON.stringify([
        {
          id: "old",
          name: "Old",
          profileId: "profile",
          sources: ["api", 3],
          preset: "bad",
          mode: "structured",
          severities: ["ERROR", "BAD"],
          predicates: [
            { path: "safe.path", operator: "gt", value: 3 },
            { path: "__proto__.x", operator: "equals", value: "x" },
          ],
        },
        null,
        { id: 4, name: "bad", profileId: "profile" },
      ]),
    );
    expect(loadSavedQueries()).toEqual([
      {
        id: "old",
        name: "Old",
        profileId: "profile",
        sources: ["api"],
        preset: "15m",
        mode: "structured",
        predicates: [{ path: "safe.path", operator: "gt", value: 3 }],
        severities: ["ERROR"],
        sort: "newest",
        display: "compact",
      },
    ]);
  });

  it("isolates field pins by connection", () => {
    saveFieldPins("one", ["request.id"]);
    saveFieldPins("two", ["trace"]);
    expect(loadFieldPins("one")).toEqual(["request.id"]);
    expect(loadFieldPins("two")).toEqual(["trace"]);
  });

  it("requires a plain pin map and safe string paths", () => {
    localStorage.setItem("logleopard.field-pins.v1", "null");
    expect(() =>
      saveFieldPins("one", ["safe.path", "__proto__.polluted", "safe.path"]),
    ).not.toThrow();
    expect(loadFieldPins("one")).toEqual(["safe.path"]);

    localStorage.setItem(
      "logleopard.field-pins.v1",
      JSON.stringify({ one: ["constructor.x", "request.id", 4] }),
    );
    expect(loadFieldPins("one")).toEqual(["request.id"]);
  });

  it("contains every storage read, write, and removal failure", () => {
    const throwingStorage: Storage = {
      length: 0,
      clear: () => {
        throw new Error("unavailable");
      },
      getItem: () => {
        throw new Error("unavailable");
      },
      key: () => {
        throw new Error("unavailable");
      },
      removeItem: () => {
        throw new Error("unavailable");
      },
      setItem: () => {
        throw new Error("unavailable");
      },
    };
    expect(loadPreferences(throwingStorage)).toEqual(defaults);
    expect(loadHistory(throwingStorage)).toEqual([]);
    expect(loadSavedQueries(throwingStorage)).toEqual([]);
    expect(loadFieldPins("profile", throwingStorage)).toEqual([]);
    expect(() => savePreferences(defaults, throwingStorage)).not.toThrow();
    expect(() => addHistory("query", throwingStorage)).not.toThrow();
    expect(() => clearHistory(throwingStorage)).not.toThrow();
    expect(() => saveSavedQueries([], throwingStorage)).not.toThrow();
    expect(() => clearSavedQueries(throwingStorage)).not.toThrow();
    expect(() =>
      saveFieldPins("profile", ["request.id"], throwingStorage),
    ).not.toThrow();
  });
});
