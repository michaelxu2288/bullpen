package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/mesh"
)

// maxDeliveryBytes bounds an inbound webhook body. Signature verification has
// to read the whole body first, so an unbounded read is a memory DoS.
const maxDeliveryBytes = 1 << 20

// MeshRouter is the webhook fabric: who is reachable, how to reach them, and how
// work moves between them.
type MeshRouter struct {
	Peers    *mesh.PeerRegistry
	Bus      *mesh.Bus
	Broker   *mesh.Broker
	Log      *mesh.DeliveryLog
	SelfID   string
	Registry *mesh.Registry
}

// NewMeshRouter wires an in-process bus to a webhook transport, so a message
// addressed to any agent id reaches it whether it is local or remote.
func NewMeshRouter(selfID string) *MeshRouter {
	if selfID == "" {
		selfID = "master-0"
	}
	peers := mesh.NewPeerRegistry()
	bus := mesh.NewBus()
	registry := mesh.NewRegistry()

	transport := mesh.NewWebhookTransport(peers, selfID)
	bus.AttachRemote(transport)

	return &MeshRouter{
		Peers:    peers,
		Bus:      bus,
		Broker:   mesh.NewBroker(bus, peers, registry, selfID),
		Log:      mesh.NewDeliveryLog(10 * time.Minute),
		SelfID:   selfID,
		Registry: registry,
	}
}

type registerRequest struct {
	ID           string            `json:"id"`
	URL          string            `json:"url"`
	Capabilities []string          `json:"capabilities"`
	MaxInFlight  int               `json:"max_in_flight"`
	Labels       map[string]string `json:"labels"`
	// Secret lets an agent supply its own shared key. Omit it and one is minted.
	Secret string `json:"secret,omitempty"`
}

type registerResponse struct {
	Peer mesh.Peer `json:"peer"`
	// Secret is returned only here, on registration. It is never listed again.
	Secret string `json:"secret"`
	Inbox  string `json:"inbox"`
}

// registerAgent enrolls a webhook-reachable agent into the mesh.
func (h *Handlers) registerAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	// A caller that supplies no secret gets a freshly minted one, including on
	// re-registration. Returning the *existing* secret would turn this endpoint
	// into a disclosure oracle: anyone who guesses an agent id could read its
	// key. Rotation is the safe default, and a restarting agent that wants to
	// keep its old key simply sends it.
	secret := req.Secret
	if secret == "" {
		generated, err := mesh.NewSecret()
		if err != nil {
			respondJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		secret = generated
	}

	peer, err := h.Mesh.Peers.Register(mesh.Peer{
		ID:           req.ID,
		URL:          req.URL,
		Secret:       secret,
		Capabilities: req.Capabilities,
		MaxInFlight:  req.MaxInFlight,
		Labels:       req.Labels,
	})
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	h.Mesh.Bus.Publish(mesh.Message{
		Kind: mesh.MsgGossip, From: h.Mesh.SelfID, To: "*",
		Body: fmt.Sprintf("agent %s joined the mesh caps=%v", peer.ID, peer.Capabilities),
	})
	h.Engine.Events.Publish(domain.Event{
		ID:        fmt.Sprintf("mesh-%d", time.Now().UnixNano()),
		Type:      domain.EventSessionLaunched,
		Actor:     peer.ID,
		Target:    peer.URL,
		CreatedAt: time.Now(),
	})

	respondJSON(w, http.StatusOK, registerResponse{Peer: peer, Secret: secret, Inbox: "/v1/agents/inbox"})
}

func (h *Handlers) deregisterAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if !h.Mesh.Peers.Remove(req.ID) {
		respondJSON(w, http.StatusNotFound, map[string]any{"error": "no such agent: " + req.ID})
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"removed": req.ID})
}

func (h *Handlers) listAgents(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]any{
		"self":  h.Mesh.SelfID,
		"peers": h.Mesh.Peers.List(),
	})
}

// agentInbox receives a signed webhook from a peer agent and republishes it on
// the local bus, which is what makes cross-process addressing transparent.
func (h *Handlers) agentInbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, maxDeliveryBytes))
	if err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": "unreadable body"})
		return
	}

	from := r.Header.Get(mesh.HeaderFrom)
	sender, ok := h.Mesh.Peers.Get(from)
	if !ok {
		// Do not distinguish "unknown sender" from "bad signature" in the status:
		// both are 401, so probing cannot enumerate registered agent ids.
		respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized delivery"})
		return
	}

	if err := mesh.VerifySignature(
		sender.Secret,
		r.Header.Get(mesh.HeaderSignature),
		r.Header.Get(mesh.HeaderTimestamp),
		body,
		time.Now(),
	); err != nil {
		respondJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized delivery"})
		return
	}

	var delivery mesh.Delivery
	if err := json.Unmarshal(body, &delivery); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": "malformed delivery"})
		return
	}

	// Retries are expected, so the inbox has to be idempotent. Acknowledge the
	// duplicate rather than erroring, or the sender keeps retrying forever.
	if h.Mesh.Log.Observe(delivery.ID) {
		respondJSON(w, http.StatusOK, map[string]any{"status": "duplicate", "delivery": delivery.ID})
		return
	}

	h.Mesh.Peers.MarkDelivered(from)

	message := delivery.Message
	if message.From == "" {
		message.From = from
	}
	h.Mesh.Bus.Publish(message)

	h.Engine.Events.Publish(domain.Event{
		ID:        fmt.Sprintf("inbox-%d", time.Now().UnixNano()),
		Type:      domain.EventPromptPiped,
		Actor:     message.From,
		Target:    message.To,
		Payload:   map[string]any{"kind": string(message.Kind), "task_id": message.TaskID},
		CreatedAt: time.Now(),
	})

	respondJSON(w, http.StatusAccepted, map[string]any{"status": "accepted", "delivery": delivery.ID})
}

// delegateTask is how one agent hands work to another, by name or by capability.
func (h *Handlers) delegateTask(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var req mesh.Delegation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	result, err := h.Mesh.Broker.Delegate(req)
	if err != nil {
		respondJSON(w, http.StatusConflict, map[string]any{"error": err.Error()})
		return
	}

	h.Engine.Events.Publish(domain.Event{
		ID:        fmt.Sprintf("delegate-%d", time.Now().UnixNano()),
		Type:      domain.EventTaskAssigned,
		Actor:     result.From,
		Target:    result.To,
		Payload:   map[string]any{"task_id": result.TaskID, "depth": result.Depth, "remote": result.Remote},
		CreatedAt: time.Now(),
	})

	respondJSON(w, http.StatusOK, result)
}

// meshMessages is the wire tail, for the dashboard and for debugging.
func (h *Handlers) meshMessages(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, h.Mesh.Bus.Tail(100))
}
