package kanban

import (
	"testing"

	"github.com/michaelxu2288/bullpen/internal/domain"
)

func TestAdvanceMovesAcrossColumns(t *testing.T) {
	m := NewModel([]Card{{ID: "1", Title: "a", Owner: "w1", State: domain.TaskQueued}})
	m.advance()
	if m.cards[0].State != domain.TaskRunning {
		t.Fatalf("queued->running, got %s", m.cards[0].State)
	}
	m.col = 1
	m.advance()
	m.col = 2
	m.advance()
	if m.cards[0].State != domain.TaskDone {
		t.Fatalf("expected done, got %s", m.cards[0].State)
	}
	m.col = 3
	m.advance() // terminal column: no-op
	if m.cards[0].State != domain.TaskDone {
		t.Fatalf("done is terminal, got %s", m.cards[0].State)
	}
}

func TestColumnPartition(t *testing.T) {
	m := NewModel([]Card{
		{ID: "1", State: domain.TaskQueued},
		{ID: "2", State: domain.TaskRunning},
		{ID: "3", State: domain.TaskQueued},
	})
	if got := len(m.column(domain.TaskQueued)); got != 2 {
		t.Fatalf("want 2 queued, got %d", got)
	}
}
