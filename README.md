# bullpen

A control plane for a crew of coding agents, in one Go binary.

A master hands tickets to worker agents over an internal gossip bus. Each worker
gets its own git worktree and PTY session, reports progress as it goes, and hands
finished work to a *different* worker for review before a ticket can close. The
whole fleet renders as a live board, in the terminal or in the browser.

Running on Replit: hit the webview and the board is already moving. Nothing on it
is placed by hand.

```
        MASTER  (scheduler, router, failure detector)
       /   |   \
  worker-1 | worker-3        each = git worktree + PTY + provider adapter
       worker-2
           |
      gossip bus  ->  board projection  ->  SSE  ->  TUI / dashboard
```

## What it does

- **Dispatch.** The master assigns a ticket to a worker that advertises the right
  capability, and records the assignment on the bus. Cards move because a worker
  said so.
- **Review.** A coder's finished ticket is re-dispatched to a reviewer. Nothing
  reaches DONE on the word of the worker that did it.
- **Failure.** Workers heartbeat. When one stops, the failure detector notices,
  its in-flight tickets bounce back to the backlog with a reason, and a survivor
  picks them up. `--chaos` kills a random worker on a timer so you can watch it.
- **Escalation.** A worker that cannot finish escalates instead of failing
  silently; the ticket returns to the backlog tagged with why.
- **Two front ends.** A Bubble Tea TUI and a React dashboard compiled into the
  binary with `go:embed`, both reading the same board over server-sent events.

## Run it

```sh
go run . server --chaos          # board, dashboard and a live crew
go run . server --live=false     # API only, static board
go run . tui                     # terminal board against a running server
```

The server binds `$PORT` when it is set, `:7070` otherwise.

Useful flags: `--workers`, `--task-min`, `--task-max`, `--fail-rate`, `--feed`,
`--chaos`, `--chaos-every`, `--chaos-down`, `--seed`, `--addr`, `--ui=false`.

## Build

```sh
make build      # builds the dashboard, then the binary
make test       # go test ./... and a dashboard typecheck
```

The dashboard bundle is committed under `internal/web/dist`, so a plain
`go build` produces a working binary without Node installed.

## API

| Route | Purpose |
|---|---|
| `GET /healthz` | liveness |
| `GET /v1/summary` | ticket counts, worker liveness, uptime |
| `GET /v1/board` | full board snapshot |
| `POST /v1/board/advance` | move a ticket by hand |
| `GET /v1/events` | event history |
| `GET /v1/stream` | SSE: events plus a board snapshot on every change |
| `POST /v1/run` | submit a goal, which is planned into tickets |

## Layout

```
cmd/              cobra commands
internal/mesh/    peers, bus, broker, master/worker scheduling, failure detection
internal/httpapi/ HTTP surface, board store, live run projection
internal/plan/    goal -> tickets, event bus
internal/crew/    agent identity and capabilities
internal/runners/ provider adapters (claude, codex, aider)
internal/pty/     PTY and tmux session handling
internal/tui/     Bubble Tea board
internal/web/     embedded dashboard bundle
web/              React dashboard source
```

MIT.
