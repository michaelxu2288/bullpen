import { useCallback, useState } from "react";

import { KeyHints } from "./components/KeyHints";
import { Rail, type ViewKey } from "./components/Rail";
import { StatusBar } from "./components/StatusBar";
import { api } from "./lib/api";
import { useEventStream, useKeys, useResource } from "./lib/hooks";
import { BoardView } from "./views/BoardView";
import { EventsView } from "./views/EventsView";
import { SessionsView } from "./views/SessionsView";

export function App() {
  const [view, setView] = useState<ViewKey>("board");
  const [selected, setSelected] = useState<string | null>(null);

  const summary = useResource(api.summary, 4000);
  // The stream pushes the board on every change; the poll is only a fallback
  // for when the stream is down.
  const polledBoard = useResource(api.board, 10000);
  const sessions = useResource(api.sessions, 8000);
  const workers = useResource(api.workers, 2000);
  const { events, status, board: streamedBoard } = useEventStream();
  const boardData = streamedBoard ?? polledBoard.data;

  const advance = useCallback(
    async (id: string) => {
      try {
        await api.advance(id);
      } catch {
        // the board refresh below will show the real state either way
      }
      polledBoard.refresh();
      summary.refresh();
    },
    [polledBoard, summary],
  );

  useKeys((key) => {
    const byKey: Record<string, ViewKey> = {
      "1": "board",
      "2": "sessions",
      "3": "events",
    };
    const next = byKey[key];
    if (next) setView(next);
  });

  return (
    <div className="app">
      <StatusBar summary={summary.data} streamStatus={status} error={summary.error} />

      <Rail
        active={view}
        counts={{
          board: boardData?.total,
          sessions: (workers.data?.length ?? 0) + (sessions.data?.length ?? 0),
          events: events.length,
        }}
        onSelect={setView}
      />

      <main className="main">
        {view === "board" && (
          <BoardView
            board={boardData}
            error={streamedBoard ? null : polledBoard.error}
            selected={selected}
            onSelect={setSelected}
            onAdvance={(id) => void advance(id)}
            onRefresh={polledBoard.refresh}
          />
        )}
        {view === "sessions" && (
          <SessionsView
            workers={workers.data}
            sessions={sessions.data}
            error={sessions.error}
            onFleetChange={() => {
              workers.refresh();
              summary.refresh();
            }}
          />
        )}
        {view === "events" && <EventsView events={events} status={status} />}
      </main>

      <KeyHints view={view} />
    </div>
  );
}
