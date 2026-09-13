package plan

import (
	"context"
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/runners"
)

type Engine struct {
	Planner    Planner
	Router     Router
	Scheduler  Scheduler
	Supervisor Supervisor
	Strategies StrategyPlanner
	Registry   *runners.Registry
	Events     *EventBus
	Handoffs   *HandoffBus
}

type RunInput struct {
	Goal           string
	TraceID        string
	PlannerContext string
}

type RunResult struct {
	Tasks         []domain.Task
	Strategy      Strategy
	SessionByTask map[string]string
	StartedAt     time.Time
	FinishedAt    time.Time
}

func NewEngine(registry *runners.Registry) *Engine {
	return &Engine{
		Planner:    Planner{},
		Scheduler:  Scheduler{MaxConcurrent: 3},
		Supervisor: Supervisor{StaleAfter: 20 * time.Minute},
		Strategies: StrategyPlanner{},
		Registry:   registry,
		Events:     NewEventBus(),
		Handoffs:   NewHandoffBus(),
	}
}

func (e *Engine) Run(ctx context.Context, input RunInput) (*RunResult, error) {
	started := time.Now()
	tasks := e.Planner.DecomposeGoal(input.Goal)
	strategy := e.Strategies.Select(tasks)
	ordered := e.Scheduler.Order(tasks)

	sessionByTask := make(map[string]string, len(ordered))
	for _, task := range ordered {
		session, role := e.Router.RouteTask(task)
		sessionByTask[task.ID] = session
		e.Events.Publish(domain.Event{Type: domain.EventTaskAssigned, Actor: string(role), Target: task.ID, CreatedAt: time.Now()})

		if e.Registry != nil && session != "" {
			_ = e.invokeProvider(ctx, session, role, task, input.TraceID)
		}
	}

	result := &RunResult{
		Tasks:         ordered,
		Strategy:      strategy,
		SessionByTask: sessionByTask,
		StartedAt:     started,
		FinishedAt:    time.Now(),
	}
	return result, nil
}

func (e *Engine) invokeProvider(ctx context.Context, session string, role runners.Role, task domain.Task, traceID string) error {
	provider, err := e.Registry.Get(task.Metadata["provider"])
	if err != nil {
		provider, err = e.Registry.Get("claude")
		if err != nil {
			return nil
		}
	}

	resp, err := provider.Complete(ctx, runners.AgentContext{
		SessionName: session,
		Provider:    provider.Name(),
		Role:        role,
		TaskID:      task.ID,
		Metadata:    task.Metadata,
	}, runners.Prompt{Input: task.Description, TraceID: traceID, Tools: []string{"workflow.handoff", "git.diff"}})
	if err != nil {
		return err
	}

	if len(resp.SuggestedNext) > 0 {
		e.Handoffs.Push(HandoffMessage{
			From:    session,
			To:      session,
			TaskID:  task.ID,
			Mode:    "summary",
			Payload: fmt.Sprintf("next: %v", resp.SuggestedNext),
		})
	}
	return nil
}
