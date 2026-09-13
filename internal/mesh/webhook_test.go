package mesh

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func testPeers(t *testing.T, url, secret string) *PeerRegistry {
	t.Helper()
	peers := NewPeerRegistry()
	if _, err := peers.Register(Peer{ID: "coder-2", URL: url, Secret: secret, Capabilities: []string{"code"}}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	return peers
}

func TestSignatureRoundTrips(t *testing.T) {
	body := []byte(`{"hello":"crew"}`)
	ts := time.Now().Unix()
	sig := Sign("s3cret", ts, body)

	if err := VerifySignature("s3cret", sig, strconv.FormatInt(ts, 10), body, time.Now()); err != nil {
		t.Fatalf("a freshly signed body should verify: %v", err)
	}
}

func TestSignatureRejectsTamperingAndReplay(t *testing.T) {
	body := []byte(`{"task":"deploy"}`)
	ts := time.Now().Unix()
	sig := Sign("s3cret", ts, body)
	now := time.Now()

	cases := map[string]struct {
		secret, sig, ts string
		body            []byte
		when            time.Time
	}{
		"wrong secret":   {"other", sig, strconv.FormatInt(ts, 10), body, now},
		"tampered body":  {"s3cret", sig, strconv.FormatInt(ts, 10), []byte(`{"task":"rm -rf"}`), now},
		"missing secret": {"", sig, strconv.FormatInt(ts, 10), body, now},
		"no signature":   {"s3cret", "", strconv.FormatInt(ts, 10), body, now},
		"bad timestamp":  {"s3cret", sig, "not-a-number", body, now},
		"stale replay":   {"s3cret", sig, strconv.FormatInt(ts, 10), body, now.Add(10 * time.Minute)},
		"future skew":    {"s3cret", sig, strconv.FormatInt(ts, 10), body, now.Add(-10 * time.Minute)},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if err := VerifySignature(tc.secret, tc.sig, tc.ts, tc.body, tc.when); err == nil {
				t.Fatal("expected verification to fail")
			}
		})
	}
}

func TestSignatureIsBoundToItsTimestamp(t *testing.T) {
	body := []byte(`{"x":1}`)
	ts := time.Now().Unix()
	sig := Sign("s3cret", ts, body)

	// same body, a different timestamp inside the window: must not verify,
	// otherwise a captured body could be replayed with a fresh timestamp
	other := strconv.FormatInt(ts+30, 10)
	if err := VerifySignature("s3cret", sig, other, body, time.Now()); err == nil {
		t.Fatal("signature must cover the timestamp")
	}
}

func TestDeliveryLogSuppressesRetries(t *testing.T) {
	log := NewDeliveryLog(time.Minute)
	if log.Observe("d1") {
		t.Fatal("first sighting is not a duplicate")
	}
	if !log.Observe("d1") {
		t.Fatal("second sighting must be reported as a duplicate")
	}
	if log.Observe("d2") {
		t.Fatal("a different id is not a duplicate")
	}
	if log.Observe("") {
		t.Fatal("an empty id can never be a duplicate")
	}
}

