import { useEffect, useMemo } from "react";

import { ago, useKeys } from "../lib/hooks";
import type { Board, BoardCard, TaskState } from "../types";

interface Props {
  board: Board | null;
  error: string | null;
  selected: string | null;
  onSelect: (id: string | null) => void;
  onAdvance: (id: string) => void;
  onRefresh: () => void;
}

const laneColor: Record<string, string> = {
  queued: "var(--lane-queued)",
  running: "var(--lane-running)",
  reviewing: "var(--lane-reviewing)",
  done: "var(--lane-done)",
};

function Card({
  card,
  selected,
  onSelect,
}: {
  card: BoardCard;
  selected: boolean;
  onSelect: () => void;
}) {
  return (
    <button type="button" className="card" data-selected={selected} onClick={onSelect}>
      <span className="card__top">
        <span className="card__id">{card.id}</span>
        <span>p{card.priority}</span>
        <span className="card__age">{ago(card.updated_at)}</span>
      </span>

      <p className="card__title">{card.title}</p>

      <span className="card__foot">
        <span className="card__owner">{card.owner || "unassigned"}</span>
        {card.requeues > 0 && (
          <span className="card__retry" title="bounced back to backlog">
            ↺{card.requeues}
          </span>
        )}
        {card.requires_approval && <span className="card__flag">hitl</span>}
      </span>

      {card.progress >= 0 && (
        <span className="meter">
          <span className="meter__track">
            <span
              className="meter__fill"
              data-lane={card.state}
              style={{ width: `${Math.round(card.progress * 100)}%` }}
            />
          </span>
          <span>{Math.round(card.progress * 100)}%</span>
        </span>
      )}

      {card.note && <span className="card__note">{card.note}</span>}
    </button>
  );
}

/**
 * The board is the default view because the operator's first question is
 * "is anything stuck". Navigation mirrors the TUI: h/l across lanes, j/k within
 * one, space to advance.
 */
export function BoardView({ board, error, selected, onSelect, onAdvance, onRefresh }: Props) {
  const lanes = useMemo(
    () => (board?.lanes ?? []).map((lane) => ({ ...lane, cards: lane.cards ?? [] })),
    [board],
  );

  const position = useMemo(() => {
    for (const [laneIndex, lane] of lanes.entries()) {
      const cardIndex = lane.cards.findIndex((c) => c.id === selected);
      if (cardIndex >= 0) return { laneIndex, cardIndex };
    }
    return null;
  }, [lanes, selected]);

  // keep a selection alive as cards move between lanes
  useEffect(() => {
    if (position || lanes.length === 0) return;
    const firstLaneWithCards = lanes.find((lane) => lane.cards.length > 0);
    if (firstLaneWithCards?.cards[0]) onSelect(firstLaneWithCards.cards[0].id);
  }, [lanes, position, onSelect]);

  useKeys((key) => {
    if (key === "r") {
      onRefresh();
      return;
    }
    if (key === " " && selected) {
      onAdvance(selected);
      return;
    }
    if (!position) return;

    const move = (laneDelta: number, cardDelta: number) => {
      let laneIndex = position.laneIndex + laneDelta;
      laneIndex = Math.max(0, Math.min(lanes.length - 1, laneIndex));
      const lane = lanes[laneIndex];
      if (!lane || lane.cards.length === 0) return;
      const cardIndex = laneDelta !== 0
        ? Math.min(position.cardIndex, lane.cards.length - 1)
        : Math.max(0, Math.min(lane.cards.length - 1, position.cardIndex + cardDelta));
      const next = lane.cards[cardIndex];
      if (next) onSelect(next.id);
    };

    if (key === "h" || key === "ArrowLeft") move(-1, 0);
    if (key === "l" || key === "ArrowRight") move(1, 0);
    if (key === "j" || key === "ArrowDown") move(0, 1);
    if (key === "k" || key === "ArrowUp") move(0, -1);
  });

  if (error) return <p className="notice notice--error">{error}</p>;

  return (
    <>
      <div className="view-head">
        <h1 className="view-head__title">board</h1>
        <span className="view-head__meta">
          {board?.total ?? 0} tasks
          {selected ? ` · ${selected} selected` : ""}
        </span>
      </div>

      <div className="board">
        {lanes.map((lane) => (
          <section className="lane" key={lane.state}>
            <header className="lane__head">
              <span
                className="lane__rule"
                style={{ background: laneColor[lane.state] ?? "var(--fg-300)" }}
              />
              <span>{lane.title}</span>
              <span className="lane__count">{lane.cards.length}</span>
            </header>

            <div className="lane__cards">
              {lane.cards.length === 0 && <p className="lane__empty">—</p>}
              {lane.cards.map((card) => (
                <Card
                  key={card.id}
                  card={card}
                  selected={card.id === selected}
                  onSelect={() => onSelect(card.id)}
                />
              ))}
            </div>
          </section>
        ))}
      </div>
    </>
  );
}

export type { TaskState };
