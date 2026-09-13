package mesh

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

func collect() (ProgressFunc, func() []string) {
	var mu sync.Mutex
	var notes []string
	report := func(_ float64, note string) {
		mu.Lock()
		notes = append(notes, note)
		mu.Unlock()
	}
	return report, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), notes...)
	}
}

func TestSimulatedHandlerWalksTheStagesAndCompletes(t *testing.T) {
	handler := SimulatedHandler(SimOptions{MinDuration: 20 * time.Millisecond, MaxDuration: 40 * time.Millisecond, Seed: 7})
	report, notes := collect()

	out, err := handler(context.Background(), Message{
		TaskID: "t-1", To: "w1", Body: "implement the retry fix", Headers: map[string]string{"capability": "code"},
	}, report)
	if err != nil {
		t.Fatalf("expected completion: %v", err)
	}
	if !strings.HasPrefix(out, "code complete") {
		t.Fatalf("unexpected result %q", out)
	}

	got := notes()
	if got[0] != "retrieving context" || got[len(got)-1] != "done" {
		t.Fatalf("stages should run first to last, got %v", got)
	}
}

func TestSimulatedHandlerUsesCapabilityStages(t *testing.T) {
	handler := SimulatedHandler(SimOptions{MinDuration: 10 * time.Millisecond, MaxDuration: 10 * time.Millisecond, Seed: 1})
	report, notes := collect()
	_, _ = handler(context.Background(), Message{TaskID: "t", To: "w", Headers: map[string]string{"capability": "review"}}, report)
	if notes()[0] != "retrieving the incident thread" {
		t.Fatalf("review tasks should narrate review stages, got %v", notes())
	}
}

func TestSimulatedHandlerIsReproducibleForASeed(t *testing.T) {
	opts := SimOptions{MinDuration: 5 * time.Millisecond, MaxDuration: 50 * time.Millisecond, FailRate: 0.5, Seed: 99}
	run := func() (string, error) {
		report, _ := collect()
		return SimulatedHandler(opts)(context.Background(), Message{TaskID: "t-3", To: "w1"}, report)
	}
	a, errA := run()
	b, errB := run()
	if a != b || (errA == nil) != (errB == nil) {
		t.Fatalf("same seed and task should behave identically: %q/%v vs %q/%v", a, errA, b, errB)
	}
}

func TestSimulatedHandlerCanFail(t *testing.T) {
	// with FailRate 1 every task escalates, past the first stage
	handler := SimulatedHandler(SimOptions{MinDuration: 5 * time.Millisecond, MaxDuration: 5 * time.Millisecond, FailRate: 1, Seed: 3})
	report, notes := collect()
	_, err := handler(context.Background(), Message{TaskID: "t", To: "w", Headers: map[string]string{"capability": "code"}}, report)
	if err == nil {
		t.Fatal("expected an escalation")
	}
	if len(notes()) < 2 {
		t.Fatalf("a failure should happen while visibly in flight, got %v", notes())
	}
}

func TestSimulatedHandlerHonoursCancellation(t *testing.T) {
	handler := SimulatedHandler(SimOptions{MinDuration: time.Minute, MaxDuration: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	report, _ := collect()

	done := make(chan error, 1)
	go func() {
		_, err := handler(ctx, Message{TaskID: "t", To: "w"}, report)
		done <- err
	}()
	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected ctx error")
		}
	case <-time.After(time.Second):
		t.Fatal("handler ignored cancellation")
	}
}

func TestWorkerReportsProgressAndRunsConcurrently(t *testing.T) {
	m := NewMaster("m0")
	tap, stop := m.Bus.Tap()
	defer stop()

	w := NewWorker(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 2}, m.Bus)
	w.HandleWithProgress = func(ctx context.Context, msg Message, report ProgressFunc) (string, error) {
		report(0.5, "halfway")
		time.Sleep(60 * time.Millisecond)
		return "done " + msg.TaskID, nil
	}
	m.Register(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 2})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go w.Run(ctx, 20*time.Millisecond, "m0")

	started := time.Now()
	for _, id := range []string{"t1", "t2"} {
		if _, err := m.Dispatch(WorkItem{TaskID: id, Capability: "code"}); err != nil {
			t.Fatalf("dispatch %s: %v", id, err)
		}
	}

	var progress, results int
	deadline := time.After(800 * time.Millisecond)
	for results < 2 {
		select {
		case msg := <-tap:
			switch msg.Kind {
			case MsgProgress:
				if msg.Headers["progress"] != "0.500" || msg.Body != "halfway" {
					t.Fatalf("unexpected progress message %+v", msg)
				}
				progress++
			case MsgResult:
				results++
			}
		case <-deadline:
			t.Fatalf("timed out: progress=%d results=%d", progress, results)
		}
	}
	if progress != 2 {
		t.Fatalf("expected a progress report per task, got %d", progress)
	}
	// two 60ms tasks on a 2-slot worker must overlap, not serialize
	if elapsed := time.Since(started); elapsed > 110*time.Millisecond {
		t.Fatalf("tasks ran serially: %s", elapsed)
	}
}

func TestKilledWorkerGoesSilent(t *testing.T) {
	m := NewMaster("m0")
	tap, stop := m.Bus.Tap()
	defer stop()

	w := NewWorker(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 1}, m.Bus)
	w.HandleWithProgress = func(ctx context.Context, _ Message, _ ProgressFunc) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}
	m.Register(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 1})

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx, 10*time.Millisecond, "m0")
	if _, err := m.Dispatch(WorkItem{TaskID: "t1", Capability: "code"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	cancel()

	deadline := time.After(300 * time.Millisecond)
	for {
		select {
		case msg := <-tap:
			if msg.Kind == MsgEscalate || msg.Kind == MsgResult {
				t.Fatalf("a killed worker must not report, saw %+v", msg)
			}
		case <-deadline:
			return
		}
	}
}
