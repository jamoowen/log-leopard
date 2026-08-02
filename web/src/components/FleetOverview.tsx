import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle, ArrowUpRight, Layers3, Search, X } from "lucide-react";
import { api } from "../api/client";
import { ApiError, type Profile, type Source } from "../api/types";
import { loadHealthPreferences, type HealthWindow } from "../preferences";
import { formatCount, formatMetricValue } from "./metricFormat";
import { MetricRefreshControl } from "./MetricRefreshControl";
import "./fleet-overview.css";

const maxFleetServices = 20;
const windows: { id: HealthWindow; label: string; milliseconds: number }[] = [
  { id: "1h", label: "1h", milliseconds: 60 * 60_000 },
  { id: "6h", label: "6h", milliseconds: 6 * 60 * 60_000 },
  { id: "24h", label: "24h", milliseconds: 24 * 60 * 60_000 },
  { id: "7d", label: "7d", milliseconds: 7 * 24 * 60 * 60_000 },
];

function metricWindow(preset: HealthWindow) {
  const end = new Date();
  end.setSeconds(0, 0);
  const duration = windows.find((item) => item.id === preset)!.milliseconds;
  return {
    start: new Date(end.getTime() - duration).toISOString(),
    end: end.toISOString(),
  };
}

interface Props {
  profile: Profile | undefined;
  sources: Source[];
  discoveryPending: boolean;
  discoveryError?: string | undefined;
  discoveryWarning?: string | undefined;
  sessionReady: boolean;
  onOpenService: (service: string, window: HealthWindow) => void;
}

