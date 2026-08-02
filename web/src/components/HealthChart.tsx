import { useState, type KeyboardEvent, type MouseEvent } from "react";
import type { HealthPoint } from "../api/types";
import { formatCount, formatMetricValue } from "./metricFormat";

function formatMetricTimestamp(timestamp: string) {
  return new Intl.DateTimeFormat("en-GB", {
    day: "2-digit",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  }).format(new Date(timestamp));
}

const chartWidth = 800;
const chartHeight = 240;

function pointX(
  timestamp: string,
  domainStart: number,
  domainEnd: number,
  bucketMilliseconds: number,
) {
  const bucketCenter = new Date(timestamp).getTime() - bucketMilliseconds / 2;
  const ratio =
    (bucketCenter - domainStart) / Math.max(domainEnd - domainStart, 1);
  return Math.min(Math.max(ratio, 0), 1) * chartWidth;
}

function pointY(value: number, max: number) {
  return chartHeight - (value / max) * (chartHeight - 8);
}

function linePaths(
  values: HealthPoint[],
  domainStart: number,
  domainEnd: number,
  bucketMilliseconds: number,
) {
  if (!values.length) return { solid: "", gaps: "" };
  const max = Math.max(...values.map((point) => point.value), 1);
  const coordinates = values.map((point) => ({
    timestamp: new Date(point.timestamp).getTime(),
    x: pointX(point.timestamp, domainStart, domainEnd, bucketMilliseconds),
    y: pointY(point.value, max),
  }));
  if (coordinates.length === 1) {
    const point = coordinates[0]!;
    return {
      solid: `M${(point.x - 3).toFixed(1)} ${point.y.toFixed(1)} L${(point.x + 3).toFixed(1)} ${point.y.toFixed(1)}`,
      gaps: "",
    };
  }
  const solid: string[] = [];
  const gaps: string[] = [];
  for (let index = 1; index < coordinates.length; index++) {
    const previous = coordinates[index - 1]!;
    const current = coordinates[index]!;
    const segment = `M${previous.x.toFixed(1)} ${previous.y.toFixed(1)} L${current.x.toFixed(1)} ${current.y.toFixed(1)}`;
    (current.timestamp - previous.timestamp > bucketMilliseconds * 1.5
      ? gaps
      : solid
    ).push(segment);
  }
  return { solid: solid.join(" "), gaps: gaps.join(" ") };
}

function timeTicks(start: number, end: number) {
  return Array.from({ length: 5 }, (_, index) => ({
    index,
    timestamp: new Date(start + (index / 4) * (end - start)).toISOString(),
  }));
}

