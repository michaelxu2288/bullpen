package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/web"
)

type Handlers struct {
	Engine *plan.Engine
	// Board is the live kanban state the dashboard reads and mutates.
	Board *BoardStore
	// Mesh is the webhook mesh: peer directory, bus, and delegation broker.
	Mesh *MeshRouter
	// Live, when set, is the running crew projecting onto the board. Without
	// it the board only changes by hand.
	Live *LiveRun
	// ServeUI mounts the embedded React dashboard at /.
	ServeUI bool

	startedAt time.Time
}

func NewHandlers(engine *plan.Engine) *Handlers {
	return &Handlers{
		Engine:    engine,
		Board:     NewBoardStore(),
		Mesh:     NewMeshRouter("master-0"),
		startedAt: time.Now(),
	}
}

// WithLive attaches a live run; /v1/run then feeds it instead of the static planner.
func (h *Handlers) WithLive(live *LiveRun) *Handlers {
	h.Live = live
	return h
}

// WithUI mounts the embedded dashboard bundle at the root.
func (h *Handlers) WithUI(enabled bool) *Handlers {
	h.ServeUI = enabled
	return h
}

func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("/healthz", h.health)
	mux.HandleFunc("/v1/run", h.runWorkflow)
	mux.HandleFunc("/v1/events", h.events)

	// dashboard surface
	mux.HandleFunc("/v1/summary", h.summary)
	mux.HandleFunc("/v1/board", h.boardSnapshot)
	mux.HandleFunc("/v1/board/advance", h.advanceCard)
	mux.HandleFunc("/v1/sessions", h.sessions)
	mux.HandleFunc("/v1/workers", h.workers)
	mux.HandleFunc("/v1/workers/kill", h.killWorker)
	mux.HandleFunc("/v1/workers/revive", h.reviveWorker)
	mux.HandleFunc("/v1/stream", h.stream)

	// agent mesh: webhook-reachable agents talking to and delegating to each other
	mux.HandleFunc("/v1/agents", h.listAgents)
	mux.HandleFunc("/v1/agents/register", h.registerAgent)
	mux.HandleFunc("/v1/agents/deregister", h.deregisterAgent)
	mux.HandleFunc("/v1/agents/inbox", h.agentInbox)
	mux.HandleFunc("/v1/agents/delegate", h.delegateTask)
	mux.HandleFunc("/v1/agents/messages", h.meshMessages)

	if h.ServeUI {
		mux.Handle("/", web.Handler())
	}
}

// contextWithTimeout bounds an outbound call made while serving a request.
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func (h *Handlers) health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"time": time.Now().UTC(),
	})
}

func (h *Handlers) runWorkflow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		Goal    string `json:"goal"`
		TraceID string `json:"trace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Goal) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": "goal is required"})
		return
	}

	// With a live crew attached, a run is real: tasks go on the board and
	// workers pick them up. Without one, fall back to the static planner.
	if h.Live != nil && h.Live.Running() {
		tasks := h.Live.Submit(req.Goal)
		respondJSON(w, http.StatusAccepted, map[string]any{
			"goal": req.Goal, "tasks": tasks, "live": true, "trace_id": req.TraceID,
		})
		return
	}

	result, err := h.Engine.Run(r.Context(), plan.RunInput{Goal: req.Goal, TraceID: req.TraceID})
	if err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	h.Board.ReplaceWithOwners(result.Tasks, result.SessionByTask)
	respondJSON(w, http.StatusOK, result)
}

func (h *Handlers) events(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.Engine.Events.History())
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
