package plan

import "testing"

func TestDecomposeGoal(t *testing.T) {
	p := Planner{}
	tasks := p.DecomposeGoal("build rag incident workflow")
	if len(tasks) < 4 {
		t.Fatalf("expected at least 4 tasks, got %d", len(tasks))
	}
	foundRetrieval := false
	for _, task := range tasks {
		if task.Title == "Retrieval" || task.Title == "retrieval" {
			foundRetrieval = true
		}
	}
	if !foundRetrieval {
		t.Fatalf("expected retrieval step when goal contains rag")
	}
}
