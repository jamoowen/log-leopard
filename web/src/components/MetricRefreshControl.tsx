import { RefreshCw } from "lucide-react";
import "./metric-refresh-control.css";

interface Props {
  updatedAt: number;
  refreshing: boolean;
  disabled?: boolean;
  onRefresh: () => void;
}

export function MetricRefreshControl({
  updatedAt,
  refreshing,
  disabled = false,
  onRefresh,
}: Props) {
  const status = refreshing
    ? "UPDATING"
    : updatedAt
      ? `UPDATED ${new Intl.DateTimeFormat("en-GB", {
          hour: "2-digit",
          minute: "2-digit",
          hour12: false,
        }).format(updatedAt)}`
      : "NOT UPDATED";
  return (
    <div className="metric-refresh">
      <span className="metric-refresh-status" aria-live="polite">
        {status}
      </span>
      <button
        type="button"
        aria-label="Refresh metrics"
        disabled={disabled || refreshing}
        onClick={onRefresh}
      >
        <RefreshCw size={12} className={refreshing ? "spinning" : ""} />
        <span>{refreshing ? "Refreshing" : "Refresh"}</span>
      </button>
    </div>
  );
}
