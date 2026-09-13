package mesh

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestLeastLoadedPicksFreest(t *testing.T) {
	r := NewRegistry()
	r.Join(Node{ID: "w1", Role: RoleWorker, Capabilities: []string{"code"}, MaxInFlight: 4, InFlight: 3})
	r.Join(Node{ID: "w2", Role: RoleWorker, Capabilities: []string{"code"}, MaxInFlight: 4, InFlight: 1})
	got, ok := r.LeastLoaded("code")
	if !ok || got != "w2" {
		t.Fatalf("want w2, got %q ok=%v", got, ok)
	}
}

func TestCapabilityFilter(t *testing.T) {
	r := NewRegistry()
	r.Join(Node{ID: "w1", Role: RoleWorker, Capabilities: []string{"review"}, MaxInFlight: 2})
	if _, ok := r.LeastLoaded("code"); ok {
		t.Fatal("expected no worker for missing capability")
	}
}

func TestReapMarksDead(t *testing.T) {
	r := NewRegistry()
	r.Join(Node{ID: "w1", Role: RoleWorker, MaxInFlight: 2})
	r.nodes["w1"].LastHeartbeat = time.Now().Add(-time.Hour)
	dead := r.Reap(time.Minute, 2*time.Minute)
	if len(dead) != 1 || dead[0] != "w1" {
		t.Fatalf("want w1 dead, got %v", dead)
	}
}

func TestMasterDispatchAndComplete(t *testing.T) {
	m := NewMaster("m0")
	m.Register(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 1})
	node, err := m.Dispatch(WorkItem{TaskID: "t1", Capability: "code", Prompt: "build"})
	if err != nil || node != "w1" {
		t.Fatalf("dispatch: node=%q err=%v", node, err)
	}
	if _, err := m.Dispatch(WorkItem{TaskID: "t2", Capability: "code"}); err == nil {
		t.Fatal("expected saturation error")
	}
	m.Complete("t1")
	if _, err := m.Dispatch(WorkItem{TaskID: "t3", Capability: "code"}); err != nil {
		t.Fatalf("after complete: %v", err)
	}
}

func TestBusBroadcastAndDirect(t *testing.T) {
	b := NewBus()
	a := b.Subscribe("a")
	c := b.Subscribe("c")
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-a }()
	go func() { defer wg.Done(); <-c }()
	b.Publish(Message{Kind: MsgBroadcast, To: "*", Body: "hi"})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("broadcast not delivered to both inboxes")
	}
}

func TestWorkerHandlesAssignment(t *testing.T) {
	m := NewMaster("m0")
	bus := m.Bus
	w := NewWorker(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 1}, bus)
	w.Handle = func(_ context.Context, msg Message) (string, error) { return "ok:" + msg.TaskID, nil }
	m.Register(Node{ID: "w1", Capabilities: []string{"code"}, MaxInFlight: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	go w.Run(ctx, 50*time.Millisecond, "m0")
	if _, err := m.Dispatch(WorkItem{TaskID: "t1", Capability: "code", Prompt: "go"}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(150 * time.Millisecond)
	saw := false
	for _, msg := range bus.Tail(50) {
		if msg.Kind == MsgResult && msg.Body == "ok:t1" {
			saw = true
		}
	}
	if !saw {
		t.Fatal("worker result not seen on bus")
	}
}
