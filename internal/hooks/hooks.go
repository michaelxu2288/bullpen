package hooks

import "fmt"

type Mode string

const (
	ModeSummary Mode = "summary"
	ModeBugfix  Mode = "bugfix"
	ModeReview  Mode = "review"
)

func BuildHandoffPrompt(mode Mode, sourceAgent string, payload string) string {
	if payload == "" {
		return ""
	}

	switch mode {
	case ModeBugfix:
		return fmt.Sprintf("[handoff:%s:bugfix]\nFind and fix the issue described below. Return patch + validation steps.\n\n%s", sourceAgent, payload)
	case ModeReview:
		return fmt.Sprintf("[handoff:%s:review]\nReview the proposal below. Call out risks/regressions and list blockers first.\n\n%s", sourceAgent, payload)
	default:
		return fmt.Sprintf("[handoff:%s:summary]\nUse this context to continue the task. Summarize assumptions and next commands before coding.\n\n%s", sourceAgent, payload)
	}
}
