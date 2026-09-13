package plan

import (
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

func TestBatching(t *testing.T) {
	now := time.Now()
	tasks := []domain.Task{
		{ID: "a", Priority: 3, CreatedAt: now},
		{ID: "b", Priority: 1, CreatedAt: now.Add(time.Second)},
		{ID: "c", Priority: 2, CreatedAt: now.Add(2 * time.Second)},
		{ID: "d", Priority: 4, CreatedAt: now.Add(3 * time.Second)},
	}
	s := Scheduler{MaxConcurrent: 2}
	batches := s.Batch(tasks)
	if len(batches) != 2 {
		t.Fatalf("expected 2 batches, got %d", len(batches))
	}
	if batches[0][0].ID != "b" {
		t.Fatalf("expected highest priority task first, got %s", batches[0][0].ID)
	}
}
