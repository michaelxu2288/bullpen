/**
 * Mirrors the Go DTOs in internal/httpapi. Field names are
 * the JSON tags on the Go side; keep both ends in lockstep.
 */

export type TaskState = "queued" | "running" | "blocked" | "reviewing" | "done" | "failed";
export type SourceKind = "vector" | "slack" | "memory";
export type AgentRole = "planner" | "coder" | "reviewer" | "operator" | "unknown";

export interface BoardCard {
  id: string;
  title: string;
  state: TaskState;
  owner: string;
  reviewer: string;
  priority: number;
  labels: string[] | null;
  /** 0..1 while an agent holds the card, -1 otherwise. */
  progress: number;
  /** What the owning agent last said it was doing. */
  note?: string;
  requires_approval: boolean;
  updated_at: string;
  /** Pickups so far. Two is normal: code, then review. */
  attempts: number;
  /** Times the card bounced back to BACKLOG. */
  requeues: number;
}

export interface BoardLane {
  state: TaskState;
  title: string;
  cards: BoardCard[] | null;
}

export interface Board {
  lanes: BoardLane[];
  total: number;
  updated_at: string;
}


export interface Summary {
  service: string;
  uptime: string;
  uptime_ms: number;
  tasks: Record<string, number>;
  total_tasks: number;
  sessions: number;
  workers: number;
  workers_alive: number;
  live: boolean;
  events: number;
  watchers: number;
  now: string;
}

export interface AgentRow {
  name: string;
  provider: string;
  program: string;
  branch: string;
  worktree: string;
  tmux_session: string;
  created_at: string;
  assigned?: string;
}

export type WorkerState = "joining" | "alive" | "suspect" | "dead" | "draining";

export interface WorkerRow {
  id: string;
  provider: string;
  state: WorkerState;
  capabilities: string[] | null;
  in_flight: number;
  max_in_flight: number;
  last_heartbeat: string;
  holding: string[] | null;
}

export interface SwarmEvent {
  id: string;
  type: string;
  actor: string;
  target: string;
  payload: Record<string, unknown> | null;
  created_at: string;
}

export interface SlackMessageRef {
  channel: string;
  channel_name?: string;
  ts: string;
  thread_ts?: string;
  user?: string;
  text: string;
  permalink?: string;
}

export interface RetrievedChunk {
  id: string;
  source: SourceKind;
  text: string;
  score: number;
  fused_score?: number;
  metadata?: Record<string, string>;
  slack?: SlackMessageRef;
}

export interface Citation {
  ref: string;
  source: SourceKind;
  label: string;
  url?: string;
}

export interface GraphTraceEntry {
  node: string;
  started_at: string;
  duration_ms: number;
  note?: string;
}

export interface ContextPack {
  query: string;
  namespace: string;
  context: string;
  chunks: RetrievedChunk[];
  citations: Citation[];
  trace: GraphTraceEntry[];
  trace_id?: string;
  latency_ms: number;
  sources: SourceKind[];
}

export interface GraphTopology {
  name: string;
  nodes: string[];
  edges: Array<{ from: string; to: string }>;
}

export interface RetrievalRequest {
  query: string;
  agent?: { id: string; role: AgentRole };
  namespace?: string;
  top_k?: number;
  include_slack?: boolean;
  slack_channels?: string[];
  trace_id?: string;
}
