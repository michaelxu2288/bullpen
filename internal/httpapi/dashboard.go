package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/mesh"
)

// Summary is what the dashboard status strip renders. One request, everything
// the operator needs to know the fleet is alive.
type Summary struct {
	Service      string         `json:"service"`
	Uptime       string         `json:"uptime"`
	UptimeMS     int64          `json:"uptime_ms"`
	Tasks        map[string]int `json:"tasks"`
	TotalTasks   int            `json:"total_tasks"`
	Sessions     int            `json:"sessions"`
	Workers      int            `json:"workers"`
	WorkersAlive int            `json:"workers_alive"`
	Live         bool           `json:"live"`
	Events       int            `json:"events"`
	Watchers     int            `json:"watchers"`
	Now          time.Time      `json:"now"`
}


// AgentRow is the sessions table projection.
type AgentRow struct {
	Name        string    `json:"name"`
	Provider    string    `json:"provider"`
	Program     string    `json:"program"`
	Branch      string    `json:"branch"`
	Worktree    string    `json:"worktree"`
	TmuxSession string    `json:"tmux_session"`
	CreatedAt   time.Time `json:"created_at"`
	// Assigned is the board card this session currently owns, if any.
	Assigned string `json:"assigned,omitempty"`
}

func (h *Handlers) summary(w http.ResponseWriter, r *http.Request) {
	board := h.Board.Snapshot()

	counts := map[string]int{}
	for _, lane := range board.Lanes {
		counts[string(lane.State)] = len(lane.Cards)
	}

	uptime := time.Since(h.startedAt)
	summary := Summary{
		Service:    "bullpen",
		Uptime:     uptime.Round(time.Second).String(),
		UptimeMS:   uptime.Milliseconds(),
		Tasks:      counts,
		TotalTasks: board.Total,
		Events:     len(h.Engine.Events.History()),
		Watchers:   h.Engine.Events.SubscriberCount(),
		Now:        time.Now().UTC(),
	}

	if sessions, err := loadSessions(); err == nil {
		summary.Sessions = len(sessions)
	}
	if h.Live != nil {
		summary.Live = h.Live.Running()
		for _, node := range h.Live.Workers() {
			summary.Workers++
			if node.State == mesh.NodeAlive {
				summary.WorkersAlive++
			}
		}
	}


	respondJSON(w, http.StatusOK, summary)
}

func (h *Handlers) boardSnapshot(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.Board.Snapshot())
}

