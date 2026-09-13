package plan

import (
	"fmt"
	"strings"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

type Planner struct{}

func (p Planner) DecomposeGoal(goal string) []domain.Task {
	parts := []string{"scaffold", "implement", "review", "handoff"}
	if strings.Contains(strings.ToLower(goal), "rag") {
		parts = append(parts, "retrieval")
	}

	out := make([]domain.Task, 0, len(parts))
	for i, part := range parts {
		t := domain.NewTask(fmt.Sprintf("task-%d", i+1), strings.Title(part), goal+" :: "+part)
		t.Priority = i + 1
		t.Labels = []string{"auto-plan", part}
		out = append(out, t)
	}
	return out
}
