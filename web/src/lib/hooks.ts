import { useCallback, useEffect, useRef, useState } from "react";

import type { Board, SwarmEvent } from "../types";

export interface Resource<T> {
  data: T | null;
  error: string | null;
  loading: boolean;
  refresh: () => void;
}

/**
 * Fetch on mount and on an interval. Deliberately dumb: the board and summary
 * are small, and the event stream carries the urgent updates.
 */
export function useResource<T>(load: () => Promise<T>, intervalMs = 4000): Resource<T> {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [tick, setTick] = useState(0);
  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    let alive = true;
    const run = async () => {
      try {
        const next = await loadRef.current();
        if (!alive) return;
        setData(next);
        setError(null);
      } catch (err) {
        if (!alive) return;
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        if (alive) setLoading(false);
      }
    };

    void run();
    if (intervalMs <= 0) return () => { alive = false; };
    const timer = setInterval(() => void run(), intervalMs);
    return () => {
      alive = false;
      clearInterval(timer);
    };
  }, [intervalMs, tick]);

  const refresh = useCallback(() => setTick((t) => t + 1), []);
  return { data, error, loading, refresh };
}

export type StreamStatus = "connecting" | "live" | "down";

/**
 * Subscribe to /v1/stream: orchestration events as they happen, and a board
 * snapshot whenever the board changes. EventSource reconnects on its own; we
 * only track the status so the header can say whether what is on screen is
 * current.
 */
export function useEventStream(limit = 300): {
  events: SwarmEvent[];
  status: StreamStatus;
  board: Board | null;
} {
  const [events, setEvents] = useState<SwarmEvent[]>([]);
  const [board, setBoard] = useState<Board | null>(null);
  const [status, setStatus] = useState<StreamStatus>("connecting");

  useEffect(() => {
    const source = new EventSource("/v1/stream");

    source.addEventListener("open", () => setStatus("live"));
    source.addEventListener("error", () => setStatus("down"));
    source.addEventListener("board", (raw) => {
      try {
        setBoard(JSON.parse((raw as MessageEvent<string>).data) as Board);
      } catch {
        // a malformed frame should not kill the feed
      }
    });
    source.addEventListener("event", (raw) => {
      setStatus("live");
      try {
        const parsed = JSON.parse((raw as MessageEvent<string>).data) as SwarmEvent;
        setEvents((prev) => {
          const next = [...prev, parsed];
          return next.length > limit ? next.slice(next.length - limit) : next;
        });
      } catch {
        // a malformed frame should not kill the feed
      }
    });

    return () => source.close();
  }, [limit]);

  return { events, status, board };
}

/** Global key handler that stays out of the way while you are typing. */
export function useKeys(handler: (key: string, event: KeyboardEvent) => void): void {
  const ref = useRef(handler);
  ref.current = handler;

  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      const target = event.target as HTMLElement | null;
      const typing =
        target?.tagName === "INPUT" || target?.tagName === "TEXTAREA" || target?.isContentEditable;
      if (typing && event.key !== "Escape") return;
      ref.current(event.key, event);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);
}

/** Relative time, in the terse form a log reader expects. */
export function ago(iso: string): string {
  const then = new Date(iso).getTime();
  if (Number.isNaN(then)) return "--";
  const seconds = Math.max(0, Math.floor((Date.now() - then) / 1000));
  if (seconds < 60) return `${seconds}s`;
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

export function clockOf(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "--:--:--";
  return date.toLocaleTimeString("en-GB", { hour12: false });
}
