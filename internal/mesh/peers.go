package mesh

import (
	"fmt"
	"sync"
	"time"
)

// Peer is a crew member reachable over HTTP rather than over the in-process
// bus. Every agent that runs in its own process registers one of these.
type Peer struct {
	ID string `json:"id"`
	// URL is the peer's inbox endpoint, which receives signed deliveries.
	URL string `json:"url"`
	// Secret signs outbound deliveries to this peer. Never serialized: it must
	// not leak through /v1/agents or the dashboard.
	Secret       string            `json:"-"`
	Capabilities []string          `json:"capabilities"`
	Labels       map[string]string `json:"labels,omitempty"`
	MaxInFlight  int               `json:"max_in_flight"`
	InFlight     int               `json:"in_flight"`
	RegisteredAt time.Time         `json:"registered_at"`
	LastSeen     time.Time         `json:"last_seen"`
	// Failures counts consecutive delivery failures; a peer is quarantined once
	// it crosses the threshold so a dead endpoint stops costing every publish.
	Failures    int  `json:"failures"`
	Quarantined bool `json:"quarantined"`
}

// HasCapability reports whether the peer advertises a capability.
func (p Peer) HasCapability(want string) bool {
	if want == "" {
		return true
	}
	return hasCap(p.Capabilities, want)
}

// Available reports whether the peer can take more work right now.
func (p Peer) Available() bool {
	if p.Quarantined {
		return false
	}
	return p.MaxInFlight <= 0 || p.InFlight < p.MaxInFlight
}

// quarantineAfter is how many consecutive failures take a peer out of rotation.
const quarantineAfter = 5

// PeerRegistry is the master's directory of webhook-reachable agents.
type PeerRegistry struct {
	mu    sync.RWMutex
	peers map[string]*Peer
}

func NewPeerRegistry() *PeerRegistry {
	return &PeerRegistry{peers: make(map[string]*Peer)}
}

// Register adds or refreshes a peer. Re-registering an existing id keeps its
// in-flight count (an agent that restarts should not lose accounting) but clears
// any quarantine, since a fresh registration is evidence it is back.
func (r *PeerRegistry) Register(p Peer) (Peer, error) {
	if p.ID == "" {
		return Peer{}, fmt.Errorf("peer id is required")
	}
	if p.URL == "" {
		return Peer{}, fmt.Errorf("peer url is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	if existing, ok := r.peers[p.ID]; ok {
		p.InFlight = existing.InFlight
		p.RegisteredAt = existing.RegisteredAt
		if p.Secret == "" {
			p.Secret = existing.Secret
		}
	} else {
		p.RegisteredAt = now
	}
	p.LastSeen = now
	p.Failures = 0
	p.Quarantined = false

	stored := p
	r.peers[p.ID] = &stored
	return stored, nil
}

func (r *PeerRegistry) Get(id string) (Peer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.peers[id]
	if !ok {
		return Peer{}, false
	}
	return *p, true
}

func (r *PeerRegistry) Remove(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.peers[id]
	delete(r.peers, id)
	return ok
}

// List returns every peer, secrets stripped by the json tag.
func (r *PeerRegistry) List() []Peer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Peer, 0, len(r.peers))
	for _, p := range r.peers {
		out = append(out, *p)
	}
	return out
}

// Known reports whether an id belongs to a registered peer.
func (r *PeerRegistry) Known(id string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.peers[id]
	return ok
}

// LeastLoaded picks the available peer with the most headroom that advertises
// the capability. This is what makes worker-to-worker delegation possible
// without routing everything through the master's own registry.
func (r *PeerRegistry) LeastLoaded(capability string, exclude ...string) (Peer, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	skip := make(map[string]bool, len(exclude))
	for _, id := range exclude {
		skip[id] = true
	}

	var best *Peer
	bestFree := -1
	for id, p := range r.peers {
		if skip[id] || !p.Available() || !p.HasCapability(capability) {
			continue
		}
		free := p.MaxInFlight - p.InFlight
		if p.MaxInFlight <= 0 {
			free = 1 << 30 // unbounded peers always have room
		}
		if free > bestFree {
			bestFree = free
			best = p
		}
	}
	if best == nil {
		return Peer{}, false
	}
	return *best, true
}

func (r *PeerRegistry) Reserve(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.peers[id]
	if !ok || !p.Available() {
		return false
	}
	p.InFlight++
	return true
}

func (r *PeerRegistry) Release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.peers[id]; ok && p.InFlight > 0 {
		p.InFlight--
	}
}

// MarkDelivered records a successful delivery and clears the failure streak.
func (r *PeerRegistry) MarkDelivered(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p, ok := r.peers[id]; ok {
		p.LastSeen = time.Now()
		p.Failures = 0
		p.Quarantined = false
	}
}

// MarkFailed records a failed delivery, quarantining the peer once the streak
// crosses the threshold. Returns whether the peer is now quarantined.
func (r *PeerRegistry) MarkFailed(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.peers[id]
	if !ok {
		return false
	}
	p.Failures++
	if p.Failures >= quarantineAfter {
		p.Quarantined = true
	}
	return p.Quarantined
}