export function FleetOverview({
  profile,
  sources,
  discoveryPending,
  discoveryError,
  discoveryWarning,
  sessionReady,
  onOpenService,
}: Props) {
  const [initial] = useState(() => loadHealthPreferences(profile?.id ?? ""));
  const [windowPreset, setWindowPreset] = useState(initial.window);
  const [windowRevision, setWindowRevision] = useState(0);
  const [serviceFilter, setServiceFilter] = useState("");
  const discovered = Array.from(
    new Map(
      sources
        .filter((source) => source.kind === "cloud-run" && source.id.trim())
        .map((source) => [source.id, source]),
    ).values(),
  ).sort((a, b) => a.id.localeCompare(b.id));
  const fleetSources = discovered.slice(0, maxFleetServices);
  const serviceNames = fleetSources.map((source) => source.id);
  const fleet = useQuery({
    queryKey: [
      "fleet-overview",
      profile?.id,
      serviceNames,
      windowPreset,
      windowRevision,
    ],
    queryFn: ({ signal }) => {
      const window = metricWindow(windowPreset);
      return api.fleetOverview(
        {
          profileId: profile!.id,
          services: serviceNames,
          start: window.start,
          end: window.end,
        },
        signal,
      );
    },
    enabled: sessionReady && Boolean(profile && serviceNames.length),
  });
  const rows = [...(fleet.data?.services ?? [])].sort(
    (a, b) =>
      b.serverErrorCount - a.serverErrorCount ||
      b.requestCount - a.requestCount ||
      a.service.localeCompare(b.service),
  );
  const normalizedFilter = serviceFilter.trim().toLowerCase();
  const filteredRows = normalizedFilter
    ? rows.filter((row) => row.service.toLowerCase().includes(normalizedFilter))
    : rows;
  const totalRequests = rows.reduce((sum, row) => sum + row.requestCount, 0);
  const totalErrors = rows.reduce((sum, row) => sum + row.serverErrorCount, 0);
  const activeServices = rows.filter((row) => row.requestCount > 0).length;

  function selectWindow(preset: HealthWindow) {
    setWindowPreset(preset);
    setWindowRevision((revision) => revision + 1);
  }

  function refreshMetrics() {
    setWindowRevision((revision) => revision + 1);
  }

  if (!profile) {
    return (
      <main className="fleet-empty">
        <Layers3 size={22} />
        <h1>Connect a project to view its Cloud Run fleet</h1>
        <p>Metrics remain in Cloud Monitoring and are queried read-only.</p>
      </main>
    );
  }

  return (
    <main className="fleet-root">
      <header className="fleet-controls">
        <div className="fleet-title">
          <span>FLEET OVERVIEW</span>
          <strong>{profile.name}</strong>
        </div>
        <span className="fleet-delay">
          <i /> METRICS DELAYED ~2M
        </span>
        <MetricRefreshControl
          updatedAt={fleet.dataUpdatedAt}
          refreshing={fleet.isFetching}
          disabled={!serviceNames.length}
          onRefresh={refreshMetrics}
        />
        <div className="fleet-window" role="group" aria-label="Fleet window">
          {windows.map((item) => (
            <button
              key={item.id}
              type="button"
              className={windowPreset === item.id ? "active" : ""}
              aria-pressed={windowPreset === item.id}
              onClick={() => selectWindow(item.id)}
            >
              {item.label}
            </button>
          ))}
        </div>
      </header>

      {discoveryPending ? (
        <FleetMessage>Discovering Cloud Run services...</FleetMessage>
      ) : discoveryError ? (
        <FleetError
          title="Service discovery unavailable"
          detail={discoveryError}
        />
      ) : discoveryWarning && discovered.length === 0 ? (
        <FleetError
          title="Cloud Run discovery unavailable"
          detail={discoveryWarning}
        />
      ) : discovered.length === 0 ? (
        <FleetMessage>No Cloud Run services were discovered.</FleetMessage>
      ) : fleet.isPending ? (
        <FleetMessage>Loading fleet metrics...</FleetMessage>
      ) : fleet.isError ? (
        <FleetError
          title={
            fleet.error instanceof ApiError && fleet.error.status === 403
              ? "Metrics access required"
              : fleet.error instanceof ApiError && fleet.error.status === 424
                ? "Cloud Monitoring unavailable"
                : "Fleet overview unavailable"
          }
          detail={fleet.error.message}
        />
      ) : (
        <>
          {(discoveryWarning || discovered.length > maxFleetServices) && (
            <div className="fleet-warning">
              <AlertTriangle size={14} />
              <span>
                {discoveryWarning ??
                  `Showing the first ${maxFleetServices} of ${discovered.length} discovered services.`}
              </span>
            </div>
          )}
          <section className="fleet-summary" aria-label="Fleet summary">
            <div>
              <span>DISCOVERED</span>
              <strong>{fleetSources.length}</strong>
            </div>
            <div>
              <span>WITH TRAFFIC</span>
              <strong>{activeServices}</strong>
            </div>
            <div>
              <span>OBSERVED REQUESTS</span>
              <strong>{formatCount(totalRequests)}</strong>
            </div>
            <div>
              <span>OBSERVED 5XX</span>
              <strong className={totalErrors ? "warning" : ""}>
                {formatCount(totalErrors)}
              </strong>
            </div>
          </section>
          <section className="fleet-table-wrap">
            <div className="fleet-table-toolbar">
              <div>
                <span>SERVICES</span>
                <strong>
                  {filteredRows.length} OF {rows.length}
                </strong>
              </div>
              <div className="fleet-filter">
                <Search size={13} aria-hidden="true" />
                <input
                  type="search"
                  aria-label="Filter fleet services"
                  placeholder="Filter services"
                  value={serviceFilter}
                  onChange={(event) => setServiceFilter(event.target.value)}
                />
                {serviceFilter && (
                  <button
                    type="button"
                    aria-label="Clear service filter"
                    onClick={() => setServiceFilter("")}
                  >
                    <X size={12} />
                  </button>
                )}
              </div>
            </div>
            <div
              className="fleet-table"
              role="table"
              aria-label={`Cloud Run fleet metrics for the last ${windowPreset}`}
            >
              <div className="fleet-row fleet-head" role="row">
                <span role="columnheader">Service</span>
                <span role="columnheader">Requests</span>
                <span role="columnheader">5xx</span>
                <span role="columnheader">5xx rate</span>
                <span role="columnheader">p95 latency</span>
              </div>
              {filteredRows.map((row) => {
                const errorRate = row.requestCount
                  ? (row.serverErrorCount / row.requestCount) * 100
                  : 0;
                return (
                  <div className="fleet-row" role="row" key={row.service}>
                    <span role="cell" data-label="Service">
                      <button
                        type="button"
                        onClick={() => onOpenService(row.service, windowPreset)}
                      >
                        <span>{row.service}</span>
                        <ArrowUpRight size={13} />
                      </button>
                    </span>
                    <span role="cell" data-label="Requests">
                      {formatCount(row.requestCount)}
                    </span>
                    <span
                      role="cell"
                      data-label="5xx"
                      className={row.serverErrorCount ? "warning" : ""}
                    >
                      {formatCount(row.serverErrorCount)}
                    </span>
                    <span
                      role="cell"
                      data-label="5xx rate"
                      className={errorRate >= 1 ? "warning" : ""}
                    >
                      {errorRate.toFixed(2)}%
                    </span>
                    <span role="cell" data-label="p95 latency">
                      {row.requestLatencyP95Ms === null
                        ? "Unavailable"
                        : formatMetricValue(row.requestLatencyP95Ms, "ms")}
                    </span>
                  </div>
                );
              })}
            </div>
            {normalizedFilter && filteredRows.length === 0 && (
              <p className="fleet-no-match" role="status">
                No services match “{serviceFilter.trim()}”.
              </p>
            )}
            {totalRequests === 0 && (
              <p className="fleet-quiet">
                No traffic was observed for these services in the selected
                window.
              </p>
            )}
          </section>
          <footer className="fleet-footnote">
            Fixed Cloud Run metrics across matching revisions. Select a service
            for its full timeline and log drilldown.
          </footer>
        </>
      )}
    </main>
  );
}

function FleetMessage({ children }: { children: string }) {
  return (
    <section className="fleet-message" role="status" aria-live="polite">
      {children}
    </section>
  );
}

function FleetError({ title, detail }: { title: string; detail: string }) {
  return (
    <section className="fleet-message error" role="alert">
      <AlertTriangle size={17} />
      <div>
        <strong>{title}</strong>
        <span>{detail}</span>
      </div>
    </section>
  );
}
