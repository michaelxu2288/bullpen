package plan

import (
	"sort"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

type Scheduler struct {
	MaxConcurrent int
}

func (s Scheduler) Order(tasks []domain.Task) []domain.Task {
	out := make([]domain.Task, len(tasks))
	copy(out, tasks)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Priority == out[j].Priority {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].Priority < out[j].Priority
	})
	return out
}

func (s Scheduler) Batch(tasks []domain.Task) [][]domain.Task {
	if s.MaxConcurrent <= 0 {
		s.MaxConcurrent = 1
	}
	ordered := s.Order(tasks)
	batches := make([][]domain.Task, 0)
	for i := 0; i < len(ordered); i += s.MaxConcurrent {
		j := i + s.MaxConcurrent
		if j > len(ordered) {
			j = len(ordered)
		}
		batches = append(batches, ordered[i:j])
	}
	return batches
}
