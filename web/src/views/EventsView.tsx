import { useEffect, useRef } from "react";

import { clockOf } from "../lib/hooks";
import type { StreamStatus } from "../lib/hooks";
import type { SwarmEvent } from "../types";

interface Props {
  events: SwarmEvent[];
  status: StreamStatus;
}

/** Live tail of the orchestration bus. Sticks to the bottom unless scrolled up. */
export function EventsView({ events, status }: Props) {
  const endRef = useRef<HTMLDivElement>(null);
  const pinnedRef = useRef(true);

  useEffect(() => {
    if (pinnedRef.current) endRef.current?.scrollIntoView({ block: "end" });
  }, [events.length]);

  return (
    <>
      <div className="view-head">
        <h1 className="view-head__title">events</h1>
        <span className="view-head__meta">
          {events.length} on the wire · stream {status}
        </span>
      </div>

      <div
        className="feed"
        onScroll={(e) => {
          const el = e.currentTarget;
          pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
        }}
      >
        {events.length === 0 && <p className="empty">waiting for the first event…</p>}
        {events.map((event, i) => (
          <div className="feed__row" key={`${event.id}-${i}`}>
            <span className="feed__time">{clockOf(event.created_at)}</span>
            <span className="feed__type">{event.type}</span>
            <span className="feed__actor">{event.actor || "—"}</span>
            <span className="feed__target">{event.target}</span>
          </div>
        ))}
        <div ref={endRef} />
      </div>
    </>
  );
}
