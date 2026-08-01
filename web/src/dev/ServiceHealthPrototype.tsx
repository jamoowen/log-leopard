// PROTOTYPE: three service-health dashboard directions, selected with ?service-health=.
import { useEffect, useState, type ReactNode } from "react";
import {
  Activity,
  AlertTriangle,
  ChevronLeft,
  ChevronRight,
  Clock3,
  Cpu,
  Gauge,
  MemoryStick,
  Search,
  Server,
  TerminalSquare,
  Zap,
} from "lucide-react";
import "./service-health.css";

const variants = ["workbench", "matrix", "timeline"] as const;
type Variant = (typeof variants)[number];

const variantNames: Record<Variant, string> = {
  workbench: "Diagnostic workbench",
  matrix: "Signal matrix",
  timeline: "Incident timeline",
};

const traffic = [
  18, 22, 26, 31, 29, 34, 42, 49, 44, 57, 72, 91, 84, 77, 68, 61, 56, 52, 48,
  45,
];
const errors = [1, 0, 1, 1, 0, 1, 2, 2, 1, 3, 4, 13, 9, 5, 3, 2, 1, 1, 0, 1];
const latency = [
  186, 178, 190, 201, 198, 208, 216, 230, 221, 248, 276, 482, 612, 428, 311,
  270, 238, 224, 212, 204,
];
const cpu = [
  31, 33, 35, 38, 36, 42, 46, 51, 48, 57, 63, 86, 79, 66, 59, 52, 48, 45, 42,
  40,
];
const memory = [
  42, 43, 43, 44, 45, 46, 47, 48, 49, 51, 54, 58, 59, 57, 55, 54, 53, 52, 51,
  50,
];
const instances = [2, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 7, 8, 7, 6, 5, 4, 4, 3, 3];

const signalRows = [
  {
    signal: "End-to-end latency",
    current: "204 ms",
    peak: "612 ms",
    baseline: "188 ms",
    state: "recovering",
    values: latency,
  },
  {
    signal: "5xx rate",
    current: "0.4%",
    peak: "14.3%",
    baseline: "0.2%",
    state: "recovering",
    values: errors,
  },
  {
    signal: "Request rate",
    current: "45 rps",
    peak: "91 rps",
    baseline: "34 rps",
    state: "healthy",
    values: traffic,
  },
  {
    signal: "CPU p95",
    current: "40%",
    peak: "86%",
    baseline: "35%",
    state: "healthy",
    values: cpu,
  },
  {
    signal: "Memory p95",
    current: "50%",
    peak: "59%",
    baseline: "44%",
    state: "healthy",
    values: memory,
  },
  {
    signal: "Active instances",
    current: "3",
    peak: "8",
    baseline: "2",
    state: "healthy",
    values: instances,
  },
] as const;

function MiniLine({
  values,
  tone = "accent",
}: {
  values: readonly number[];
  tone?: "accent" | "error" | "warning";
}) {
  const max = Math.max(...values);
  const min = Math.min(...values);
  const span = max - min || 1;
  const points = values
    .map(
      (value, index) =>
        `${(index / (values.length - 1)) * 100},${30 - ((value - min) / span) * 26}`,
    )
    .join(" ");
  return (
    <svg
      className={`health-spark health-spark-${tone}`}
      viewBox="0 0 100 32"
      preserveAspectRatio="none"
      aria-hidden="true"
    >
      <polyline points={points} fill="none" vectorEffect="non-scaling-stroke" />
    </svg>
  );
}

function MainChart({ onInspect }: { onInspect: () => void }) {
  const maxTraffic = Math.max(...traffic);
  const maxLatency = Math.max(...latency);
  const latencyPoints = latency
    .map(
      (value, index) =>
        `${22 + index * 38},${176 - (value / maxLatency) * 126}`,
    )
    .join(" ");
  return (
    <button
      className="health-main-chart"
      type="button"
      onClick={onInspect}
      aria-label="Inspect the incident interval in logs"
    >
      <svg
        viewBox="0 0 790 205"
        preserveAspectRatio="none"
        role="img"
        aria-label="Traffic and latency over the past hour"
      >
        {[0, 1, 2, 3].map((line) => (
          <line
            key={line}
            x1="22"
            x2="770"
            y1={38 + line * 42}
            y2={38 + line * 42}
            className="health-gridline"
          />
        ))}
        {traffic.map((value, index) => (
          <rect
            key={index}
            x={15 + index * 38}
            y={183 - (value / maxTraffic) * 98}
            width="20"
            height={(value / maxTraffic) * 98}
            className={
              index >= 11 && index <= 13 ? "health-bar-hot" : "health-bar"
            }
          />
        ))}
        <polyline
          points={latencyPoints}
          className="health-latency-line"
          fill="none"
        />
        <line
          x1="439"
          x2="439"
          y1="18"
          y2="188"
          className="health-incident-line"
        />
        <circle cx="439" cy="50" r="4" className="health-incident-dot" />
      </svg>
      <span className="health-chart-axis">
        <span>10:00</span>
        <span>10:15</span>
        <span>10:30</span>
        <span>10:45</span>
        <span>Now</span>
      </span>
    </button>
  );
}

