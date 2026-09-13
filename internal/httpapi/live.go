package httpapi

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/mesh"
)

// LiveRun connects the crew to the board.
//
// It is the loop that was missing: the master dispatches, workers execute and
// report over the bus, and every message is projected onto the kanban board
// the dashboard is watching. Nothing on the board is placed by hand; a card is
// in RUNNING because a worker acked it, its progress is what that worker last
// reported, and it is in DONE because a reviewer said so.
type LiveRun struct {
	router *MeshRouter
	board  *BoardStore
	events *plan.EventBus
	master *mesh.Master
	sim    mesh.SimOptions

	// Tick paces the dispatch loop and the failure detector.
	Tick time.Duration
	// Heartbeat is how often each worker announces itself.
	Heartbeat time.Duration

	mu      sync.Mutex
	workers map[string]*liveWorker
	phase   map[string]string // task id -> capability of the current phase
	prompt  map[string]string
	pending map[string]bool // tasks waiting for a worker
	cancel  context.CancelFunc
	ctx     context.Context
	seq     int
}

type liveWorker struct {
	node   mesh.Node
	cancel context.CancelFunc
}

// Phases each card passes through. A code task, once its worker reports a
// result, is re-dispatched to a reviewer; the reviewer's result moves it to DONE.
const (
	phaseCode   = "code"
	phaseReview = "review"
)

func NewLiveRun(router *MeshRouter, board *BoardStore, events *plan.EventBus, sim mesh.SimOptions) *LiveRun {
	return &LiveRun{
		router:    router,
		board:     board,
		events:    events,
		master:    mesh.NewMasterWith(router.SelfID, router.Bus, router.Registry),
		sim:       sim,
		Tick:      500 * time.Millisecond,
		Heartbeat: 400 * time.Millisecond,
		workers:   map[string]*liveWorker{},
		phase:     map[string]string{},
		prompt:    map[string]string{},
		pending:   map[string]bool{},
	}
}

// Start boots the projection, the dispatcher, the failure detector, and n
// workers. It returns immediately; everything runs until ctx is cancelled or
// Stop is called.
func (l *LiveRun) Start(ctx context.Context, n int) {
	l.mu.Lock()
	if l.cancel != nil {
		l.mu.Unlock()
		return
	}
	l.ctx, l.cancel = context.WithCancel(ctx)
	ctx = l.ctx
	l.mu.Unlock()

	tap, stopTap := l.router.Bus.Tap()
	go func() {
		defer stopTap()
		for {
			select {
			case <-ctx.Done():
				return
			case m, open := <-tap:
				if !open {
					return
				}
				l.project(m)
			}
		}
	}()

	go l.dispatchLoop(ctx)

	// The SWIM detector: a worker that stops heartbeating goes suspect, then
	// dead, and its cards come back to BACKLOG for someone else.
	go l.master.Run(ctx, l.Tick, func(item mesh.WorkItem) {
		l.requeue(item.TaskID, "worker died; requeued")
	})

	for i := 1; i <= n; i++ {
		l.SpawnWorker(fmt.Sprintf("worker-%d", i))
	}
}

// Stop tears everything down.
func (l *LiveRun) Stop() {
	l.mu.Lock()
	cancel := l.cancel
	l.cancel = nil
	l.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// Running reports whether Start has been called and not yet stopped.
func (l *LiveRun) Running() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.cancel != nil
}

// SpawnWorker adds a simulated agent to the fleet. Every worker can code and
// review; capability routing still matters because the master prefers the
// least loaded, so the two phases spread across the fleet.
func (l *LiveRun) SpawnWorker(id string) mesh.Node {
	l.mu.Lock()
	ctx := l.ctx
	if existing, ok := l.workers[id]; ok {
		l.mu.Unlock()
		return existing.node
	}
	l.mu.Unlock()

	node := mesh.Node{
		ID:           id,
		Provider:     providerFor(id),
		Capabilities: []string{phaseCode, phaseReview},
		MaxInFlight:  2,
		Labels:       map[string]string{"simulated": "true"},
	}

	worker := mesh.NewWorker(node, l.router.Bus)
	worker.HandleWithProgress = mesh.SimulatedHandler(l.sim)
	l.master.Register(node)

	if ctx == nil {
		ctx = context.Background()
	}
	wctx, cancel := context.WithCancel(ctx)
	go worker.Run(wctx, l.Heartbeat, l.master.ID)

	l.mu.Lock()
	l.workers[id] = &liveWorker{node: node, cancel: cancel}
	l.mu.Unlock()

	l.event(domain.EventSessionLaunched, id, "crew", map[string]any{"provider": node.Provider})
	return node
}