export function HealthChart({
  requests,
  errors,
  start,
  end,
  alignmentSeconds,
  onSelect,
  selectionFallback,
  showErrors = true,
  primaryLabel = "Requests",
  primaryTone = "request",
  valueUnit = "count",
}: {
  requests: HealthPoint[];
  errors: HealthPoint[];
  start: string;
  end: string;
  alignmentSeconds: number;
  onSelect?: (timestamp: string) => void;
  selectionFallback?: string;
  showErrors?: boolean;
  primaryLabel?: string;
  primaryTone?: "request" | "error" | "latency";
  valueUnit?: "count" | "ms";
}) {
  const [hoveredIndex, setHoveredIndex] = useState<number | null>(null);
  const [keyboardIndex, setKeyboardIndex] = useState<number | null>(null);
  const [focused, setFocused] = useState(false);
  const domainStart = new Date(start).getTime();
  const domainEnd = new Date(end).getTime();
  const domainDuration = Math.max(domainEnd - domainStart, 1);
  const bucketMilliseconds = alignmentSeconds * 1000;
  const maxRequests = Math.max(...requests.map((point) => point.value), 1);
  const maxErrors = Math.max(...errors.map((point) => point.value), 1);
  const requestLines = linePaths(
    requests,
    domainStart,
    domainEnd,
    bucketMilliseconds,
  );
  const errorLines = linePaths(
    errors,
    domainStart,
    domainEnd,
    bucketMilliseconds,
  );
  const fallbackIndex = Math.max(
    requests.findIndex((point) => point.timestamp === selectionFallback),
    0,
  );
  const activeIndex = hoveredIndex ?? keyboardIndex ?? fallbackIndex;
  const inspecting = hoveredIndex !== null || focused;
  const activeRequest = requests[activeIndex];
  const activeError = activeRequest
    ? errors.find((point) => point.timestamp === activeRequest.timestamp)
    : undefined;
  const activeX = activeRequest
    ? pointX(
        activeRequest.timestamp,
        domainStart,
        domainEnd,
        bucketMilliseconds,
      )
    : 0;
  function nearestIndex(clientX: number, element: HTMLElement) {
    const plot = element.querySelector<HTMLElement>(
      ".service-health-chart-plot",
    );
    const bounds = plot?.getBoundingClientRect();
    if (!bounds) return activeIndex;
    const plotWidth = Math.max(bounds.width, 1);
    const offset = Math.min(Math.max(clientX - bounds.left, 0), plotWidth);
    const pointerTime = domainStart + (offset / plotWidth) * domainDuration;
    let candidateIndex = 0;
    let nearestDistance = Number.POSITIVE_INFINITY;
    requests.forEach((point, index) => {
      const center =
        new Date(point.timestamp).getTime() - bucketMilliseconds / 2;
      const distance = Math.abs(center - pointerTime);
      if (distance < nearestDistance) {
        nearestDistance = distance;
        candidateIndex = index;
      }
    });
    return candidateIndex;
  }
  function updateHovered(event: MouseEvent<HTMLElement>) {
    setHoveredIndex(nearestIndex(event.clientX, event.currentTarget));
  }
  function navigate(event: KeyboardEvent<HTMLElement>) {
    let next = activeIndex;
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowDown":
        next = Math.max(activeIndex - 1, 0);
        break;
      case "ArrowRight":
      case "ArrowUp":
        next = Math.min(activeIndex + 1, requests.length - 1);
        break;
      case "Home":
        next = 0;
        break;
      case "End":
        next = requests.length - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    setFocused(true);
    setHoveredIndex(null);
    setKeyboardIndex(next);
  }
  const content = (
    <>
      <div className="service-health-chart-plot">
        <span
          className={`service-health-scale ${primaryTone}`}
          aria-hidden="true"
        >
          <strong>{formatMetricValue(maxRequests, valueUnit)}</strong>
          <small>{primaryLabel}</small>
          <span>0</span>
        </span>
        {showErrors && (
          <span className="service-health-scale errors" aria-hidden="true">
            <strong>{formatCount(maxErrors)}</strong>
            <small>5xx</small>
            <span>0</span>
          </span>
        )}
        <svg
          viewBox={`0 0 ${chartWidth} ${chartHeight}`}
          preserveAspectRatio="none"
        >
          {[1, 2, 3, 4].map((line) => (
            <line
              key={line}
              x1="0"
              x2={chartWidth}
              y1={line * (chartHeight / 5)}
              y2={line * (chartHeight / 5)}
              className="service-health-gridline"
            />
          ))}
          {showErrors && (
            <>
              <path
                d={errorLines.gaps}
                className="service-health-gap-line error"
                fill="none"
              />
              <path
                d={errorLines.solid}
                className="service-health-secondary-line"
                fill="none"
              />
            </>
          )}
          <path
            d={requestLines.gaps}
            className={`service-health-gap-line ${primaryTone}`}
            fill="none"
          />
          <path
            d={requestLines.solid}
            className={`service-health-request-line ${primaryTone}`}
            fill="none"
          />
          {inspecting && activeRequest && (
            <>
              <line
                x1={activeX}
                x2={activeX}
                y1="0"
                y2={chartHeight}
                className="service-health-crosshair"
              />
              <circle
                cx={activeX}
                cy={pointY(activeRequest.value, maxRequests)}
                r="4"
                className={`service-health-marker ${primaryTone}`}
              />
              {showErrors && activeError && (
                <circle
                  cx={activeX}
                  cy={pointY(activeError.value, maxErrors)}
                  r="4"
                  className="service-health-marker error"
                />
              )}
            </>
          )}
        </svg>
        {inspecting && activeRequest && (
          <span
            className={`service-health-tooltip ${activeX / chartWidth < 0.15 ? "start" : activeX / chartWidth > 0.85 ? "end" : ""}`}
            role="tooltip"
            style={{ left: `${(activeX / chartWidth) * 100}%` }}
          >
            <strong>
              Bucket ending {formatMetricTimestamp(activeRequest.timestamp)}
            </strong>
            <span>
              <i className={primaryTone} /> {primaryLabel}
              <b>{formatMetricValue(activeRequest.value, valueUnit)}</b>
            </span>
            {showErrors && (
              <span>
                <i className="error" /> 5xx
                <b>{formatCount(activeError?.value ?? 0)}</b>
              </span>
            )}
          </span>
        )}
      </div>
      <span className="service-health-axis">
        {timeTicks(domainStart, domainEnd).map(({ index, timestamp }) => (
          <span
            key={timestamp}
            className={index === 0 ? "start" : index === 4 ? "end" : ""}
            style={{ left: `${(index / 4) * 100}%` }}
          >
            {formatMetricTimestamp(timestamp)}
          </span>
        ))}
      </span>
    </>
  );
  const valueText = activeRequest
    ? `Bucket ending ${formatMetricTimestamp(activeRequest.timestamp)}. ${primaryLabel}: ${formatMetricValue(activeRequest.value, valueUnit)}${showErrors ? `, 5xx: ${formatCount(activeError?.value ?? 0)}` : ""}`
    : "No metric points";
  const leave = () => setHoveredIndex(null);
  const blur = () => {
    setFocused(false);
    setKeyboardIndex(null);
  };
  if (!onSelect) {
    return (
      <div
        className="service-health-chart"
        role="slider"
        tabIndex={0}
        aria-label={`${primaryLabel} metric interval`}
        aria-valuemin={0}
        aria-valuemax={Math.max(requests.length - 1, 0)}
        aria-valuenow={activeIndex}
        aria-valuetext={valueText}
        onMouseMove={updateHovered}
        onMouseLeave={leave}
        onFocus={() => setFocused(true)}
        onBlur={blur}
        onKeyDown={navigate}
      >
        {content}
      </div>
    );
  }
  return (
    <button
      className="service-health-chart"
      type="button"
      onMouseMove={updateHovered}
      onMouseLeave={leave}
      onFocus={() => setFocused(true)}
      onBlur={blur}
      onKeyDown={navigate}
      onClick={(event) => {
        const index = event.detail
          ? nearestIndex(event.clientX, event.currentTarget)
          : activeIndex;
        const point = requests[index];
        if (point) onSelect(point.timestamp);
      }}
      aria-label={`Open metric interval. ${valueText}`}
    >
      {content}
    </button>
  );
}