function Shell({ children }: { children: ReactNode }) {
  return (
    <div className="health-shell">
      <header className="health-topbar">
        <div className="brand">
          <img className="brand-mark" src="/log-leopard.png" alt="" />
          <strong>LOGLEOPARD</strong>
          <span className="edition">SERVICE HEALTH / PROTOTYPE</span>
        </div>
        <div className="health-context">
          <span className="health-live">
            <i /> METRICS DELAYED ~2M
          </span>
          <button type="button">
            <span>rome-app-stg</span>
            <small>europe-west1</small>
          </button>
          <button type="button">Last 60 minutes</button>
        </div>
      </header>
      <nav className="health-subnav">
        <div>
          <button type="button">Logs</button>
          <button type="button" className="active">
            Service health
          </button>
        </div>
        <div className="health-service">
          <span>Service</span>
          <strong>api</strong>
          <span>Revision</span>
          <strong>api-00241-q7f</strong>
        </div>
      </nav>
      {children}
    </div>
  );
}

function SummaryStrip() {
  return (
    <section className="health-summary">
      <div>
        <span>REQUESTS</span>
        <strong>184.2k</strong>
        <small>45 req/s now</small>
      </div>
      <div>
        <span>5XX RATE</span>
        <strong className="warning">0.4%</strong>
        <small>peak 14.3% at 10:31</small>
      </div>
      <div>
        <span>E2E P95</span>
        <strong>204 ms</strong>
        <small className="good">recovered from 612 ms</small>
      </div>
      <div>
        <span>ACTIVE INSTANCES</span>
        <strong>3</strong>
        <small>8 peak / 1 idle</small>
      </div>
      <div>
        <span>LATEST REVISION</span>
        <strong className="revision">00241-q7f</strong>
        <small>deployed 09:42</small>
      </div>
    </section>
  );
}

function Workbench({ inspect }: { inspect: (message: string) => void }) {
  return (
    <main className="health-workbench">
      <SummaryStrip />
      <section className="health-workbench-grid">
        <article className="health-panel health-traffic-panel">
          <header>
            <div>
              <span className="health-eyebrow">REQUEST HEALTH</span>
              <h2>Traffic recovered after a 5xx burst</h2>
            </div>
            <div className="health-legend">
              <span className="traffic">Requests</span>
              <span className="latency">E2E p95</span>
            </div>
          </header>
          <MainChart
            onInspect={() =>
              inspect("Logs scoped to api · 10:29–10:35 · HTTP 5xx")
            }
          />
          <footer>
            <AlertTriangle size={14} />
            <span>
              <strong>10:31</strong> 5xx rate reached 14.3% while p95 latency
              climbed to 612 ms.
            </span>
            <button
              type="button"
              onClick={() =>
                inspect(
                  "Logs scoped to revision api-00241-q7f around the anomaly",
                )
              }
            >
              Inspect 128 logs
            </button>
          </footer>
        </article>
        <aside className="health-panel health-diagnostics">
          <header>
            <span className="health-eyebrow">CAPACITY NOW</span>
            <strong>Healthy</strong>
          </header>
          <div>
            <Cpu size={15} />
            <span>CPU p95</span>
            <strong>40%</strong>
            <MiniLine values={cpu} />
          </div>
          <div>
            <MemoryStick size={15} />
            <span>Memory p95</span>
            <strong>50%</strong>
            <MiniLine values={memory} />
          </div>
          <div>
            <Server size={15} />
            <span>Instances</span>
            <strong>3 active</strong>
            <MiniLine values={instances} />
          </div>
          <div>
            <Zap size={15} />
            <span>Startup p95</span>
            <strong>1.24 s</strong>
            <MiniLine
              values={[12, 11, 13, 12, 14, 44, 36, 18, 14, 12]}
              tone="warning"
            />
          </div>
          <p>
            Autoscaler recommendation and actual capacity converged at 10:37.
          </p>
        </aside>
        <article className="health-panel health-errors">
          <header>
            <div>
              <span className="health-eyebrow">CORRELATED LOGS</span>
              <h2>Errors during the selected interval</h2>
            </div>
            <button
              type="button"
              onClick={() => inspect("Full log view · api · severity ERROR")}
            >
              Open log view <ChevronRight size={14} />
            </button>
          </header>
          <div className="health-log-row">
            <time>10:31:48.221</time>
            <b>500</b>
            <code>POST /v1/photos</code>
            <span>database connection pool exhausted</span>
            <em>req_93d7</em>
          </div>
          <div className="health-log-row">
            <time>10:31:42.805</time>
            <b>503</b>
            <code>GET /v1/feed</code>
            <span>upstream request deadline exceeded</span>
            <em>req_b13a</em>
          </div>
          <div className="health-log-row">
            <time>10:30:59.118</time>
            <b>500</b>
            <code>POST /v1/photos</code>
            <span>database connection pool exhausted</span>
            <em>req_782c</em>
          </div>
        </article>
      </section>
    </main>
  );
}