// Submit plans a goal into tasks and queues them for dispatch.
func (l *LiveRun) Submit(goal string) []domain.Task {
	tasks := plan.Planner{}.DecomposeGoal(goal)

	l.mu.Lock()
	l.seq++
	run := l.seq
	for i := range tasks {
		// task ids from the planner repeat per goal; namespace them by run so
		// the board can hold more than one goal at a time
		tasks[i].ID = fmt.Sprintf("r%d-%s", run, tasks[i].ID)
		tasks[i].Metadata["goal"] = goal
		l.phase[tasks[i].ID] = phaseCode
		l.prompt[tasks[i].ID] = tasks[i].Description
		l.pending[tasks[i].ID] = true
	}
	l.mu.Unlock()

	l.board.Add(tasks)
	for _, task := range tasks {
		l.event(domain.EventTaskCreated, "planner", task.ID, map[string]any{"goal": goal})
	}
	return tasks
}

// dispatchLoop hands every pending task to a worker as soon as one has room.
// Running it on a tick rather than on demand keeps the retry path trivial: a
// task nobody could take just stays pending until the next tick.
func (l *LiveRun) dispatchLoop(ctx context.Context) {
	ticker := time.NewTicker(l.Tick / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.dispatchPending()
			l.markSuspects()
		}
	}
}

func (l *LiveRun) dispatchPending() {
	l.mu.Lock()
	ids := make([]string, 0, len(l.pending))
	for id := range l.pending {
		ids = append(ids, id)
	}
	l.mu.Unlock()

	// The planner assigns priorities and the board sorts by them; dispatch has
	// to honour them too, or BACKLOG order is a lie. Ties break on id so the
	// order is stable rather than map-random.
	sort.Slice(ids, func(i, j int) bool {
		a, _ := l.board.Card(ids[i])
		b, _ := l.board.Card(ids[j])
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return ids[i] < ids[j]
	})

	for _, id := range ids {
		l.mu.Lock()
		capability := l.phase[id]
		prompt := l.prompt[id]
		l.mu.Unlock()

		node, err := l.master.Dispatch(mesh.WorkItem{TaskID: id, Capability: capability, Prompt: prompt})
		if err != nil {
			continue // fleet is saturated; try again next tick
		}

		l.mu.Lock()
		delete(l.pending, id)
		l.mu.Unlock()

		state := domain.TaskRunning
		if capability == phaseReview {
			state = domain.TaskReviewing
		}
		_ = l.board.Assign(id, node, state)
		l.event(domain.EventTaskAssigned, node, id, map[string]any{"phase": capability})
	}
}

// project applies one bus message to the board.
func (l *LiveRun) project(m mesh.Message) {
	switch m.Kind {
	case mesh.MsgHeartbeat:
		l.router.Registry.Heartbeat(m.From)

	case mesh.MsgProgress:
		fraction, _ := strconv.ParseFloat(m.Headers["progress"], 64)
		l.board.SetProgress(m.TaskID, fraction, m.Body)

	case mesh.MsgResult:
		l.master.Complete(m.TaskID)
		l.mu.Lock()
		phase := l.phase[m.TaskID]
		l.mu.Unlock()

		switch phase {
		case phaseCode:
			// the coder is done; the same card now needs a reviewer
			l.mu.Lock()
			l.phase[m.TaskID] = phaseReview
			l.prompt[m.TaskID] = "review: " + l.prompt[m.TaskID]
			l.pending[m.TaskID] = true
			l.mu.Unlock()
			_, _ = l.board.SetState(m.TaskID, domain.TaskReviewing)
			l.board.SetProgress(m.TaskID, 0, "awaiting a reviewer")
			l.event(domain.EventTaskStateChanged, m.From, m.TaskID, map[string]any{"state": "reviewing"})
		case phaseReview:
			_, _ = l.board.SetState(m.TaskID, domain.TaskDone)
			l.board.SetProgress(m.TaskID, 1, "approved")
			l.event(domain.EventTaskStateChanged, m.From, m.TaskID, map[string]any{"state": "done"})
		}

	case mesh.MsgEscalate:
		l.master.Complete(m.TaskID)
		l.requeue(m.TaskID, "escalated: "+trimPrefix(m.Body, "error: "))
		l.event(domain.EventHITLRequested, m.From, m.TaskID, map[string]any{"reason": m.Body})
	}
}

// requeue returns a card to BACKLOG and marks it for redispatch. It is
// idempotent, because a task can bounce for two reasons at once — the worker
// escalated and then the detector reaped it.
func (l *LiveRun) requeue(taskID, reason string) {
	l.mu.Lock()
	if _, known := l.phase[taskID]; !known {
		l.mu.Unlock()
		return
	}
	l.pending[taskID] = true
	l.mu.Unlock()
	_ = l.board.Requeue(taskID, reason)
	l.event(domain.EventTaskStateChanged, "master", taskID, map[string]any{"state": "queued", "reason": reason})
}

// Workers returns the fleet with liveness, for the dashboard.
func (l *LiveRun) Workers() []mesh.Node {
	return l.router.Registry.Workers()
}

func (l *LiveRun) event(kind domain.EventType, actor, target string, payload map[string]any) {
	l.events.Publish(domain.Event{
		ID:        fmt.Sprintf("live-%d", time.Now().UnixNano()),
		Type:      kind,
		Actor:     actor,
		Target:    target,
		Payload:   payload,
		CreatedAt: time.Now(),
	})
}

