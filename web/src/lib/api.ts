import type {
  AgentRow,
  Board,
  WorkerRow,
  BoardCard,
  Summary,
  TaskState,
} from "../types";

export class ApiError extends Error {
  constructor(
    message: string,
    readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: { Accept: "application/json", ...(init?.body ? { "Content-Type": "application/json" } : {}), ...init?.headers },
  });
  const raw = await res.text();
  if (!res.ok) {
    let detail = raw.trim();
    try {
      const parsed = JSON.parse(raw) as { error?: string };
      if (parsed.error) detail = parsed.error;
    } catch {
      // non-json error body; use it as-is
    }
    throw new ApiError(detail || res.statusText, res.status);
  }
  return raw ? (JSON.parse(raw) as T) : ({} as T);
}

export const api = {
  summary: () => request<Summary>("/v1/summary"),
  board: () => request<Board>("/v1/board"),
  sessions: () => request<AgentRow[]>("/v1/sessions"),
  workers: () => request<WorkerRow[]>("/v1/workers"),
  killWorker: (id: string) =>
    request<{ killed: string }>("/v1/workers/kill", { method: "POST", body: JSON.stringify({ id }) }),
  reviveWorker: (id: string) =>
    request<{ revived: string }>("/v1/workers/revive", { method: "POST", body: JSON.stringify({ id }) }),

  advance: (id: string, state?: TaskState) =>
    request<BoardCard>("/v1/board/advance", {
      method: "POST",
      body: JSON.stringify(state ? { id, state } : { id }),
    }),


  run: (goal: string) =>
    request<unknown>("/v1/run", {
      method: "POST",
      body: JSON.stringify({ goal, trace_id: `ui-${Date.now()}` }),
    }),
};