function Matrix({ inspect }: { inspect: (message: string) => void }) {
  return (
    <main className="health-matrix-page">
      <header className="health-matrix-head">
        <div>
          <span className="health-eyebrow">SERVICE SIGNAL MATRIX</span>
          <h1>
            api <small>recovered, watch latency</small>
          </h1>
        </div>
        <div className="health-score">
          <strong>92</strong>
          <span>health score</span>
        </div>
      </header>
      <section className="health-matrix-layout">
        <article className="health-signal-table">
          <div className="health-matrix-row heading">
            <span>Signal</span>
            <span>Last 60 minutes</span>
            <span>Now</span>
            <span>Peak</span>
            <span>Baseline</span>
            <span>Status</span>
          </div>
          {signalRows.map((row, index) => (
            <button
              className="health-matrix-row"
              type="button"
              key={row.signal}
              onClick={() =>
                inspect(
                  `${row.signal} selected · click interval 10:29–10:35 to inspect logs`,
                )
              }
            >
              <span>
                <i className={index < 2 ? "recovering" : "healthy"} />
                {row.signal}
              </span>
              <MiniLine
                values={row.values}
                tone={
                  index === 1 ? "error" : index === 0 ? "warning" : "accent"
                }
              />
              <strong>{row.current}</strong>
              <span>{row.peak}</span>
              <span>{row.baseline}</span>
              <em className={row.state}>{row.state}</em>
            </button>
          ))}
        </article>
        <aside className="health-matrix-inspector">
          <span className="health-eyebrow">ANOMALY EXPLAINER</span>
          <div className="health-time-badge">
            <Clock3 size={15} />
            <strong>10:29–10:35</strong>
            <span>6 minute interval</span>
          </div>
          <h2>Latency and errors moved together</h2>
          <p>
            Request volume rose 64%, but CPU remained below saturation. Error
            logs point to database connection pressure rather than compute
            capacity.
          </p>
          <dl>
            <div>
              <dt>5xx requests</dt>
              <dd>128</dd>
            </div>
            <div>
              <dt>Affected revision</dt>
              <dd>00241-q7f</dd>
            </div>
            <div>
              <dt>Dominant message</dt>
              <dd>pool exhausted</dd>
            </div>
          </dl>
          <button
            type="button"
            onClick={() =>
              inspect("128 correlated logs · api-00241-q7f · 10:29–10:35")
            }
          >
            <Search size={14} /> Inspect correlated logs
          </button>
          <small>
            Inference is deterministic from metric overlap and log counts. No AI
            analysis.
          </small>
        </aside>
      </section>
      <footer className="health-matrix-footer">
        <span>
          <Activity size={14} /> Current state
        </span>
        <strong>Serving normally</strong>
        <span>3 active instances</span>
        <span>0.4% 5xx</span>
        <span>204 ms p95</span>
      </footer>
    </main>
  );
}

const timelineTracks = [
  { label: "DEPLOYMENTS", kind: "deploy", note: "api-00241-q7f · 09:42" },
  { label: "REQUESTS / SEC", kind: "bars", values: traffic },
  { label: "5XX RATE", kind: "line-error", values: errors },
  { label: "E2E P95", kind: "line-warning", values: latency },
  { label: "CPU P95", kind: "line", values: cpu },
  { label: "INSTANCES", kind: "steps", values: instances },
] as const;

