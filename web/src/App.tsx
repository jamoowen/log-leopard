import { useEffect, useMemo, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useVirtualizer } from "@tanstack/react-virtual";
import * as Select from "@radix-ui/react-select";
import * as Tabs from "@radix-ui/react-tabs";
import {
  AlertCircle,
  Activity,
  Bookmark,
  Check,
  ChevronDown,
  ChevronRight,
  CircleDot,
  Clock3,
  Command as CommandIcon,
  Database,
  History,
  Layers3,
  Moon,
  PanelRightClose,
  Play,
  Plus,
  RotateCw,
  Search,
  Settings2,
  Sun,
  Zap,
} from "lucide-react";
import { api } from "./api/client";
import {
  ApiError,
  type LogEntry,
  type Profile,
  type ProfileInput,
  type QueryMode,
  type QueryRequest,
  type Severity,
} from "./api/types";
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
  saveHealthPreferences,
  savePreferences,
  saveSavedQueries,
  type PollInterval,
  type Preferences,
  type SavedQuery,
  type HealthWindow,
} from "./preferences";
import { compactValue, discoverFields, valueAtPath } from "./field-browser";
import {
  predicateToDraft,
  toPredicate,
  validateCustomRange,
  type PredicateOperator,
} from "./query-tools";
import { CommandPalette, type Command } from "./components/CommandPalette";
import { FieldBrowser } from "./components/FieldBrowser";
import { FleetOverview } from "./components/FleetOverview";
import { JsonText } from "./components/JsonText";
import { ProfileDialog } from "./components/ProfileDialog";
import { ServiceHealth } from "./components/ServiceHealth";
import { StructuredBuilder } from "./components/StructuredBuilder";

const severities: Severity[] = [
  "DEBUG",
  "INFO",
  "NOTICE",
  "WARNING",
  "ERROR",
  "CRITICAL",
];
const presets = [
  { id: "5m", label: "5 min", ms: 300_000 },
  { id: "15m", label: "15 min", ms: 900_000 },
  { id: "1h", label: "1 hour", ms: 3_600_000 },
  { id: "6h", label: "6 hours", ms: 21_600_000 },
  { id: "24h", label: "24 hours", ms: 86_400_000 },
  { id: "7d", label: "7 days", ms: 604_800_000 },
];
const modeLabels: Record<QueryMode, string> = {
  leopard: "LogLeopard",
  structured: "Structured builder",
  native: "Native GCP",
};
const pollIntervals: PollInterval[] = [0, 5, 10, 30];
type PrimaryView = "fleet" | "health" | "logs";

function initialPrimaryView(): PrimaryView {
  const view = new URLSearchParams(window.location.search).get("view");
  return view === "health" || view === "logs" ? view : "fleet";
}
interface Execution {
  request: QueryRequest;
  run: number;
}

function formatTime(timestamp: string, timezone: Preferences["timezone"]) {
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    fractionalSecondDigits: 3,
    hour12: false,
    timeZone: timezone === "utc" ? "UTC" : undefined,
  }).format(new Date(timestamp));
}

