package plan

import (
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

type Supervisor struct {
	StaleAfter time.Duration
}

func (s Supervisor) DetectStaleTasks(tasks []domain.Task) []domain.Task {
	if s.StaleAfter <= 0 {
		s.StaleAfter = 30 * time.Minute
	}
	now := time.Now()
	stale := make([]domain.Task, 0)
	for _, t := range tasks {
		if t.State == domain.TaskDone || t.State == domain.TaskFailed {
			continue
		}
		if now.Sub(t.UpdatedAt) > s.StaleAfter {
			stale = append(stale, t)
		}
	}
	return stale
}

func (s Supervisor) EscalationMessage(task domain.Task) string {
	return fmt.Sprintf("task %s (%s) stale in state %s; escalating to reviewer", task.ID, task.Title, task.State)
}
