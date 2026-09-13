import { useState } from "react";

import { api } from "../lib/api";
import { ago } from "../lib/hooks";
import type { AgentRow, WorkerRow, WorkerState } from "../types";

interface Props {
  workers: WorkerRow[] | null;
  sessions: AgentRow[] | null;
  error: string | null;
  onFleetChange: () => void;
}

const stateDot: Record<WorkerState, string> = {
  joining: "warn",
  alive: "live",
  suspect: "warn",
  dead: "down",
  draining: "warn",
};

/**
 * Two tables, because they are two different things: the crew fleet (workers
 * the master dispatches to, with SWIM liveness) and the tmux-backed sessions
 * launched with `orchestrate`.
 */
export function SessionsView({ workers, sessions, error, onFleetChange }: Props) {
  const fleet = workers ?? [];
  const rows = sessions ?? [];
  const alive = fleet.filter((w) => w.state === "alive").length;
  const [busy, setBusy] = useState<string | null>(null);
  const [fleetError, setFleetError] = useState<string | null>(null);

  const act = async (id: string, kind: "kill" | "revive") => {
    setBusy(id);
    setFleetError(null);
    try {
      if (kind === "kill") await api.killWorker(id);
      else await api.reviveWorker(id);
      onFleetChange();
    } catch (err) {
      setFleetError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(null);
    }
  };

  return (
    <>
      <div className="view-head">
        <h1 className="view-head__title">fleet</h1>
        <span className="view-head__meta">
          {fleet.length === 0
            ? "no live crew"
            : `${alive}/${fleet.length} workers alive · SWIM heartbeat · kill one and watch its cards requeue`}
        </span>
      </div>

      {fleetError && <p className="notice notice--error">{fleetError}</p>}

      {fleet.length === 0 ? (
        <p className="empty">
          no workers. start the server with <code>--live</code> (the default) to boot a fleet.
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th></th>
              <th>worker</th>
              <th>provider</th>
              <th>state</th>
              <th>in flight</th>
              <th>holding</th>
              <th>heartbeat</th>
              <th></th>
            </tr>
          </thead>
          <tbody>
            {fleet.map((w) => (
              <tr key={w.id} data-state={w.state}>
                <td>
                  <span className={`dot dot--${stateDot[w.state] ?? "warn"}`} />
                </td>
                <td className="key">{w.id}</td>
                <td>{w.provider}</td>
                <td>{w.state}</td>
                <td className="num">
                  {w.in_flight}/{w.max_in_flight}
                </td>
                <td>{w.holding?.length ? w.holding.join(", ") : "—"}</td>
                <td className="num">{ago(w.last_heartbeat)}</td>
                <td className="actions">
                  {w.state === "dead" ? (
                    <button
                      type="button"
                      className="btn btn--small"
                      disabled={busy === w.id}
                      onClick={() => void act(w.id, "revive")}
                    >
                      revive
                    </button>
                  ) : (
                    <button
                      type="button"
                      className="btn btn--small btn--danger"
                      disabled={busy === w.id || w.state !== "alive"}
                      onClick={() => void act(w.id, "kill")}
                    >
                      kill
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <div className="view-head" style={{ marginTop: 24 }}>
        <h1 className="view-head__title">sessions</h1>
        <span className="view-head__meta">{rows.length} tmux-backed · one git worktree each</span>
      </div>

      {error && <p className="notice notice--error">{error}</p>}

      {rows.length === 0 && !error ? (
        <p className="empty">
          no agent sessions yet. launch one with{" "}
          <code>bullpen orchestrate --name planner --provider claude</code>
        </p>
      ) : (
        <table className="table">
          <thead>
            <tr>
              <th>name</th>
              <th>provider</th>
              <th>branch</th>
              <th>tmux</th>
              <th>assigned</th>
              <th>worktree</th>
              <th>age</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <tr key={row.name}>
                <td className="key">{row.name}</td>
                <td>{row.provider}</td>
                <td>{row.branch}</td>
                <td>{row.tmux_session}</td>
                <td>{row.assigned ?? "—"}</td>
                <td className="truncate" title={row.worktree}>
                  {row.worktree}
                </td>
                <td className="num">{ago(row.created_at)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}