// providerFor spreads simulated workers across the adapters the registry
// knows, so the fleet reads as heterogeneous on the board.
func providerFor(id string) string {
	providers := []string{"claude", "codex", "aider", "gemini"}
	var sum int
	for _, c := range id {
		sum += int(c)
	}
	return providers[sum%len(providers)]
}

func trimPrefix(s, prefix string) string {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):]
	}
	return s
}

// DemoGoals is what the feeder cycles through. They are phrased the way a
// person would type them, because they end up on the cards.
var DemoGoals = []string{
	"ship the checkout latency fix",
	"add retry budgets to the payments client",
	"rag: index the incident postmortems",
	"cut the auth service 429 storm",
	"migrate the kanban board to the event bus",
	"harden the webhook mesh against replay",
}

// Feed keeps the board busy: whenever every card has reached DONE and stays
// that way for idleFor, the next goal is submitted. It returns when ctx ends.
func (l *LiveRun) Feed(ctx context.Context, goals []string, idleFor time.Duration) {
	if len(goals) == 0 {
		goals = DemoGoals
	}
	if idleFor <= 0 {
		idleFor = 3 * time.Second
	}

	next := 0
	var idleSince time.Time
	ticker := time.NewTicker(l.Tick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if l.board.Active() {
				idleSince = time.Time{}
				continue
			}
			if idleSince.IsZero() {
				idleSince = time.Now()
				continue
			}
			if time.Since(idleSince) < idleFor {
				continue
			}
			l.Submit(goals[next%len(goals)])
			next++
			idleSince = time.Time{}
		}
	}
}

// KillWorker stops a worker dead: no escalation, no goodbye. Its heartbeats
// stop, the SWIM detector moves it alive -> suspect -> dead, and the cards it
// was holding come back to BACKLOG for someone else. The node stays in the
// registry so the fleet view shows the transition rather than a vanishing row.
func (l *LiveRun) KillWorker(id string) error {
	l.mu.Lock()
	w, ok := l.workers[id]
	if ok {
		delete(l.workers, id)
	}
	l.mu.Unlock()
	if !ok {
		return fmt.Errorf("no such worker: %s", id)
	}
	w.cancel()
	l.event(domain.EventSessionLaunched, id, "killed", map[string]any{"state": "killed"})
	return nil
}

// ReviveWorker brings a killed worker back under the same id. Registry.Join
// bumps its incarnation and marks it alive, so the next detector tick sees a
// healthy node again.
func (l *LiveRun) ReviveWorker(id string) (mesh.Node, error) {
	l.mu.Lock()
	_, alive := l.workers[id]
	l.mu.Unlock()
	if alive {
		return mesh.Node{}, fmt.Errorf("worker %s is already running", id)
	}
	return l.SpawnWorker(id), nil
}

// markSuspects annotates cards whose owner has stopped heartbeating, so the
// board tells the story between "working" and "requeued" instead of freezing.
func (l *LiveRun) markSuspects() {
	states := map[string]mesh.NodeState{}
	for _, node := range l.router.Registry.Workers() {
		states[node.ID] = node.State
	}
	for _, lane := range l.board.Snapshot().Lanes {
		if lane.State != domain.TaskRunning && lane.State != domain.TaskReviewing {
			continue
		}
		for _, card := range lane.Cards {
			switch states[card.Owner] {
			case mesh.NodeSuspect:
				l.board.AnnotateIfHeld(card.ID, card.Owner, card.Owner+" stopped responding")
			case mesh.NodeDead:
				l.board.AnnotateIfHeld(card.ID, card.Owner, card.Owner+" is dead; waiting for requeue")
			}
		}
	}
}

// Chaos kills a random live worker every `every` and revives it `downFor`
// later. It is the demo's way of showing the failure path without anyone
// having to type a kill command at the right moment.
func (l *LiveRun) Chaos(ctx context.Context, every, downFor time.Duration) {
	if every <= 0 {
		every = 25 * time.Second
	}
	if downFor <= 0 {
		downFor = 8 * time.Second
	}
	ticker := time.NewTicker(every)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			ids := make([]string, 0, len(l.workers))
			for id := range l.workers {
				ids = append(ids, id)
			}
			l.mu.Unlock()
			// never take the fleet below two: the point is seeing work move to
			// a survivor, not watching a queue with nobody to serve it
			if len(ids) < 2 {
				continue
			}
			victim := ids[time.Now().UnixNano()%int64(len(ids))]
			if err := l.KillWorker(victim); err != nil {
				continue
			}
			l.event(domain.EventSessionLaunched, "chaos", victim, map[string]any{"state": "killed", "revive_in": downFor.String()})

			time.AfterFunc(downFor, func() {
				select {
				case <-ctx.Done():
				default:
					_, _ = l.ReviveWorker(victim)
				}
			})
		}
	}
}