func (h *Handlers) advanceCard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		ID    string           `json:"id"`
		State domain.TaskState `json:"state"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if req.ID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
		return
	}

	var (
		task domain.Task
		err  error
	)
	if req.State != "" {
		task, err = h.Board.SetState(req.ID, req.State)
	} else {
		task, err = h.Board.Advance(req.ID)
	}
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	h.Engine.Events.Publish(domain.Event{
		ID:        fmt.Sprintf("board-%d", time.Now().UnixNano()),
		Type:      domain.EventTaskStateChanged,
		Actor:     "dashboard",
		Target:    task.ID,
		Payload:   map[string]any{"state": string(task.State)},
		CreatedAt: time.Now(),
	})

	card, _ := h.Board.Card(task.ID)
	respondJSON(w, http.StatusOK, card)
}

func (h *Handlers) sessions(w http.ResponseWriter, r *http.Request) {
	sessions, err := loadSessions()
	if err != nil {
		// An operator with no sessions yet is the normal first-run state, not an error.
		respondJSON(w, http.StatusOK, []AgentRow{})
		return
	}

	owners := map[string]string{}
	for _, lane := range h.Board.Snapshot().Lanes {
		for _, card := range lane.Cards {
			if card.Owner != "" {
				owners[card.Owner] = card.ID
			}
		}
	}

	rows := make([]AgentRow, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, AgentRow{
			Name:        session.Name,
			Provider:    session.Provider,
			Program:     session.Program,
			Branch:      session.Branch,
			Worktree:    session.Worktree,
			TmuxSession: session.TmuxSession,
			CreatedAt:   session.CreatedAt,
			Assigned:    owners[session.Name],
		})
	}
	respondJSON(w, http.StatusOK, rows)
}

// WorkerRow is the fleet table projection.
type WorkerRow struct {
	ID            string    `json:"id"`
	Provider      string    `json:"provider"`
	State         string    `json:"state"`
	Capabilities  []string  `json:"capabilities"`
	InFlight      int       `json:"in_flight"`
	MaxInFlight   int       `json:"max_in_flight"`
	LastHeartbeat time.Time `json:"last_heartbeat"`
	// Holding lists the card ids this worker currently owns.
	Holding []string `json:"holding"`
}

func (h *Handlers) workers(w http.ResponseWriter, r *http.Request) {
	if h.Live == nil {
		respondJSON(w, http.StatusOK, []WorkerRow{})
		return
	}

	holding := map[string][]string{}
	for _, lane := range h.Board.Snapshot().Lanes {
		if lane.State != domain.TaskRunning && lane.State != domain.TaskReviewing {
			continue
		}
		for _, card := range lane.Cards {
			if card.Owner != "" {
				holding[card.Owner] = append(holding[card.Owner], card.ID)
			}
		}
	}

	nodes := h.Live.Workers()
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ID < nodes[j].ID })
	rows := make([]WorkerRow, 0, len(nodes))
	for _, node := range nodes {
		rows = append(rows, WorkerRow{
			ID:            node.ID,
			Provider:      node.Provider,
			State:         string(node.State),
			Capabilities:  node.Capabilities,
			InFlight:      node.InFlight,
			MaxInFlight:   node.MaxInFlight,
			LastHeartbeat: node.LastHeartbeat,
			Holding:       holding[node.ID],
		})
	}
	respondJSON(w, http.StatusOK, rows)
}

// killWorker stops a worker dead so the SWIM detector and the requeue path can
// be watched happening. revive brings it back under the same id.
func (h *Handlers) killWorker(w http.ResponseWriter, r *http.Request) {
	id, ok := h.workerTarget(w, r)
	if !ok {
		return
	}
	if err := h.Live.KillWorker(id); err != nil {
		respondJSON(w, http.StatusNotFound, map[string]any{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"killed": id, "detector": "swim", "note": "cards it held will requeue once it is declared dead"})
}

func (h *Handlers) reviveWorker(w http.ResponseWriter, r *http.Request) {
	id, ok := h.workerTarget(w, r)
	if !ok {
		return
	}
	node, err := h.Live.ReviveWorker(id)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"revived": node.ID, "incarnation": node.Incarnation})
}

func (h *Handlers) workerTarget(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return "", false
	}
	if h.Live == nil || !h.Live.Running() {
		respondJSON(w, http.StatusServiceUnavailable, map[string]any{"error": "no live crew; start the server with --live"})
		return "", false
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": "id is required"})
		return "", false
	}
	return req.ID, true
}

// boardPushInterval bounds how often the stream sends a board snapshot. Progress
// ticks arrive far faster than a person can read them.
const boardPushInterval = 200 * time.Millisecond

// stream is a server-sent event feed: orchestration events as they happen, and
// a fresh board snapshot whenever the board changes. The dashboard opens one of
// these and never polls for either.
func (h *Handlers) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		respondJSON(w, http.StatusInternalServerError, map[string]any{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	events, cancel := h.Engine.Events.Subscribe()
	defer cancel()
	boardChanged, stopBoard := h.Board.Changed.Subscribe()
	defer stopBoard()

	// Replay the tail so a freshly opened dashboard is not blank, and send the
	// board as it stands right now.
	history := h.Engine.Events.History()
	if len(history) > 25 {
		history = history[len(history)-25:]
	}
	for _, ev := range history {
		writeSSE(w, "event", ev)
	}
	writeSSE(w, "board", h.Board.Snapshot())
	flusher.Flush()

	// Keep intermediaries from closing an idle connection.
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()

	// Board pushes are throttled: a change arms the timer, and one snapshot goes
	// out when it fires, however many changes landed in between.
	var pushTimer *time.Timer
	var pushDue <-chan time.Time

	for {
		select {
		case <-r.Context().Done():
			return
		case ev, open := <-events:
			if !open {
				return
			}
			writeSSE(w, "event", ev)
			flusher.Flush()
		case <-boardChanged:
			if pushTimer == nil {
				pushTimer = time.NewTimer(boardPushInterval)
				pushDue = pushTimer.C
			}
		case <-pushDue:
			pushTimer, pushDue = nil, nil
			writeSSE(w, "board", h.Board.Snapshot())
			flusher.Flush()
		case <-heartbeat.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func writeSSE(w http.ResponseWriter, name string, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, body)
}

func loadSessions() ([]crew.Session, error) {
	store, err := crew.NewStore()
	if err != nil {
		return nil, err
	}
	return store.Load()
}
