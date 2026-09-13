import type { StreamStatus } from "../lib/hooks";
import type { Summary } from "../types";

interface Props {
  summary: Summary | null;
  streamStatus: StreamStatus;
  error: string | null;
}

function Stat({ label, value }: { label: string; value: string | number }) {
  return (
    <span className="stat">
      <span className="stat__label">{label}</span>
      <span className="stat__value">{value}</span>
    </span>
  );
}

/**
 * One line, everything an operator needs to know the fleet is alive. Order is
 * deliberate: what is running, then what is connected, then how long.
 */
export function StatusBar({ summary, streamStatus, error }: Props) {
  const plane = summary?.context_plane;
  const planeState = !plane?.attached ? "down" : plane.reachable ? "live" : "warn";

  return (
    <header className="status">
      <span className="status__brand">bullpen</span>

      {error ? (
        <span className="stat">
          <span className="dot dot--down" />
          <span className="stat__value">{error}</span>
        </span>
      ) : (
        <>
          <Stat label="tasks" value={summary?.total_tasks ?? "--"} />
          <Stat label="run" value={summary?.tasks.running ?? 0} />
          <Stat label="review" value={summary?.tasks.reviewing ?? 0} />
          <Stat label="done" value={summary?.tasks.done ?? 0} />
          <span className="stat">
            <span
              className={`dot dot--${
                !summary?.live ? "down" : (summary.workers_alive ?? 0) < (summary.workers ?? 0) ? "warn" : "live"
              }`}
            />
            <span className="stat__label">fleet</span>
            <span className="stat__value">
              {summary?.live ? `${summary.workers_alive}/${summary.workers}` : "static"}
            </span>
          </span>
        </>
      )}

      <span className="status__spacer" />

      <span className="stat">
        <span className={`dot dot--${planeState}`} />
        <span className="stat__label">plane</span>
        <span className="stat__value">
          {plane?.attached ? `${plane.slack_mcp || "?"}/${plane.vector_store || "?"}` : "detached"}
        </span>
      </span>

      <span className="stat">
        <span className={`dot dot--${streamStatus === "live" ? "live" : streamStatus === "down" ? "down" : "warn"}`} />
        <span className="stat__label">stream</span>
        <span className="stat__value">{streamStatus}</span>
      </span>

      <Stat label="up" value={summary?.uptime ?? "--"} />
    </header>
  );
}