function capitalize(value: string) {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function toDatetimeLocal(timestamp: string) {
  const date = new Date(timestamp);
  return new Date(date.getTime() - date.getTimezoneOffset() * 60_000)
    .toISOString()
    .slice(0, 16);
}

function App() {
  const [prefs, setPrefs] = useState(loadPreferences);
  const [selected, setSelected] = useState<LogEntry | null>(null);
  const [contextSelected, setContextSelected] = useState<LogEntry | null>(null);
  const [inspectorTab, setInspectorTab] = useState("overview");
  const [profileOpen, setProfileOpen] = useState(false);
  const [editingProfile, setEditingProfile] = useState<Profile | undefined>();
  const [sourceOpen, setSourceOpen] = useState(false);
  const [manualSource, setManualSource] = useState("");
  const [customOpen, setCustomOpen] = useState(false);
  const [customStart, setCustomStart] = useState("");
  const [customEnd, setCustomEnd] = useState("");
  const [history, setHistory] = useState(() =>
    prefs.historyEnabled ? loadHistory() : [],
  );
  const [saved, setSaved] = useState(() =>
    prefs.localRecipesEnabled ? loadSavedQueries() : [],
  );
  const [savedOpen, setSavedOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [saveName, setSaveName] = useState("");
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [executed, setExecuted] = useState<Execution | null>(null);
  const [appView, setAppView] = useState<PrimaryView>(initialPrimaryView);
  const [healthRevision, setHealthRevision] = useState(0);
  const [pins, setPins] = useState<string[]>([]);
  const pairingToken = useRef(
    new URLSearchParams(window.location.hash.slice(1)).get("pair"),
  );
  const [sessionState, setSessionState] = useState<
    "checking" | "pairing" | "ready" | "unpaired"
  >(() => (pairingToken.current ? "pairing" : "checking"));
  const [sessionRevision, setSessionRevision] = useState(0);
  const paletteTriggerRef = useRef<HTMLButtonElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const inFlightRef = useRef(false);
  const runRef = useRef(0);
  const previousProfileRef = useRef<string | undefined>(undefined);
  const executeRef = useRef<(poll?: boolean) => void>(() => undefined);
  const queryClient = useQueryClient();

  function update(patch: Partial<Preferences>) {
    setPrefs((current) => ({ ...current, ...patch }));
  }
  useEffect(() => {
    savePreferences(prefs);
    if (!prefs.historyEnabled) clearHistory();
    if (!prefs.localRecipesEnabled) clearSavedQueries();
    document.documentElement.dataset.theme = prefs.theme;
  }, [prefs]);
  useEffect(() => {
    const token = pairingToken.current;
    if (!token) return;
    pairingToken.current = null;
    async function pairSession(tokenValue: string) {
      try {
        await api.pair(tokenValue);
        setSessionRevision((revision) => revision + 1);
        setSessionState("ready");
      } catch {
        setSessionState("unpaired");
      } finally {
        window.history.replaceState(
          null,
          "",
          `${location.pathname}${location.search}`,
        );
      }
    }
    void pairSession(token);
  }, []);
  useEffect(() => {
    const url = new URL(window.location.href);
    if (appView === "fleet") url.searchParams.delete("view");
    else url.searchParams.set("view", appView);
    const next = `${url.pathname}${url.search}${url.hash}`;
    const current = `${window.location.pathname}${window.location.search}${window.location.hash}`;
    if (next !== current) window.history.replaceState(null, "", next);
  }, [appView]);

  const authStatus = useQuery({
    queryKey: ["auth-status"],
    queryFn: () => api.authStatus(),
    enabled: sessionState === "checking",
  });
  const sessionPhase =
    sessionState === "checking" && authStatus.isSuccess
      ? "ready"
      : sessionState === "checking" && authStatus.isError
        ? "unpaired"
        : sessionState;
  const profiles = useQuery({
    queryKey: ["profiles", sessionRevision],
    queryFn: () => api.profiles(),
    enabled: sessionPhase === "ready",
  });
  const profileList = profiles.data;
  const activeProfile =
    profileList?.find((profile) => profile.id === prefs.profileId) ??
    profileList?.[0];
  useEffect(() => {
    if (!prefs.profileId && profileList?.[0])
      update({ profileId: profileList[0].id });
  }, [profileList, prefs.profileId]);
  useEffect(() => {
    setPins(loadFieldPins(activeProfile?.id ?? ""));
  }, [activeProfile?.id]);
  const profileUnauthorized =
    profiles.error instanceof ApiError && profiles.error.status === 401;
  const sources = useQuery({
    queryKey: ["sources", activeProfile?.id],
    queryFn: () => api.sources(activeProfile!.id),
    enabled: sessionPhase === "ready" && Boolean(activeProfile),
  });
  const saveProfile = useMutation({
    mutationFn: ({ input, id }: { input: ProfileInput; id?: string }) =>
      api.saveProfile(input, id),
    onSuccess: (profile) => {
      const previous = profileList?.find((item) => item.id === profile.id);
      void queryClient.invalidateQueries({ queryKey: ["profiles"] });
      if (previous && previous.projectId !== profile.projectId) {
        saveHealthPreferences(profile.id, { target: "", window: "1h" });
        setHealthRevision((revision) => revision + 1);
        void queryClient.resetQueries({ queryKey: ["sources", profile.id] });
        void queryClient.resetQueries({
          queryKey: ["service-health", profile.id],
        });
        void queryClient.resetQueries({
          queryKey: ["fleet-overview", profile.id],
        });
      }
      update({ profileId: profile.id });
    },
  });
  const activeExecution =
    executed?.request.profileId === activeProfile?.id ? executed : null;
  const results = useInfiniteQuery({
    queryKey: ["query", activeExecution?.request, activeExecution?.run],
    enabled: sessionPhase === "ready" && Boolean(activeExecution),
    initialPageParam: undefined as string | undefined,
    queryFn: ({ pageParam, signal }) =>
      api.query(
        {
          ...activeExecution!.request,
          ...(pageParam ? { cursor: pageParam } : {}),
        },
        signal,
      ),
    getNextPageParam: (page) => page.nextCursor,
  });
  useEffect(() => {
    inFlightRef.current = results.isFetching;
  }, [results.isFetching]);
  useEffect(() => {
    const profileId = activeProfile?.id;
    if (
      previousProfileRef.current &&
      previousProfileRef.current !== profileId
    ) {
      void queryClient.cancelQueries({ queryKey: ["query"] });
      void queryClient.cancelQueries({ queryKey: ["request-context"] });
      setExecuted(null);
      setSelected(null);
      setContextSelected(null);
    }
    previousProfileRef.current = profileId;
  }, [activeProfile?.id, queryClient]);
  const entries = useMemo(
    () =>
      activeExecution
        ? (results.data?.pages.flatMap((page) => page.entries ?? []) ?? [])
        : [],
    [activeExecution, results.data?.pages],
  );
  const fields = useMemo(() => discoverFields(entries), [entries]);
  const expired = Boolean(
    results.data?.pages.some((page) => new Date(page.expiresAt) < new Date()),
  );
  const queryUnauthorized =
    results.error instanceof ApiError && results.error.status === 401;
  const sessionUnavailable =
    sessionPhase === "unpaired" || profileUnauthorized || queryUnauthorized;
  const sessionReady = sessionPhase === "ready" && !sessionUnavailable;
  const discoveredSources = sources.data?.sources ?? [];
  const selectableSources = discoveredSources.filter(
    (source) => source.id.trim() !== "",
  );
  const discoveryFallback = discoveredSources.find(
    (source) => source.id.trim() === "",
  );
  const selectedSources = prefs.sources.filter(Boolean);
  const customRangeError = customOpen
    ? validateCustomRange(customStart, customEnd)
    : null;
  const customRangeInvalid = customRangeError !== null;
  const predicates = prefs.predicateDrafts.map(toPredicate);
  const predicatesValid = predicates.every(Boolean) && predicates.length <= 50;

  const context = useQuery({
    queryKey: [
      "request-context",
      activeExecution?.request.profileId,
      selected?.id,
    ],
    queryFn: ({ signal }) =>
      api.requestContext(
        {
          profileId: activeExecution!.request.profileId,
          eventTimestamp: selected!.timestamp,
          ...(selected!.requestId ? { requestId: selected!.requestId } : {}),
          ...(selected!.trace ? { traceId: selected!.trace } : {}),
        },
        signal,
      ),
    enabled:
      inspectorTab === "context" &&
      Boolean(
        activeExecution && selected && (selected.requestId || selected.trace),
      ),
  });

  // TanStack Virtual owns an imperative measurement cache and is intentionally not compiler-memoized.
  // eslint-disable-next-line react-hooks/incompatible-library
  const rowVirtualizer = useVirtualizer({
    count: entries.length,
    getScrollElement: () => listRef.current,
    estimateSize: () =>
      prefs.display === "compact" && pins.length === 0 ? 38 : 58,
    overscan: 12,
  });

  function execute(poll = false) {
    if (
      appView !== "logs" ||
      !activeProfile ||
      !sessionReady ||
      inFlightRef.current ||
      customRangeInvalid ||
      (prefs.queryMode === "structured" && !predicatesValid)
    )
      return;
    const preset =
      presets.find((item) => item.id === prefs.preset) ?? presets[1]!;
    const useCustom = !poll && customOpen;
    const end = useCustom && customEnd ? new Date(customEnd) : new Date();
    const start =
      useCustom && customStart
        ? new Date(customStart)
        : new Date(end.getTime() - preset.ms);
    const request: QueryRequest = {
      profileId: activeProfile.id,
      mode: prefs.queryMode,
      sources: selectedSources,
      severities: prefs.severities,
      start: start.toISOString(),
      end: end.toISOString(),
      limit: 80,
      ...(prefs.queryMode === "structured"
        ? { predicates: predicates as NonNullable<QueryRequest["predicates"]> }
        : { query: prefs.drafts[prefs.queryMode] }),
    };
    setSelected(null);
    setContextSelected(null);
    setExecuted({ request, run: ++runRef.current });
    if (!poll && prefs.historyEnabled && prefs.queryMode !== "structured") {
      addHistory(request.query ?? "");
      setHistory(loadHistory());
    }
  }
  executeRef.current = execute;

  function openHealthLogs(service: string, start: string, end: string) {
    if (!activeProfile) return;
    const request: QueryRequest = {
      profileId: activeProfile.id,
      mode: "native",
      query: "httpRequest.status >= 500 AND httpRequest.status < 600",
      sources: [service],
      severities: [],
      start,
      end,
      limit: 80,
    };
    update({
      sources: [service],
      severities: [],
      queryMode: "native",
      drafts: { ...prefs.drafts, native: request.query ?? "" },
      polling: 0,
    });
    setCustomStart(toDatetimeLocal(start));
    setCustomEnd(toDatetimeLocal(end));
    setCustomOpen(true);
    setSelected(null);
    setContextSelected(null);
    setExecuted({ request, run: ++runRef.current });
    setAppView("logs");
  }

  function openFleetService(service: string, window: HealthWindow) {
    if (!activeProfile) return;
    saveHealthPreferences(activeProfile.id, { target: service, window });
    setHealthRevision((revision) => revision + 1);
    setSelected(null);
    setContextSelected(null);
    setAppView("health");
  }

  function openPrimaryView(view: PrimaryView) {
    if (view !== "logs") {
      setSelected(null);
      setContextSelected(null);
    }
    setAppView(view);
  }

  useEffect(() => {
    if (!prefs.polling) return undefined;
    const timer = window.setInterval(() => {
      if (
        document.visibilityState === "visible" &&
        !selected &&
        !inFlightRef.current
      )
        executeRef.current(true);
    }, prefs.polling * 1000);
    return () => window.clearInterval(timer);
  }, [prefs.polling, selected]);

  useEffect(() => {
    function shortcut(event: KeyboardEvent) {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "k") {
        event.preventDefault();
        setPaletteOpen(true);
      }
      if (
        appView === "logs" &&
        (event.metaKey || event.ctrlKey) &&
        event.key === "Enter"
      ) {
        event.preventDefault();
        executeRef.current(false);
      }
      if (event.key === "Escape" && !paletteOpen) {
        setSelected(null);
        setSourceOpen(false);
        setCustomOpen(false);
        setSavedOpen(false);
        setHistoryOpen(false);
      }
      if (
        appView === "logs" &&
        event.key === "/" &&
        !["INPUT", "TEXTAREA", "SELECT"].includes(
          (event.target as HTMLElement).tagName,
        )
      ) {
        event.preventDefault();
        document.querySelector<HTMLTextAreaElement>("#query-input")?.focus();
      }
    }
    window.addEventListener("keydown", shortcut);
    return () => window.removeEventListener("keydown", shortcut);
  }, [appView, paletteOpen]);

  function toggleSource(id: string) {
    if (id)
      update({
        sources: selectedSources.includes(id)
          ? selectedSources.filter((item) => item !== id)
          : [...selectedSources, id],
      });
  }
  function toggleSeverity(value: Severity) {
    update({
      severities: prefs.severities.includes(value)
        ? prefs.severities.filter((item) => item !== value)
        : [...prefs.severities, value],
    });
  }
  function togglePin(path: string) {
    const next = pins.includes(path)
      ? pins.filter((item) => item !== path)
      : [...pins, path].slice(0, 12);
    setPins(next);
    if (activeProfile) saveFieldPins(activeProfile.id, next);
  }
  function addPredicate(path: string, type = "string") {
    const operator: PredicateOperator = type === "number" ? "gt" : "equals";
    update({
      queryMode: "structured",
      predicateDrafts: [
        ...prefs.predicateDrafts,
        {
          id: crypto.randomUUID(),
          path,
          operator,
          value: type === "number" ? "0" : "",
        },
      ].slice(0, 50),
    });
  }
  function persistSaved(next: SavedQuery[]) {
    setSaved(next);
    saveSavedQueries(next);
  }
  function createSaved() {
    if (
      !activeProfile ||
      !saveName.trim() ||
      !prefs.localRecipesEnabled ||
      (prefs.queryMode === "structured" && !predicatesValid)
    )
      return;
    const recipe: SavedQuery = {
      id: crypto.randomUUID(),
      name: saveName.trim(),
      profileId: activeProfile.id,
      sources: selectedSources,
      preset: prefs.preset,
      mode: prefs.queryMode,
      ...(prefs.queryMode === "structured"
        ? { predicates: predicates as NonNullable<QueryRequest["predicates"]> }
        : { query: prefs.drafts[prefs.queryMode] }),
      severities: prefs.severities,
      sort: "newest",
      display: prefs.display,
    };
    persistSaved([recipe, ...saved]);
    setSaveName("");
  }
  function applySaved(recipe: SavedQuery) {
    if (!profileList?.some((profile) => profile.id === recipe.profileId))
      return;
    update({
      profileId: recipe.profileId,
      sources: recipe.sources,
      preset: recipe.preset,
      queryMode: recipe.mode,
      severities: recipe.severities,
      display: recipe.display,
      ...(recipe.mode === "structured"
        ? {
            predicateDrafts: (recipe.predicates ?? []).map((predicate) =>
              predicateToDraft(predicate),
            ),
          }
        : { drafts: { ...prefs.drafts, [recipe.mode]: recipe.query ?? "" } }),
    });
    setCustomOpen(false);
    setSavedOpen(false);
  }
  const currentIndex = selected
    ? entries.findIndex((entry) => entry.id === selected.id)
    : -1;
  const pollStatus = !prefs.polling
    ? "Polling off"
    : selected
      ? `Polling paused · inspector open`
      : document.visibilityState === "hidden"
        ? "Polling paused · tab hidden"
        : `Polling every ${prefs.polling}s`;
  const commands: Command[] = [
    ...(
      [
        ["fleet", "Fleet overview"],
        ["health", "Service health"],
        ["logs", "Logs"],
      ] as const
    ).map(
      ([view, label]): Command => ({
        id: `view-${view}`,
        label: `Open ${label}`,
        group: "Navigation",
        run: () => openPrimaryView(view),
      }),
    ),
    ...(appView === "logs"
      ? [
          {
            id: "run",
            label: "Run query",
            group: "Query",
            hint: "⌘↵",
            run: () => executeRef.current(false),
          },
          {
            id: "focus",
            label: "Focus query editor",
            group: "Query",
            hint: "/",
            run: () =>
              document
                .querySelector<HTMLTextAreaElement>("#query-input")
                ?.focus(),
          },
        ]
      : []),
    {
      id: "theme",
      label: `Use ${prefs.theme === "dark" ? "light" : "dark"} theme`,
      group: "View",
      run: () => update({ theme: prefs.theme === "dark" ? "light" : "dark" }),
    },
    {
      id: "timezone",
      label: `Show ${prefs.timezone === "utc" ? "local time" : "UTC"}`,
      group: "View",
      run: () =>
        update({ timezone: prefs.timezone === "utc" ? "local" : "utc" }),
    },
    ...(["compact", "structured", "raw"] as const).map(
      (display): Command => ({
        id: `display-${display}`,
        label: `${capitalize(display)} rows`,
        group: "Display",
        run: () => update({ display }),
      }),
    ),
    ...pollIntervals.map(
      (polling): Command => ({
        id: `poll-${polling}`,
        label: polling ? `Poll every ${polling} seconds` : "Turn polling off",
        group: "Polling",
        run: () => {
          update({ polling });
          if (polling) setCustomOpen(false);
        },
      }),
    ),
    ...(profileList ?? []).map(
      (profile): Command => ({
        id: `profile-${profile.id}`,
        label: profile.name,
        group: "Connections",
        run: () => update({ profileId: profile.id, sources: [] }),
      }),
    ),
    ...saved.map(
      (recipe): Command => ({
        id: `saved-${recipe.id}`,
        label: recipe.name,
        group: "Saved queries",
        run: () => applySaved(recipe),
      }),
    ),
  ];

  return (
    <div className="app-shell">
      <header className="topbar">
        <div className="brand">
          <img className="brand-mark" src="/log-leopard.png" alt="" />
          <strong>Log Leopard</strong>
          <span className="edition">LOCAL</span>
        </div>
        <div className="top-actions">
          {sessionPhase !== "ready" && (
            <span
              className={`pair-state ${sessionUnavailable ? "failed" : ""}`}
            >
              {sessionPhase === "pairing"
                ? "Pairing session…"
                : sessionPhase === "checking"
                  ? "Checking session…"
                  : "Session required"}
            </span>
          )}
          <button
            ref={paletteTriggerRef}
            className="palette-trigger"
            aria-label="Open command palette"
            onClick={() => setPaletteOpen(true)}
          >
            <CommandIcon size={13} />
            <span>Commands</span>
            <kbd>⌘K</kbd>
          </button>
          <Select.Root
            value={activeProfile?.id ?? ""}
            onValueChange={(profileId) => update({ profileId, sources: [] })}
          >
            <Select.Trigger className="profile-trigger" aria-label="Connection">
              <CircleDot size={13} />
              <span>{activeProfile?.name ?? "No connection"}</span>
              <ChevronDown size={13} />
            </Select.Trigger>
            <Select.Portal>
              <Select.Content className="select-content" position="popper">
                <Select.Viewport>
                  {profileList?.map((profile) => (
                    <Select.Item
                      className="select-item"
                      value={profile.id}
                      key={profile.id}
                    >
                      <Select.ItemText>{profile.name}</Select.ItemText>
                      <Select.ItemIndicator>
                        <Check size={12} />
                      </Select.ItemIndicator>
                    </Select.Item>
                  ))}
                  <button
                    className="select-action"
                    onClick={() => {
                      setEditingProfile(undefined);
                      setProfileOpen(true);
                    }}
                  >
                    <Plus size={13} /> New connection
                  </button>
                </Select.Viewport>
              </Select.Content>
            </Select.Portal>
          </Select.Root>
          <button
            className="icon-button desktop-only"
            aria-label="Edit connection"
            onClick={() => {
              setEditingProfile(activeProfile);
              setProfileOpen(true);
            }}
          >
            <Settings2 size={15} />
          </button>
          <button
            className="icon-button"
            aria-label="Toggle theme"
            onClick={() =>
              update({ theme: prefs.theme === "dark" ? "light" : "dark" })
            }
          >
            {prefs.theme === "dark" ? <Sun size={15} /> : <Moon size={15} />}
          </button>
        </div>
      </header>

      <nav className="app-view-tabs" aria-label="Primary view">
        <button
          type="button"
          className={appView === "fleet" ? "active" : ""}
          aria-pressed={appView === "fleet"}
          onClick={() => openPrimaryView("fleet")}
        >
          <Layers3 size={13} /> Fleet overview
        </button>
        <button
          type="button"
          className={appView === "health" ? "active" : ""}
          aria-pressed={appView === "health"}
          onClick={() => openPrimaryView("health")}
        >
          <Activity size={13} /> Service health
        </button>
        <button
          type="button"
          className={appView === "logs" ? "active" : ""}
          aria-pressed={appView === "logs"}
          onClick={() => openPrimaryView("logs")}
        >
          <Search size={13} /> Logs
        </button>
      </nav>

      {appView === "fleet" && (
        <FleetOverview
          key={activeProfile?.id ?? "no-profile"}
          profile={activeProfile}
          sources={discoveredSources}
          discoveryPending={sources.isPending}
          discoveryError={
            sources.error instanceof Error ? sources.error.message : undefined
          }
          discoveryWarning={sources.data?.warning}
          sessionReady={sessionReady}
          onOpenService={openFleetService}
        />
      )}

      {appView === "health" && (
        <ServiceHealth
          key={`${activeProfile?.id ?? "no-profile"}:${healthRevision}`}
          profile={activeProfile}
          sources={discoveredSources}
          discoveryPending={sources.isPending}
          discoveryError={
            sources.error instanceof Error ? sources.error.message : undefined
          }
          discoveryWarning={sources.data?.warning}
          sessionReady={sessionReady}
          onOpenLogs={openHealthLogs}
        />
      )}
      <main className="workspace" hidden={appView !== "logs"}>
        <section className="query-panel">
          <div className="control-strip">
            <div className="source-control">
              <button
                className="control-button"
                aria-expanded={sourceOpen}
                aria-controls="source-popover"
                onClick={() => setSourceOpen(!sourceOpen)}
              >
                <Database size={14} />
                <span>
                  {selectedSources.length
                    ? `${selectedSources.length} source${selectedSources.length > 1 ? "s" : ""}`
                    : "All sources"}
                </span>
                <ChevronDown size={13} />
              </button>
              {sourceOpen && (
                <div
                  id="source-popover"
                  className="popover sources-popover"
                  role="dialog"
                  aria-label="Sources"
                >
                  <div className="popover-title">
                    Sources{" "}
                    <span>
                      {sources.isFetching
                        ? "Discovering…"
                        : `${selectableSources.length} found`}
                    </span>
                  </div>
                  <button
                    aria-pressed={selectedSources.length === 0}
                    className={`all-sources ${selectedSources.length === 0 ? "active" : ""}`}
                    onClick={() => update({ sources: [] })}
                  >
                    <Check size={12} /> All sources
                  </button>
                  {sources.isError && (
                    <p className="inline-error">
                      Discovery unavailable. Add a source manually.
                    </p>
                  )}
                  {sources.data?.warning && (
                    <p className="fallback-note">{sources.data.warning}</p>
                  )}
                  {sources.isSuccess &&
                    selectableSources.length === 0 &&
                    !sources.data.warning && (
                      <p className="fallback-note">
                        {discoveryFallback
                          ? `${discoveryFallback.label}. Add a service manually to narrow results.`
                          : "No concrete services were discovered. Queries will use all sources."}
                      </p>
                    )}
                  {selectableSources.map((source) => (
                    <label className="check-row" key={source.id}>
                      <input
                        type="checkbox"
                        checked={selectedSources.includes(source.id)}
                        onChange={() => toggleSource(source.id)}
                      />
                      <span>{source.label}</span>
                      <small>{source.kind}</small>
                    </label>
                  ))}
                  <form
                    className="manual-source"
                    onSubmit={(event) => {
                      event.preventDefault();
                      if (manualSource.trim()) {
                        toggleSource(manualSource.trim());
                        setManualSource("");
                      }
                    }}
                  >
                    <input
                      aria-label="Manual service name"
                      value={manualSource}
                      onChange={(event) => setManualSource(event.target.value)}
                      placeholder="Manual service name"
                    />
                    <button>Add</button>
                  </form>
                </div>
              )}
            </div>
            <div
              className="severity-control"
              role="group"
              aria-label="Severity filters"
            >
              {severities.map((severity) => (
                <button
                  aria-pressed={prefs.severities.includes(severity)}
                  key={severity}
                  data-severity={severity}
                  className={
                    prefs.severities.includes(severity) ? "active" : ""
                  }
                  onClick={() => toggleSeverity(severity)}
                >
                  {severity}
                </button>
              ))}
            </div>
            <div className="time-control">
              <Clock3 size={13} />
              {presets.map((preset) => (
                <button
                  aria-pressed={!customOpen && prefs.preset === preset.id}
                  key={preset.id}
                  className={
                    !customOpen && prefs.preset === preset.id ? "active" : ""
                  }
                  onClick={() => {
                    update({ preset: preset.id });
                    setCustomOpen(false);
                  }}
                >
                  {preset.label}
                </button>
              ))}
              <button
                aria-pressed={customOpen}
                aria-expanded={customOpen}
                aria-controls="custom-popover"
                className={customOpen ? "active" : ""}
                onClick={() => {
                  setCustomOpen(!customOpen);
                  if (!customOpen) update({ polling: 0 });
                }}
              >
                Custom
              </button>
              {customOpen && (
                <div
                  id="custom-popover"
                  className="popover custom-popover"
                  role="dialog"
                  aria-label="Custom time range"
                >
                  <label>
                    Start
                    <input
                      aria-label="Custom start"
                      type="datetime-local"
                      value={customStart}
                      onChange={(event) => setCustomStart(event.target.value)}
                    />
                  </label>
                  <label>
                    End
                    <input
                      aria-label="Custom end"
                      type="datetime-local"
                      value={customEnd}
                      onChange={(event) => setCustomEnd(event.target.value)}
                    />
                  </label>
                  <small className={customRangeInvalid ? "range-error" : ""}>
                    {customRangeError ?? "Maximum range: 7 days"}
                  </small>
                </div>
              )}
            </div>
          </div>
          <div className="mode-tabs" role="group" aria-label="Query mode">
            {(Object.keys(modeLabels) as QueryMode[]).map((mode) => (
              <button
                aria-pressed={prefs.queryMode === mode}
                key={mode}
                className={prefs.queryMode === mode ? "active" : ""}
                onClick={() => update({ queryMode: mode })}
              >
                {modeLabels[mode]}
              </button>
            ))}
          </div>
          <div className="query-editor">
            {prefs.queryMode === "structured" ? (
              <StructuredBuilder
                drafts={prefs.predicateDrafts}
                onChange={(predicateDrafts) => update({ predicateDrafts })}
              />
            ) : (
              <textarea
                id="query-input"
                aria-label="Query"
                spellCheck={false}
                value={prefs.drafts[prefs.queryMode]}
                onChange={(event) =>
                  update({
                    drafts: {
                      ...prefs.drafts,
                      [prefs.queryMode]: event.target.value,
                    },
                  })
                }
                placeholder={
                  prefs.queryMode === "native"
                    ? "severity >= ERROR"
                    : 'timeout "connection reset" service:checkout'
                }
              />
            )}
            <div className="editor-footer">
              <span>
                {prefs.queryMode === "structured"
                  ? "Values are typed from the selected operator; conditions are combined with AND"
                  : prefs.queryMode === "leopard"
                    ? "Fields use field:value; terms are implicitly ANDed"
                    : "Passed to GCP Cloud Logging inside the fixed resource and time scope"}
              </span>
              <span className="shortcut">⌘ ↵</span>
              <button
                className="run-button"
                onClick={() => execute()}
                disabled={
                  !activeProfile ||
                  !sessionReady ||
                  results.isFetching ||
                  customRangeInvalid ||
                  (prefs.queryMode === "structured" && !predicatesValid)
                }
              >
                <Play size={14} fill="currentColor" />
                {results.isFetching ? "Running" : "Run query"}
              </button>
            </div>
          </div>
        </section>

        <section className="results-panel">
          <div className="results-toolbar">
            <div className="result-count">
              <Zap size={13} />
              {!sessionReady
                ? sessionUnavailable
                  ? "Session required"
                  : "Establishing session"
                : results.isFetching && !entries.length
                  ? "Executing query"
                  : activeExecution
                    ? `${entries.length.toLocaleString()} entries loaded`
                    : "Ready to query"}
              {(sessionPhase === "pairing" ||
                sessionPhase === "checking" ||
                results.isFetching) && <span className="pulse" />}
            </div>
            <div className="toolbar-group">
              <Select.Root
                value={String(prefs.polling)}
                onValueChange={(value) => {
                  update({ polling: Number(value) as PollInterval });
                  if (value !== "0") setCustomOpen(false);
                }}
              >
                <Select.Trigger
                  className={`poll-trigger ${prefs.polling ? "active" : ""}`}
                  aria-label="Polling"
                >
                  <span className="poll-dot" />
                  {pollStatus}
                  <ChevronDown size={12} />
                </Select.Trigger>
                <Select.Portal>
                  <Select.Content className="select-content" position="popper">
                    {pollIntervals.map((seconds) => (
                      <Select.Item
                        className="select-item"
                        value={String(seconds)}
                        key={seconds}
                      >
                        <Select.ItemText>
                          {seconds ? `Every ${seconds} seconds` : "Off"}
                        </Select.ItemText>
                      </Select.Item>
                    ))}
                  </Select.Content>
                </Select.Portal>
              </Select.Root>
              <div
                className="segmented"
                role="group"
                aria-label="Result display"
              >
                {(["compact", "structured", "raw"] as const).map((mode) => (
                  <button
                    aria-pressed={prefs.display === mode}
                    key={mode}
                    className={prefs.display === mode ? "active" : ""}
                    onClick={() => update({ display: mode })}
                  >
                    {capitalize(mode)}
                  </button>
                ))}
              </div>
              <button
                className="text-button"
                aria-pressed={prefs.timezone === "utc"}
                onClick={() =>
                  update({
                    timezone: prefs.timezone === "utc" ? "local" : "utc",
                  })
                }
              >
                {prefs.timezone.toUpperCase()}
              </button>
              <button
                className="icon-button"
                title="Saved queries"
                aria-label="Saved queries"
                aria-expanded={savedOpen}
                aria-controls="saved-panel"
                onClick={() => setSavedOpen(!savedOpen)}
              >
                <Bookmark size={14} />
              </button>
              <button
                className="icon-button"
                title="Query history"
                aria-label="Query history"
                aria-expanded={historyOpen}
                aria-controls="history-panel"
                onClick={() => setHistoryOpen(!historyOpen)}
              >
                <History size={14} />
              </button>
            </div>
          </div>
          {savedOpen && (
            <div
              id="saved-panel"
              className="saved-panel"
              role="dialog"
              aria-label="Saved queries"
            >
              <div className="saved-title">
                <strong>Saved queries</strong>
                <button
                  onClick={() => {
                    clearSavedQueries();
                    setSaved([]);
                  }}
                >
                  Clear all
                </button>
              </div>
              <p className="privacy-note">
                Recipes are stored in this browser and may contain sensitive
                query text, source names, and field values. Results are never
                stored. Draft restoration is controlled separately under query
                history.
              </p>
              <label className="storage-toggle">
                <input
                  type="checkbox"
                  checked={prefs.localRecipesEnabled}
                  onChange={(event) => {
                    if (!event.target.checked) {
                      clearSavedQueries();
                      setSaved([]);
                    }
                    update({
                      localRecipesEnabled: event.target.checked,
                    });
                  }}
                />{" "}
                Enable local recipe storage
              </label>
              {prefs.localRecipesEnabled && (
                <form
                  className="save-query-form"
                  onSubmit={(event) => {
                    event.preventDefault();
                    createSaved();
                  }}
                >
                  <input
                    aria-label="Saved query name"
                    value={saveName}
                    onChange={(event) => setSaveName(event.target.value)}
                    placeholder="Name this query"
                  />
                  <button>Save current</button>
                </form>
              )}
              {saved.map((recipe) => {
                const available = Boolean(
                  profileList?.some(
                    (profile) => profile.id === recipe.profileId,
                  ),
                );
                return (
                  <div className="saved-item" key={recipe.id}>
                    <button
                      disabled={!available}
                      title={
                        available
                          ? undefined
                          : "The saved connection no longer exists"
                      }
                      onClick={() => applySaved(recipe)}
                    >
                      <strong>{recipe.name}</strong>
                      <span>
                        {profileList?.find(
                          (profile) => profile.id === recipe.profileId,
                        )?.name ?? "Missing connection"}{" "}
                        · {recipe.preset} · {recipe.mode}
                      </span>
                    </button>
                    <button
                      aria-label={`Rename ${recipe.name}`}
                      onClick={() => {
                        const name = window
                          .prompt("Rename saved query", recipe.name)
                          ?.trim();
                        if (name)
                          persistSaved(
                            saved.map((item) =>
                              item.id === recipe.id ? { ...item, name } : item,
                            ),
                          );
                      }}
                    >
                      Rename
                    </button>
                    <button
                      aria-label={`Delete ${recipe.name}`}
                      onClick={() =>
                        persistSaved(
                          saved.filter((item) => item.id !== recipe.id),
                        )
                      }
                    >
                      Delete
                    </button>
                  </div>
                );
              })}
              {saved.length === 0 && <p>No saved queries.</p>}
            </div>
          )}
          {historyOpen && (
            <div
              id="history-panel"
              className="history-panel open"
              role="dialog"
              aria-label="Query history"
            >
              <div>
                <strong>Query history</strong>
                <label>
                  <input
                    type="checkbox"
                    checked={prefs.historyEnabled}
                    onChange={(event) => {
                      if (!event.target.checked) {
                        clearHistory();
                        setHistory([]);
                      }
                      update({
                        historyEnabled: event.target.checked,
                      });
                    }}
                  />{" "}
                  Save locally
                </label>
                <button
                  onClick={() => {
                    clearHistory();
                    setHistory([]);
                  }}
                >
                  Clear
                </button>
              </div>
              <p className="privacy-note">
                History can contain sensitive query text. It stays in this
                browser, is limited to 50 items, and never includes results.
                History storage is disabled by default.
              </p>
              <label className="storage-toggle">
                <input
                  type="checkbox"
                  checked={prefs.queryDraftsEnabled}
                  onChange={(event) =>
                    update({ queryDraftsEnabled: event.target.checked })
                  }
                />{" "}
                Restore current query drafts locally
              </label>
              <button
                className="text-button"
                onClick={() => {
                  clearHistory();
                  clearSavedQueries();
                  setHistory([]);
                  setSaved([]);
                  update({
                    drafts: defaults.drafts,
                    predicateDrafts: [],
                    queryDraftsEnabled: false,
                    historyEnabled: false,
                    localRecipesEnabled: false,
                  });
                }}
              >
                Clear all local query data
              </button>
              {history.length ? (
                history.map((item) => (
                  <button
                    key={item}
                    onClick={() =>
                      update({
                        queryMode: "leopard",
                        drafts: { ...prefs.drafts, leopard: item },
                      })
                    }
                  >
                    {item}
                  </button>
                ))
              ) : (
                <p>No query history.</p>
              )}
            </div>
          )}
          <div className={`grid-head ${pins.length ? "with-pins" : ""}`}>
            <span>TIME</span>
            <span>SEV</span>
            <span>SOURCE</span>
            <span>MESSAGE</span>
            {pins.length > 0 && <span>PINNED FIELDS</span>}
          </div>
          <div className="log-list" ref={listRef}>
            {!sessionReady && (
              <State
                icon={<AlertCircle />}
                title={
                  sessionPhase === "pairing" || sessionPhase === "checking"
                    ? "Establishing local session"
                    : "Local browser pairing required"
                }
                detail={
                  sessionPhase === "pairing"
                    ? "Exchanging the one-time pairing token before loading protected data."
                    : sessionPhase === "checking"
                      ? "Checking the browser session with the local LogLeopard service."
                      : "The one-time local pairing link is missing, expired, or already used. This happens before Google Cloud authentication. Restart LogLeopard and open the newest startup URL."
                }
                {...(sessionUnavailable ? { tone: "error" } : {})}
              />
            )}
            {sessionReady && !activeExecution && (
              <State
                icon={<Search />}
                title="Define a query"
                detail="Choose a window and run a query. Results are held in memory only."
              />
            )}
            {sessionReady && results.isFetching && !entries.length && (
              <div className="loading-lines">
                {Array.from({ length: 10 }, (_, index) => (
                  <span key={index} />
                ))}
              </div>
            )}
            {sessionReady && results.isError && (
              <State
                icon={<AlertCircle />}
                title="Query failed"
                detail={results.error.message}
                action={
                  <button onClick={() => results.refetch()}>
                    <RotateCw size={13} /> Retry
                  </button>
                }
                tone="error"
              />
            )}
            {sessionReady && expired && (
              <State
                icon={<Clock3 />}
                title="Query cursor expired"
                detail="Cursor validity elapsed. Run the query again to refresh the fixed time window."
                action={
                  <button onClick={() => execute()}>
                    <RotateCw size={13} /> Run again
                  </button>
                }
                tone="error"
              />
            )}
            {sessionReady &&
              activeExecution &&
              !results.isFetching &&
              !results.isError &&
              !expired &&
              entries.length === 0 && (
                <State
                  icon={<Search />}
                  title="No matching entries"
                  detail="Try widening the time window or removing a filter."
                />
              )}
            {sessionReady && entries.length > 0 && (
              <div
                style={{
                  height: rowVirtualizer.getTotalSize(),
                  position: "relative",
                }}
              >
                {rowVirtualizer.getVirtualItems().map((virtualRow) => {
                  const entry = entries[virtualRow.index];
                  if (!entry) return null;
                  return (
                    <button
                      data-index={virtualRow.index}
                      ref={rowVirtualizer.measureElement}
                      key={entry.id}
                      className={`log-row ${selected?.id === entry.id ? "selected" : ""} ${prefs.display} ${pins.length ? "with-pins" : ""}`}
                      style={{
                        transform: `translateY(${virtualRow.start}px)`,
                      }}
                      onClick={() => {
                        setSelected(entry);
                        setInspectorTab("overview");
                        setContextSelected(null);
                      }}
                    >
                      <span className="row-time">
                        {formatTime(entry.timestamp, prefs.timezone)}
                      </span>
                      <span
                        className="severity-dot"
                        data-severity={entry.severity}
                      >
                        {entry.severity.slice(0, 4)}
                      </span>
                      <span className="row-source">{entry.source}</span>
                      <span className="row-message">
                        {prefs.display === "compact"
                          ? entry.message
                          : prefs.display === "structured"
                            ? JSON.stringify(entry.structured)
                            : JSON.stringify(entry.raw)}
                      </span>
                      {pins.length > 0 && (
                        <span className="pinned-values">
                          {pins.map((path) => (
                            <span
                              key={path}
                              title={`${path}: ${compactValue(valueAtPath(entry.structured, path))}`}
                            >
                              <b>{path.split(".").at(-1)}</b>
                              {compactValue(
                                valueAtPath(entry.structured, path),
                              )}
                            </span>
                          ))}
                        </span>
                      )}
                      <ChevronRight size={13} className="row-chevron" />
                    </button>
                  );
                })}
              </div>
            )}
            {results.hasNextPage && entries.length > 0 && (
              <button
                className="load-more"
                disabled={results.isFetchingNextPage}
                onClick={() => results.fetchNextPage()}
              >
                {results.isFetchingNextPage ? "Loading…" : "Load more"}
              </button>
            )}
          </div>
        </section>
      </main>

      {selected && (
        <aside className="inspector">
          <div className="inspector-head">
            <div>
              <span>ENTRY</span>
              <strong>{selected.id}</strong>
            </div>
            <div>
              <button
                className="icon-button"
                disabled={currentIndex <= 0}
                onClick={() =>
                  setSelected(entries[currentIndex - 1] ?? selected)
                }
              >
                ↑
              </button>
              <button
                className="icon-button"
                disabled={currentIndex >= entries.length - 1}
                onClick={() =>
                  setSelected(entries[currentIndex + 1] ?? selected)
                }
              >
                ↓
              </button>
              <button
                className="icon-button"
                onClick={() => setSelected(null)}
                aria-label="Close inspector"
              >
                <PanelRightClose size={15} />
              </button>
            </div>
          </div>
          <Tabs.Root
            value={inspectorTab}
            onValueChange={(value) => {
              setInspectorTab(value);
              setContextSelected(null);
            }}
            className="inspector-tabs"
          >
            <Tabs.List>
              {["overview", "structured", "fields", "raw", "context"].map(
                (tab) => (
                  <Tabs.Trigger key={tab} value={tab}>
                    {capitalize(tab)}
                  </Tabs.Trigger>
                ),
              )}
            </Tabs.List>
            <Tabs.Content value="overview">
              <dl className="overview-grid">
                <dt>Timestamp</dt>
                <dd>{new Date(selected.timestamp).toISOString()}</dd>
                <dt>Severity</dt>
                <dd>
                  <span
                    className="severity-badge"
                    data-severity={selected.severity}
                  >
                    {selected.severity}
                  </span>
                </dd>
                <dt>Source</dt>
                <dd>{selected.source}</dd>
                <dt>Message</dt>
                <dd className="message-detail">{selected.message}</dd>
              </dl>
              <h3>Labels</h3>
              <JsonText value={selected.labels} label="labels" />
            </Tabs.Content>
            <Tabs.Content value="structured">
              <JsonText
                value={selected.structured}
                label="structured payload"
              />
            </Tabs.Content>
            <Tabs.Content value="fields">
              <FieldBrowser
                fields={fields}
                pins={pins}
                onTogglePin={togglePin}
                onAdd={addPredicate}
              />
            </Tabs.Content>
            <Tabs.Content value="raw">
              <JsonText value={selected.raw} label="raw payload" />
            </Tabs.Content>
            <Tabs.Content value="context">
              <ContextView
                selected={selected}
                entries={context.data?.entries ?? []}
                active={contextSelected}
                loading={context.isLoading}
                error={context.error}
                onSelect={setContextSelected}
                timezone={prefs.timezone}
              />
            </Tabs.Content>
          </Tabs.Root>
        </aside>
      )}
      {profileOpen && (
        <ProfileDialog
          open
          onOpenChange={setProfileOpen}
          {...(editingProfile ? { profile: editingProfile } : {})}
          onSave={async (input, id) => {
            await saveProfile.mutateAsync({
              input,
              ...(id ? { id } : {}),
            });
          }}
        />
      )}
      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        commands={commands}
        returnFocusRef={paletteTriggerRef}
      />
    </div>
  );
}

function ContextView({
  selected,
  entries,
  active,
  loading,
  error,
  onSelect,
  timezone,
}: {
  selected: LogEntry;
  entries: LogEntry[];
  active: LogEntry | null;
  loading: boolean;
  error: Error | null;
  onSelect: (entry: LogEntry) => void;
  timezone: Preferences["timezone"];
}) {
  if (!selected.requestId && !selected.trace)
    return (
      <State
        icon={<Search />}
        title="No request identity"
        detail="This entry has no normalized request ID or trace ID, so context cannot be requested."
      />
    );
  if (loading)
    return (
      <State
        icon={<RotateCw />}
        title="Loading request context"
        detail="Searching the project-wide ±15 minute window without active query filters."
      />
    );
  if (error)
    return (
      <State
        icon={<AlertCircle />}
        title="Context failed"
        detail={error.message}
        tone="error"
      />
    );
  if (!entries.length)
    return (
      <State
        icon={<Search />}
        title="No request context"
        detail="No matching entries were found across the project-wide ±15 minute window."
      />
    );
  return (
    <div className="context-view">
      <p>Project-wide ±15m · active source and query filters are not applied</p>
      <div className="context-list">
        {entries.map((entry) => (
          <button
            className={active?.id === entry.id ? "active" : ""}
            key={entry.id}
            onClick={() => onSelect(entry)}
          >
            <time>{formatTime(entry.timestamp, timezone)}</time>
            <span data-severity={entry.severity}>{entry.severity}</span>
            <strong>{entry.source}</strong>
            <small>{entry.message}</small>
          </button>
        ))}
      </div>
      {active && (
        <div className="context-detail">
          <strong>{active.id}</strong>
          <p>{active.message}</p>
          <JsonText
            value={active.structured ?? active.raw}
            label="context entry"
          />
        </div>
      )}
    </div>
  );
}

function State({
  icon,
  title,
  detail,
  action,
  tone,
}: {
  icon: React.ReactNode;
  title: string;
  detail: string;
  action?: React.ReactNode;
  tone?: string;
}) {
  return (
    <div className={`result-state ${tone ?? ""}`}>
      <span>{icon}</span>
      <strong>{title}</strong>
      <p>{detail}</p>
      {action}
    </div>
  );
}

export default App;