func TestDeliverSignsAndSucceeds(t *testing.T) {
	var (
		mu      sync.Mutex
		gotBody []byte
		gotSig  string
		gotTS   string
		gotFrom string
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		gotBody = readAll(r)
		gotSig = r.Header.Get(HeaderSignature)
		gotTS = r.Header.Get(HeaderTimestamp)
		gotFrom = r.Header.Get(HeaderFrom)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	peers := testPeers(t, server.URL, "shared")
	transport := NewWebhookTransport(peers, "master-0")

	err := transport.Deliver(context.Background(), Message{
		Kind: MsgHandoff, From: "planner-1", To: "coder-2", TaskID: "t-9", Body: "write the patch",
	})
	if err != nil {
		t.Fatalf("deliver failed: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if err := VerifySignature("shared", gotSig, gotTS, gotBody, time.Now()); err != nil {
		t.Fatalf("the receiver could not verify what we sent: %v", err)
	}
	if gotFrom != "master-0" {
		t.Fatalf("unexpected From header %q", gotFrom)
	}

	var delivery Delivery
	if err := json.Unmarshal(gotBody, &delivery); err != nil {
		t.Fatalf("body was not a Delivery: %v", err)
	}
	if delivery.Message.TaskID != "t-9" || delivery.Message.Kind != MsgHandoff {
		t.Fatalf("unexpected message %+v", delivery.Message)
	}

	if peer, _ := peers.Get("coder-2"); peer.Failures != 0 {
		t.Fatal("a successful delivery should clear the failure streak")
	}
}

func TestDeliverRetriesThenSucceeds(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	transport := NewWebhookTransport(testPeers(t, server.URL, "shared"), "master-0")
	transport.Sleep = func(time.Duration) {} // do not wait out the backoff

	if err := transport.Deliver(context.Background(), Message{To: "coder-2", TaskID: "t-1"}); err != nil {
		t.Fatalf("expected the third attempt to succeed: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 attempts, got %d", calls)
	}
}

func TestDeliverDoesNotRetryPermanentRejections(t *testing.T) {
	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	transport := NewWebhookTransport(testPeers(t, server.URL, "wrong-secret"), "master-0")
	transport.Sleep = func(time.Duration) {}

	err := transport.Deliver(context.Background(), Message{To: "coder-2", TaskID: "t-1"})
	if err == nil {
		t.Fatal("expected a delivery error")
	}
	if calls != 1 {
		t.Fatalf("401 is permanent; expected 1 attempt, got %d", calls)
	}
}

func TestDeliverQuarantinesAPersistentlyDeadPeer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	peers := testPeers(t, server.URL, "shared")
	transport := NewWebhookTransport(peers, "master-0")
	transport.Sleep = func(time.Duration) {}

	for i := 0; i < quarantineAfter; i++ {
		_ = transport.Deliver(context.Background(), Message{To: "coder-2", TaskID: fmt.Sprintf("t-%d", i)})
	}

	peer, _ := peers.Get("coder-2")
	if !peer.Quarantined {
		t.Fatalf("expected quarantine after %d failures, got %d", quarantineAfter, peer.Failures)
	}
	if peer.Available() {
		t.Fatal("a quarantined peer must not be selectable for work")
	}
}

func TestDeliverToUnknownPeerFails(t *testing.T) {
	transport := NewWebhookTransport(NewPeerRegistry(), "master-0")
	if err := transport.Deliver(context.Background(), Message{To: "ghost"}); err == nil {
		t.Fatal("expected an error for an unregistered peer")
	}
}

func TestBroadcastSkipsSenderAndQuarantined(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := readAll(r)
		var d Delivery
		_ = json.Unmarshal(body, &d)
		mu.Lock()
		hits[d.Message.To]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	peers := NewPeerRegistry()
	for _, id := range []string{"planner-1", "coder-2", "reviewer-3"} {
		if _, err := peers.Register(Peer{ID: id, URL: server.URL, Secret: "s"}); err != nil {
			t.Fatal(err)
		}
	}
	peers.MarkFailed("reviewer-3")
	for i := 1; i < quarantineAfter; i++ {
		peers.MarkFailed("reviewer-3")
	}

	transport := NewWebhookTransport(peers, "master-0")
	transport.Sleep = func(time.Duration) {}
	errs := transport.Broadcast(context.Background(), Message{Kind: MsgBroadcast, From: "planner-1", To: "*"})
	if len(errs) != 0 {
		t.Fatalf("unexpected broadcast errors: %v", errs)
	}

	mu.Lock()
	defer mu.Unlock()
	if hits["planner-1"] != 0 {
		t.Fatal("the sender must not receive its own broadcast")
	}
	if hits["reviewer-3"] != 0 {
		t.Fatal("a quarantined peer must be skipped")
	}
	if hits["coder-2"] != 1 {
		t.Fatalf("expected exactly one delivery to coder-2, got %d", hits["coder-2"])
	}
}

func readAll(r *http.Request) []byte {
	buf := make([]byte, 0, 1024)
	tmp := make([]byte, 512)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return buf
}