function Timeline({ inspect }: { inspect: (message: string) => void }) {
  return (
    <main className="health-timeline-page">
      <header className="health-timeline-title">
        <div>
          <span className="health-eyebrow">INCIDENT TIMELINE</span>
          <h1>One hour of service behavior, aligned</h1>
        </div>
        <div>
          <b>10:29–10:35</b>
          <span>suspect interval</span>
        </div>
      </header>
      <section className="health-timeline-layout">
        <article className="health-tracks">
          <div className="health-timeline-axis">
            <span>10:00</span>
            <span>10:15</span>
            <span>10:30</span>
            <span>10:45</span>
            <span>11:00</span>
          </div>
          {timelineTracks.map((track) => (
            <button
              className={`health-track ${track.kind}`}
              key={track.label}
              type="button"
              onClick={() =>
                inspect(
                  `${track.label} · 10:29–10:35 selected for log correlation`,
                )
              }
            >
              <span>{track.label}</span>
              <div className="health-track-plot">
                {"values" in track && track.kind === "bars"
                  ? track.values.map((value, index) => (
                      <i
                        key={index}
                        style={{ height: `${Math.max(8, value)}%` }}
                      />
                    ))
                  : null}
                {"values" in track && track.kind !== "bars" ? (
                  <MiniLine
                    values={track.values}
                    tone={
                      track.kind === "line-error"
                        ? "error"
                        : track.kind === "line-warning"
                          ? "warning"
                          : "accent"
                    }
                  />
                ) : null}
                {track.kind === "deploy" ? (
                  <>
                    <i className="health-deploy-mark" />
                    <strong>{track.note}</strong>
                  </>
                ) : null}
                <i className="health-window" />
              </div>
              <strong>
                {track.kind === "line-error"
                  ? "0.4%"
                  : track.kind === "line-warning"
                    ? "204 ms"
                    : track.kind === "steps"
                      ? "3"
                      : ""}
              </strong>
            </button>
          ))}
          <div className="health-log-events">
            <span>LOG EVENTS</span>
            <button
              type="button"
              onClick={() => inspect("ERROR · pool exhausted · req_93d7")}
            >
              10:31:48 <b>ERROR</b> pool exhausted
            </button>
            <button
              type="button"
              onClick={() => inspect("WARNING · autoscaler added 4 instances")}
            >
              10:32:10 <em>WARN</em> autoscaler +4
            </button>
            <button
              type="button"
              onClick={() => inspect("INFO · error rate returned below 1%")}
            >
              10:36:02 <i>INFO</i> error rate normal
            </button>
          </div>
        </article>
        <aside className="health-incident-card">
          <span className="health-eyebrow">SELECTED WINDOW</span>
          <h2>10:29–10:35</h2>
          <p className="health-incident-status">
            <AlertTriangle size={16} /> Degraded, recovered
          </p>
          <div className="health-causal-step">
            <span>01</span>
            <div>
              <b>Traffic increased</b>
              <small>34 → 91 requests/sec</small>
            </div>
          </div>
          <div className="health-causal-step">
            <span>02</span>
            <div>
              <b>Database pool saturated</b>
              <small>96 matching error logs</small>
            </div>
          </div>
          <div className="health-causal-step">
            <span>03</span>
            <div>
              <b>Latency and 5xx spiked</b>
              <small>612 ms p95 · 14.3% 5xx</small>
            </div>
          </div>
          <div className="health-causal-step">
            <span>04</span>
            <div>
              <b>Capacity caught up</b>
              <small>2 → 8 active instances</small>
            </div>
          </div>
          <button
            type="button"
            onClick={() =>
              inspect(
                "Log query prepared for api · 10:29–10:35 · ERROR|WARNING",
              )
            }
          >
            <TerminalSquare size={14} /> Open this window in logs
          </button>
        </aside>
      </section>
    </main>
  );
}

function PrototypeSwitcher({ current }: { current: Variant }) {
  const move = (offset: number) => {
    const index = variants.indexOf(current);
    const next =
      variants[(index + offset + variants.length) % variants.length]!;
    const url = new URL(window.location.href);
    url.searchParams.set("service-health", next);
    window.location.assign(url);
  };

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      if (target?.matches("input, textarea, [contenteditable='true']")) return;
      if (event.key === "ArrowLeft") move(-1);
      if (event.key === "ArrowRight") move(1);
    };
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  });

  return (
    <div className="health-prototype-switcher">
      <button
        type="button"
        onClick={() => move(-1)}
        aria-label="Previous dashboard variant"
      >
        <ChevronLeft size={15} />
      </button>
      <span>
        {current.toUpperCase()} — {variantNames[current]}
      </span>
      <button
        type="button"
        onClick={() => move(1)}
        aria-label="Next dashboard variant"
      >
        <ChevronRight size={15} />
      </button>
    </div>
  );
}

export function ServiceHealthPrototype({ variant }: { variant: string }) {
  const selectedVariant = variants.includes(variant as Variant)
    ? (variant as Variant)
    : "workbench";
  const [drilldown, setDrilldown] = useState(
    "No drilldown selected — choose a chart, signal, or event.",
  );
  return (
    <Shell>
      {selectedVariant === "workbench" ? (
        <Workbench inspect={setDrilldown} />
      ) : null}
      {selectedVariant === "matrix" ? <Matrix inspect={setDrilldown} /> : null}
      {selectedVariant === "timeline" ? (
        <Timeline inspect={setDrilldown} />
      ) : null}
      <div className="health-drilldown">
        <Gauge size={13} />
        <span>{drilldown}</span>
      </div>
      <PrototypeSwitcher current={selectedVariant} />
    </Shell>
  );
}
