package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/michaelxu2288/bullpen/internal/mesh"
)

func post(t *testing.T, mux *http.ServeMux, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func registerPeer(t *testing.T, mux *http.ServeMux, id, url, caps string) registerResponse {
	t.Helper()
	body := `{"id":"` + id + `","url":"` + url + `","capabilities":[` + caps + `],"max_in_flight":2}`
	res := post(t, mux, "/v1/agents/register", body)
	if res.Code != http.StatusOK {
		t.Fatalf("register %s failed: %d %s", id, res.Code, res.Body.String())
	}
	var out registerResponse
	if err := json.Unmarshal(res.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	return out
}

func TestRegisterMintsASecretAndListingNeverLeaksIt(t *testing.T) {
	_, mux := testHandlers(t)
	reg := registerPeer(t, mux, "coder-remote", "http://127.0.0.1:9/inbox", `"code"`)

	if len(reg.Secret) != 64 {
		t.Fatalf("expected a 32-byte hex secret, got %d chars", len(reg.Secret))
	}
	if reg.Peer.ID != "coder-remote" {
		t.Fatalf("unexpected peer %+v", reg.Peer)
	}

	res := httptest.NewRecorder()
	mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/v1/agents", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("list failed: %d", res.Code)
	}
	if strings.Contains(res.Body.String(), reg.Secret) {
		t.Fatal("the peer listing must never contain a shared secret")
	}
	if !strings.Contains(res.Body.String(), "coder-remote") {
		t.Fatal("the peer should be listed")
	}
}

func TestRegisterRejectsIncompletePeers(t *testing.T) {
	_, mux := testHandlers(t)
	if res := post(t, mux, "/v1/agents/register", `{"url":"http://x"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("an id is required, got %d", res.Code)
	}
	if res := post(t, mux, "/v1/agents/register", `{"id":"a"}`); res.Code != http.StatusBadRequest {
		t.Fatalf("a url is required, got %d", res.Code)
	}
}

func TestReregisterRotatesTheSecretAndKeepsTheInFlightCount(t *testing.T) {
	h, mux := testHandlers(t)
	first := registerPeer(t, mux, "coder-remote", "http://127.0.0.1:9/inbox", `"code"`)
	h.Mesh.Peers.Reserve("coder-remote")

	res := post(t, mux, "/v1/agents/register", `{"id":"coder-remote","url":"http://127.0.0.1:10/inbox"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("re-register failed: %d", res.Code)
	}
	var second registerResponse
	_ = json.Unmarshal(res.Body.Bytes(), &second)

	// Re-registration must never hand back the old key: that would make this
	// endpoint a disclosure oracle for anyone who can guess an agent id.
	if second.Secret == first.Secret {
		t.Fatal("re-registering without a secret must rotate, not disclose")
	}

	peer, _ := h.Mesh.Peers.Get("coder-remote")
	if peer.Secret != second.Secret {
		t.Fatal("the registry should hold the rotated secret")
	}
	if peer.InFlight != 1 {
		t.Fatalf("in-flight accounting should survive a restart, got %d", peer.InFlight)
	}
	if peer.URL != "http://127.0.0.1:10/inbox" {
		t.Fatalf("the url should be updated, got %s", peer.URL)
	}
}

func TestReregisterKeepsASuppliedSecret(t *testing.T) {
	h, mux := testHandlers(t)
	registerPeer(t, mux, "coder-remote", "http://127.0.0.1:9/inbox", `"code"`)

	res := post(t, mux, "/v1/agents/register",
		`{"id":"coder-remote","url":"http://127.0.0.1:9/inbox","secret":"pinned-by-the-agent"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("re-register failed: %d", res.Code)
	}
	peer, _ := h.Mesh.Peers.Get("coder-remote")
	if peer.Secret != "pinned-by-the-agent" {
		t.Fatalf("an agent that supplies its key should keep it, got %q", peer.Secret)
	}
}

// signedInbox posts a delivery the way a real peer would.
func signedInbox(t *testing.T, mux *http.ServeMux, from, secret string, delivery mesh.Delivery, ts int64) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(delivery)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/inbox", bytes.NewReader(body))
	req.Header.Set(mesh.HeaderFrom, from)
	req.Header.Set(mesh.HeaderTimestamp, strconv.FormatInt(ts, 10))
	req.Header.Set(mesh.HeaderSignature, mesh.Sign(secret, ts, body))
	req.Header.Set(mesh.HeaderDelivery, delivery.ID)
	res := httptest.NewRecorder()
	mux.ServeHTTP(res, req)
	return res
}

func TestInboxAcceptsASignedDeliveryAndPutsItOnTheBus(t *testing.T) {
	h, mux := testHandlers(t)
	reg := registerPeer(t, mux, "planner-remote", "http://127.0.0.1:9/inbox", `"plan"`)

	inbox := h.Mesh.Bus.Subscribe("coder-local")
	delivery := mesh.Delivery{
		ID: "d-1",
		Message: mesh.Message{
			Kind: mesh.MsgHandoff, From: "planner-remote", To: "coder-local",
			TaskID: "t-1", Body: "implement the retry fix",
		},
	}

	res := signedInbox(t, mux, "planner-remote", reg.Secret, delivery, time.Now().Unix())
	if res.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", res.Code, res.Body.String())
	}

	select {
	case m := <-inbox:
		if m.TaskID != "t-1" || m.From != "planner-remote" {
			t.Fatalf("unexpected message on the bus: %+v", m)
		}
	case <-time.After(time.Second):
		t.Fatal("the delivery never reached the local bus")
	}
}

func TestInboxRejectsForgedAndUnknownSenders(t *testing.T) {
	_, mux := testHandlers(t)
	reg := registerPeer(t, mux, "planner-remote", "http://127.0.0.1:9/inbox", `"plan"`)
	delivery := mesh.Delivery{ID: "d-2", Message: mesh.Message{To: "coder-local", TaskID: "t"}}

	if res := signedInbox(t, mux, "planner-remote", "wrong-secret", delivery, time.Now().Unix()); res.Code != http.StatusUnauthorized {
		t.Fatalf("a bad signature must be rejected, got %d", res.Code)
	}
	if res := signedInbox(t, mux, "ghost-agent", reg.Secret, delivery, time.Now().Unix()); res.Code != http.StatusUnauthorized {
		t.Fatalf("an unregistered sender must be rejected, got %d", res.Code)
	}

	stale := time.Now().Add(-30 * time.Minute).Unix()
	if res := signedInbox(t, mux, "planner-remote", reg.Secret, delivery, stale); res.Code != http.StatusUnauthorized {
		t.Fatalf("a replayed delivery must be rejected, got %d", res.Code)
	}
}

func TestInboxIsIdempotentAcrossRetries(t *testing.T) {
	h, mux := testHandlers(t)
	reg := registerPeer(t, mux, "planner-remote", "http://127.0.0.1:9/inbox", `"plan"`)
	inbox := h.Mesh.Bus.Subscribe("coder-local")

	delivery := mesh.Delivery{
		ID:      "d-retry",
		Message: mesh.Message{Kind: mesh.MsgHandoff, To: "coder-local", TaskID: "t-9"},
	}

	first := signedInbox(t, mux, "planner-remote", reg.Secret, delivery, time.Now().Unix())
	second := signedInbox(t, mux, "planner-remote", reg.Secret, delivery, time.Now().Unix())

	if first.Code != http.StatusAccepted {
		t.Fatalf("first delivery should be accepted, got %d", first.Code)
	}
	// a duplicate must be acknowledged, not errored, or the sender retries forever
	if second.Code != http.StatusOK || !strings.Contains(second.Body.String(), "duplicate") {
		t.Fatalf("duplicate should be acknowledged as such, got %d %s", second.Code, second.Body.String())
	}

	<-inbox
	select {
	case m := <-inbox:
		t.Fatalf("the message was published twice: %+v", m)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestDelegateEndpointRoutesByCapability(t *testing.T) {
	h, mux := testHandlers(t)
	h.Mesh.Registry.Join(mesh.Node{
		ID: "reviewer-local", Role: mesh.RoleWorker, Capabilities: []string{"review"}, MaxInFlight: 2,
	})

	res := post(t, mux, "/v1/agents/delegate",
		`{"from":"coder-1","capability":"review","task_id":"t-1","prompt":"check this","reason":"needs review"}`)
	if res.Code != http.StatusOK {
		t.Fatalf("delegate failed: %d %s", res.Code, res.Body.String())
	}

	var result mesh.DelegationResult
	_ = json.Unmarshal(res.Body.Bytes(), &result)
	if result.To != "reviewer-local" || result.Depth != 1 {
		t.Fatalf("unexpected result %+v", result)
	}

	var found bool
	for _, ev := range h.Engine.Events.History() {
		if ev.Actor == "coder-1" && ev.Target == "reviewer-local" {
			found = true
		}
	}
	if !found {
		t.Fatal("a delegation should show up on the event timeline")
	}
}

func TestDelegateEndpointRefusesCyclesWithAReadableError(t *testing.T) {
	_, mux := testHandlers(t)
	res := post(t, mux, "/v1/agents/delegate",
		`{"from":"reviewer-1","to":"coder-1","task_id":"t-2","chain":["coder-1","planner-1"]}`)
	if res.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", res.Code)
	}
	if !strings.Contains(res.Body.String(), "cycle") {
		t.Fatalf("the error should name the cycle: %s", res.Body.String())
	}
}

func TestDeregisterRemovesAPeer(t *testing.T) {
	h, mux := testHandlers(t)
	registerPeer(t, mux, "coder-remote", "http://127.0.0.1:9/inbox", `"code"`)

	if res := post(t, mux, "/v1/agents/deregister", `{"id":"coder-remote"}`); res.Code != http.StatusOK {
		t.Fatalf("deregister failed: %d", res.Code)
	}
	if h.Mesh.Peers.Known("coder-remote") {
		t.Fatal("the peer should be gone")
	}
	if res := post(t, mux, "/v1/agents/deregister", `{"id":"coder-remote"}`); res.Code != http.StatusNotFound {
		t.Fatalf("removing an unknown peer should 404, got %d", res.Code)
	}
}

func TestMeshEndpointsRejectGetWhereTheyShould(t *testing.T) {
	_, mux := testHandlers(t)
	for _, path := range []string{"/v1/agents/register", "/v1/agents/deregister", "/v1/agents/inbox", "/v1/agents/delegate"} {
		res := httptest.NewRecorder()
		mux.ServeHTTP(res, httptest.NewRequest(http.MethodGet, path, nil))
		if res.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s should reject GET, got %d", path, res.Code)
		}
	}
}
