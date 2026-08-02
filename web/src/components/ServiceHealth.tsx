import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Clock3,
  Search,
} from "lucide-react";
import { api } from "../api/client";
import type {
  HealthPoint,
  Profile,
  ServiceHealthResponse,
  Source,
} from "../api/types";
import { ApiError } from "../api/types";
import { HealthChart } from "./HealthChart";
import { MetricRefreshControl } from "./MetricRefreshControl";
import { formatCount, formatMetricValue } from "./metricFormat";
import {
  loadHealthPreferences,
  saveHealthPreferences,
  type HealthWindow,
} from "../preferences";
import "./service-health.css";

interface Props {
  profile: Profile | undefined;
  sources: Source[];
  discoveryPending: boolean;
  discoveryError?: string | undefined;
  discoveryWarning?: string | undefined;
  sessionReady: boolean;
  onOpenLogs: (service: string, start: string, end: string) => void;
}

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

function healthSeriesPoints(
  data: ServiceHealthResponse | undefined,
  name: "request_count" | "server_error_count" | "request_latency_p95",
): HealthPoint[] {
  return data?.series?.find((item) => item.name === name)?.points ?? [];
}

function formatClock(timestamp: string) {
  return new Intl.DateTimeFormat("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(timestamp));
}

export function ServiceHealth({
  profile,
  sources,
  discoveryPending,
  discoveryError,
  discoveryWarning,
  sessionReady,
  onOpenLogs,
}: Props) {
  const [initial] = useState(() => loadHealthPreferences(profile?.id ?? ""));
  const [selectedService, setSelectedService] = useState(initial.target);
  const [windowPreset, setWindowPreset] = useState(initial.window);
  const [windowRevision, setWindowRevision] = useState(0);
  const [view, setView] = useState<"workbench" | "timeline">("workbench");
  const [selectedIntervalEnd, setSelectedIntervalEnd] = useState("");
  const available = sources.filter((source) => source.id.trim());
  const service = available.some((source) => source.id === selectedService)
    ? selectedService
    : "";
  function selectService(target: string) {
    setSelectedService(target);
    setWindowRevision((revision) => revision + 1);
    setView("workbench");
    setSelectedIntervalEnd("");
    saveHealthPreferences(profile!.id, { target, window: windowPreset });
  }
  function selectWindow(preset: HealthWindow) {
    setWindowPreset(preset);
    setWindowRevision((revision) => revision + 1);
    setView("workbench");
    setSelectedIntervalEnd("");
    saveHealthPreferences(profile!.id, { target: service, window: preset });
  }
  function refreshMetrics() {
    setWindowRevision((revision) => revision + 1);
    setView("workbench");
    setSelectedIntervalEnd("");
  }
  const health = useQuery({
    queryKey: [
      "service-health",
      profile?.id,
      service,
      windowPreset,
      windowRevision,
    ],
    queryFn: ({ signal }) => {
      const window = metricWindow(windowPreset);
      return api.serviceHealth(
        {
          profileId: profile!.id,
          service,
          start: window.start,
          end: window.end,
        },
        signal,
      );
    },
    enabled: sessionReady && Boolean(profile && service),
  });
  const requests = healthSeriesPoints(health.data, "request_count");
  const errors = healthSeriesPoints(health.data, "server_error_count");
  const latencyP95 = healthSeriesPoints(health.data, "request_latency_p95");
  const totalRequests = requests.reduce((sum, point) => sum + point.value, 0);
  const totalErrors = errors.reduce((sum, point) => sum + point.value, 0);
  const peakLatencyP95 = latencyP95.length
    ? Math.max(...latencyP95.map((point) => point.value))
    : undefined;
  const errorRate = totalRequests ? (totalErrors / totalRequests) * 100 : 0;
  const highestError = errors.reduce<HealthPoint | undefined>(
    (highest, point) =>
      !highest || point.value > highest.value ? point : highest,
    undefined,
  );
  const intervalEnd = requests.some(
    (point) => point.timestamp === selectedIntervalEnd,
  )
    ? selectedIntervalEnd
    : (highestError?.timestamp ??
      health.data?.end ??
      metricWindow(windowPreset).end);
  const intervalStart = new Date(
    new Date(intervalEnd).getTime() -
      (health.data?.alignmentSeconds ?? 60) * 1000,
  ).toISOString();
  const intervalError =
    errors.find((point) => point.timestamp === intervalEnd)?.value ?? 0;
  const intervalLatencyP95 = latencyP95.find(
    (point) => point.timestamp === intervalEnd,
  )?.value;

  if (!profile) {
    return (
      <main className="service-health-empty">
        <Activity size={22} />
        <h1>Connect a project to view service health</h1>
        <p>Metrics remain in Cloud Monitoring and are queried read-only.</p>
      </main>
    );
  }
  return (
    <main className="service-health-root">
      <header className="service-health-controls">
        <div className="service-health-target">
          <span>SERVICE</span>
          <select
            aria-label="Health service"
            value={service}
            onChange={(event) => selectService(event.target.value)}
          >
            <option value="">Choose a service</option>
            {available.map((source) => (
              <option key={source.id} value={source.id}>
                {source.label}
              </option>
            ))}
          </select>
        </div>
        <span className="service-health-delay">
          <i /> METRICS DELAYED ~2M
        </span>
        <MetricRefreshControl
          updatedAt={health.dataUpdatedAt}
          refreshing={health.isFetching}
          disabled={!service}
          onRefresh={refreshMetrics}
        />
        <div
          className="service-health-window"
          role="group"
          aria-label="Health window"
        >
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
        <section className="service-health-message">
          Discovering Cloud Run services…
        </section>
      ) : discoveryError ? (
        <section className="service-health-message error">
          <AlertTriangle size={17} />
          <div>
            <strong>Service discovery unavailable</strong>
            <span>{discoveryError}</span>
          </div>
        </section>
      ) : discoveryWarning && available.length === 0 ? (
        <section className="service-health-message error">
          <AlertTriangle size={17} />
          <div>
            <strong>Cloud Run discovery unavailable</strong>
            <span>{discoveryWarning}</span>
          </div>
        </section>
      ) : available.length === 0 ? (
        <section className="service-health-message">
          <strong>No Cloud Run services discovered</strong>
          <span>
            This connection can still use Logs, but it has no supported health
            targets.
          </span>
        </section>
      ) : !service ? (
        <section className="service-health-message">
          <strong>Choose a Cloud Run service</strong>
          <span>
            Health targets are explicit so a quiet worker is not mistaken for
            the whole project.
          </span>
        </section>
      ) : health.isPending ? (
        <section className="service-health-message">
          Loading service health…
        </section>
      ) : health.isError ? (
        <section className="service-health-message error">
          <AlertTriangle size={17} />
          <div>
            <strong>
              {health.error instanceof ApiError && health.error.status === 403
                ? "Metrics access required"
                : health.error instanceof ApiError &&
                    health.error.status === 424
                  ? "Cloud Monitoring unavailable"
                  : "Service health unavailable"}
            </strong>
            <span>{health.error.message}</span>
          </div>
        </section>
      ) : requests.length === 0 ? (
        <section className="service-health-message">
          <strong>No traffic observed</strong>
          <span>
            {service} returned no request points in the last {windowPreset}.
            This usually means the service was quiet; recent data can take about
            two minutes to appear.
          </span>
          <div className="service-health-message-actions">
            {windowPreset !== "24h" && windowPreset !== "7d" && (
              <button type="button" onClick={() => selectWindow("24h")}>
                Try 24h
              </button>
            )}
            <button type="button" onClick={() => selectService("")}>
              Choose another service
            </button>
          </div>
        </section>
      ) : view === "workbench" ? (
        <>
          <section className="service-health-summary">
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
            <div>
              <span>5XX RATE</span>
              <strong className={errorRate >= 1 ? "warning" : ""}>
                {errorRate.toFixed(2)}%
              </strong>
            </div>
            <div>
              <span>PEAK P95 LATENCY</span>
              <strong>
                {peakLatencyP95 === undefined
                  ? "Unavailable"
                  : formatMetricValue(peakLatencyP95, "ms")}
              </strong>
            </div>
          </section>
          <section className="service-health-workbench">
            <article>
              <header>
                <div>
                  <span>REQUEST HEALTH</span>
                  <h1>Traffic and container-reaching 5xx responses</h1>
                </div>
                <div className="service-health-legend">
                  <span>Requests</span>
                  <span>5xx</span>
                </div>
              </header>
              <HealthChart
                requests={requests}
                errors={errors}
                start={health.data.start}
                end={health.data.end}
                alignmentSeconds={health.data.alignmentSeconds}
                {...(totalErrors
                  ? {
                      selectionFallback: highestError!.timestamp,
                      onSelect: (timestamp: string) => {
                        setSelectedIntervalEnd(timestamp);
                        setView("timeline");
                      },
                    }
                  : {})}
              />
              {latencyP95.length > 0 && (
                <div className="service-health-latency-track">
                  <div>
                    <span>REQUEST LATENCY</span>
                    <strong>P95 container time</strong>
                  </div>
                  <HealthChart
                    requests={latencyP95}
                    errors={[]}
                    start={health.data.start}
                    end={health.data.end}
                    alignmentSeconds={health.data.alignmentSeconds}
                    showErrors={false}
                    primaryLabel="P95 latency"
                    primaryTone="latency"
                    valueUnit="ms"
                  />
                </div>
              )}
              <footer>
                <AlertTriangle size={14} />
                <span>
                  {totalErrors
                    ? `Highest observed error bucket: ${highestError?.value ?? 0} 5xx responses at ${formatClock(highestError!.timestamp)}.`
                    : "No 5xx responses were observed in this window."}
                </span>
                {totalErrors > 0 && (
                  <button
                    type="button"
                    onClick={() => {
                      setSelectedIntervalEnd(highestError!.timestamp);
                      setView("timeline");
                    }}
                  >
                    Inspect timeline
                  </button>
                )}
              </footer>
            </article>
            <aside>
              <span>WHAT THIS VIEW KNOWS</span>
              <h2>Observed request outcomes</h2>
              <p>
                Counts include requests that reached a Cloud Run revision. Some
                edge rejections and max-instance failures are not included.
              </p>
              <dl>
                <div>
                  <dt>Source</dt>
                  <dd>Cloud Monitoring</dd>
                </div>
                <div>
                  <dt>Scope</dt>
                  <dd>All matching revisions</dd>
                </div>
                <div>
                  <dt>Persistence</dt>
                  <dd>Metric data: none</dd>
                </div>
              </dl>
              <small>
                Target and window preferences are stored in this browser. Metric
                data is not persisted. <br />
                P95 latency merges request distributions across revisions before
                percentile conversion. CPU, memory, and instance metrics remain
                intentionally deferred.
              </small>
            </aside>
          </section>
        </>
      ) : (
        <section className="service-health-timeline">
          <header>
            <button type="button" onClick={() => setView("workbench")}>
              <ArrowLeft size={14} /> Workbench
            </button>
            <div>
              <span>SELECTED METRIC INTERVAL</span>
              <h1>
                {formatClock(intervalStart)}–{formatClock(intervalEnd)}
              </h1>
            </div>
          </header>
          <div className="service-health-tracks">
            <div>
              <span>REQUEST COUNT</span>
              <HealthChart
                requests={requests}
                errors={[]}
                start={health.data.start}
                end={health.data.end}
                alignmentSeconds={health.data.alignmentSeconds}
                showErrors={false}
              />
            </div>
            <div>
              <span>SERVER ERRORS</span>
              <HealthChart
                requests={errors}
                errors={[]}
                start={health.data.start}
                end={health.data.end}
                alignmentSeconds={health.data.alignmentSeconds}
                showErrors={false}
                primaryLabel="5xx"
                primaryTone="error"
              />
            </div>
            {latencyP95.length > 0 && (
              <div>
                <span>P95 REQUEST LATENCY</span>
                <HealthChart
                  requests={latencyP95}
                  errors={[]}
                  start={health.data.start}
                  end={health.data.end}
                  alignmentSeconds={health.data.alignmentSeconds}
                  showErrors={false}
                  primaryLabel="P95 latency"
                  primaryTone="latency"
                  valueUnit="ms"
                />
              </div>
            )}
          </div>
          <aside>
            <Clock3 size={16} />
            <h2>
              {intervalError} observed 5xx ·{" "}
              {intervalLatencyP95 === undefined
                ? "p95 unavailable"
                : `${formatMetricValue(intervalLatencyP95, "ms")} p95`}
            </h2>
            <p>
              Open the exact service and aligned interval in Logs to inspect
              request entries that reached the container.
            </p>
            <button
              type="button"
              onClick={() => onOpenLogs(service, intervalStart, intervalEnd)}
            >
              <Search size={14} /> Open interval in logs
            </button>
          </aside>
        </section>
      )}
    </main>
  );
}
