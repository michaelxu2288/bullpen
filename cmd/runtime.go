package cmd

import (
	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/michaelxu2288/bullpen/internal/runners"
)

func newProviderRegistry() *runners.Registry {
	r := runners.NewRegistry()
	r.Register(runners.ClaudeAdapter{})
	r.Register(runners.CodexAdapter{})
	r.Register(runners.AiderAdapter{})
	r.Register(runners.GeminiAdapter{})
	return r
}

func newEngine() *plan.Engine {
	registry := newProviderRegistry()
	engine := plan.NewEngine(registry)
	engine.Router = plan.Router{
		PlannerSession:  "planner-claude",
		CoderSession:    "coder-codex",
		ReviewerSession: "reviewer-claude",
	}
	return engine
}

func orchestrationInput(goal string) plan.RunInput {
	return plan.RunInput{Goal: goal, TraceID: "seed"}
}
