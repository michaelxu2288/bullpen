package mesh

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func brokerWithWorkers(t *testing.T, workers ...Node) (*Broker, *Bus) {
	t.Helper()
	bus := NewBus()
	registry := NewRegistry()
	for _, n := range workers {
		n.Role = RoleWorker
		registry.Join(n)
	}
	return NewBroker(bus, NewPeerRegistry(), registry, "master-0"), bus
}

func TestDelegationValidation(t *testing.T) {
	cases := map[string]Delegation{
		"no from":       {TaskID: "t", To: "b"},
		"no task":       {From: "a", To: "b"},
		"no target":     {From: "a", TaskID: "t"},
		"self delegate": {From: "a", To: "a", TaskID: "t"},
	}
	for name, d := range cases {
		t.Run(name, func(t *testing.T) {
			if err := d.Validate(); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}

	valid := Delegation{From: "a", To: "b", TaskID: "t"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected the delegation to be valid: %v", err)
	}
}

func TestDelegationRefusesACycle(t *testing.T) {
	d := Delegation{From: "reviewer-1", To: "coder-1", TaskID: "t-4", Chain: []string{"coder-1", "planner-1"}}
	err := d.Validate()
	if err == nil {
		t.Fatal("expected the cycle to be caught")
	}
	if !strings.Contains(err.Error(), "cycle") || !strings.Contains(err.Error(), "coder-1") {
		t.Fatalf("error should name the cycle and the repeat hop: %v", err)
	}
}

func TestDelegationRefusesRunawayDepth(t *testing.T) {
	chain := make([]string, MaxDelegationDepth)
	for i := range chain {
		chain[i] = "hop"
	}
	d := Delegation{From: "a", To: "b", TaskID: "t", Chain: chain}
	if err := d.Validate(); err == nil || !strings.Contains(err.Error(), "depth limit") {
		t.Fatalf("expected a depth-limit error, got %v", err)
	}
}

func TestEnvelopeCarriesTheChainForward(t *testing.T) {
	d := Delegation{
		From: "coder-1", To: "reviewer-1", TaskID: "t-7",
		Prompt: "check the blast radius", Reason: "needs a second pair of eyes",
		Chain: []string{"planner-1"},
	}
	m := d.Envelope("reviewer-1")

	if m.Kind != MsgHandoff || m.To != "reviewer-1" || m.From != "coder-1" {
		t.Fatalf("unexpected envelope %+v", m)
	}
	if m.Headers["delegation-chain"] != "planner-1,coder-1" {
		t.Fatalf("chain should append the sender, got %q", m.Headers["delegation-chain"])
	}
	if m.Headers["delegation-depth"] != "2" {
		t.Fatalf("unexpected depth %q", m.Headers["delegation-depth"])
	}
	if !strings.Contains(m.Body, "needs a second pair of eyes") {
		t.Fatal("the reason should travel with the prompt")
	}

	// the receiving agent must be able to continue the chain, not restart it
	if got := ChainFrom(m); len(got) != 2 || got[1] != "coder-1" {
		t.Fatalf("ChainFrom did not round-trip: %v", got)
	}
	if ChainFrom(Message{}) != nil {
		t.Fatal("a message with no chain header has no chain")
	}
}

func TestDelegateRoutesToALocalWorkerByCapability(t *testing.T) {
	broker, bus := brokerWithWorkers(t,
		Node{ID: "coder-1", Capabilities: []string{"code"}, MaxInFlight: 2},
		Node{ID: "reviewer-1", Capabilities: []string{"review"}, MaxInFlight: 2},
	)
	inbox := bus.Subscribe("reviewer-1")

	res, err := broker.Delegate(Delegation{
		From: "coder-1", Capability: "review", TaskID: "t-1", Prompt: "review this",
	})
	if err != nil {
		t.Fatalf("delegate failed: %v", err)
	}
	if res.To != "reviewer-1" || res.Remote {
		t.Fatalf("expected a local reviewer, got %+v", res)
	}
	if res.Depth != 1 || res.Chain[0] != "coder-1" {
		t.Fatalf("unexpected chain %+v", res.Chain)
	}

	select {
	case m := <-inbox:
		if m.TaskID != "t-1" || m.Kind != MsgHandoff {
			t.Fatalf("unexpected inbox message %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("the delegate never received the handoff")
	}
}

func TestDelegateNeverPicksTheSenderOrAPriorHop(t *testing.T) {
	broker, _ := brokerWithWorkers(t,
		Node{ID: "coder-1", Capabilities: []string{"code"}, MaxInFlight: 2},
	)

	// only coder-1 can code, and coder-1 is the sender
	if _, err := broker.Delegate(Delegation{From: "coder-1", Capability: "code", TaskID: "t-2"}); err == nil {
		t.Fatal("expected no available delegate")
	}

	// only coder-1 can code, and it already appears in the chain
	_, err := broker.Delegate(Delegation{
		From: "reviewer-1", Capability: "code", TaskID: "t-3", Chain: []string{"coder-1"},
	})
	if err == nil {
		t.Fatal("a prior hop must not be selected again")
	}
}

func TestDelegateRespectsCapacity(t *testing.T) {
	broker, _ := brokerWithWorkers(t,
		Node{ID: "reviewer-1", Capabilities: []string{"review"}, MaxInFlight: 1},
	)

	if _, err := broker.Delegate(Delegation{From: "coder-1", Capability: "review", TaskID: "t-1"}); err != nil {
		t.Fatalf("first delegation should succeed: %v", err)
	}
	if _, err := broker.Delegate(Delegation{From: "coder-1", Capability: "review", TaskID: "t-2"}); err == nil {
		t.Fatal("the reviewer is saturated; the second delegation should fail")
	}
}

func TestDelegateReachesARemotePeerOverWebhook(t *testing.T) {
	var (
		mu       sync.Mutex
		received Delivery
		done     = make(chan struct{})
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		_ = json.Unmarshal(readAll(r), &received)
		mu.Unlock()
		close(done)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	bus := NewBus()
	peers := NewPeerRegistry()
	if _, err := peers.Register(Peer{
		ID: "reviewer-remote", URL: server.URL, Secret: "s", Capabilities: []string{"review"}, MaxInFlight: 2,
	}); err != nil {
		t.Fatal(err)
	}
	transport := NewWebhookTransport(peers, "master-0")
	transport.Sleep = func(time.Duration) {}
	bus.AttachRemote(transport)

	broker := NewBroker(bus, peers, NewRegistry(), "master-0")
	res, err := broker.Delegate(Delegation{
		From: "coder-1", Capability: "review", TaskID: "t-remote", Prompt: "review the patch",
	})
	if err != nil {
		t.Fatalf("delegate failed: %v", err)
	}
	if !res.Remote || res.To != "reviewer-remote" {
		t.Fatalf("expected a remote delegate, got %+v", res)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the remote agent never received the webhook")
	}

	mu.Lock()
	defer mu.Unlock()
	if received.Message.TaskID != "t-remote" {
		t.Fatalf("unexpected delivered message %+v", received.Message)
	}
	if received.Message.Headers["delegation-chain"] != "coder-1" {
		t.Fatalf("the chain must survive the process hop, got %q", received.Message.Headers["delegation-chain"])
	}
}

func TestLocalWorkerIsPreferredOverARemotePeer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("a local worker was available; no webhook should have been sent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	bus := NewBus()
	registry := NewRegistry()
	registry.Join(Node{ID: "reviewer-local", Role: RoleWorker, Capabilities: []string{"review"}, MaxInFlight: 2})
	peers := NewPeerRegistry()
	_, _ = peers.Register(Peer{ID: "reviewer-remote", URL: server.URL, Secret: "s", Capabilities: []string{"review"}})

	broker := NewBroker(bus, peers, registry, "master-0")
	res, err := broker.Delegate(Delegation{From: "coder-1", Capability: "review", TaskID: "t-1"})
	if err != nil {
		t.Fatalf("delegate failed: %v", err)
	}
	if res.Remote || res.To != "reviewer-local" {
		t.Fatalf("in-process handoff should win, got %+v", res)
	}
}

func TestBusRoutesRemotelyOnlyWhenThereIsNoLocalInbox(t *testing.T) {
	var mu sync.Mutex
	var delivered []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var d Delivery
		_ = json.Unmarshal(readAll(r), &d)
		mu.Lock()
		delivered = append(delivered, d.Message.To)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	bus := NewBus()
	peers := NewPeerRegistry()
	_, _ = peers.Register(Peer{ID: "local-and-remote", URL: server.URL, Secret: "s"})
	_, _ = peers.Register(Peer{ID: "remote-only", URL: server.URL, Secret: "s"})
	transport := NewWebhookTransport(peers, "master-0")
	transport.Sleep = func(time.Duration) {}
	bus.AttachRemote(transport)

	// this id has a local inbox, so it must not also go out over the wire
	inbox := bus.Subscribe("local-and-remote")
	bus.Publish(Message{Kind: MsgAssign, To: "local-and-remote", TaskID: "t-local"})
	select {
	case <-inbox:
	case <-time.After(time.Second):
		t.Fatal("local delivery failed")
	}

	bus.Publish(Message{Kind: MsgAssign, To: "remote-only", TaskID: "t-remote"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		n := len(delivered)
		mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(delivered) != 1 || delivered[0] != "remote-only" {
		t.Fatalf("expected exactly one remote delivery to remote-only, got %v", delivered)
	}
}

func TestBusRemoteErrorsSurfaceThroughTheHook(t *testing.T) {
	bus := NewBus()
	peers := NewPeerRegistry()
	_, _ = peers.Register(Peer{ID: "dead", URL: "http://127.0.0.1:1", Secret: "s"})
	transport := NewWebhookTransport(peers, "master-0")
	transport.Sleep = func(time.Duration) {}
	transport.Retry = RetryPolicy{Attempts: 1}
	bus.AttachRemote(transport)

	got := make(chan error, 1)
	bus.OnRemoteError = func(err error) {
		select {
		case got <- err:
		default:
		}
	}

	bus.Publish(Message{Kind: MsgAssign, To: "dead", TaskID: "t"})
	select {
	case err := <-got:
		if err == nil {
			t.Fatal("expected a delivery error")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a failed remote delivery should not be silent")
	}
}

var _ RemoteTransport = (*WebhookTransport)(nil)

var _ = context.Background
