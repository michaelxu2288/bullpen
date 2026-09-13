package mesh

import (
	"sync"
	"time"
)

// NodeRole distinguishes the single master coordinator from worker agents.
type NodeRole string

const (
	RoleMaster NodeRole = "master"
	RoleWorker NodeRole = "worker"
)

// NodeState tracks liveness for the heartbeat / gossip layer.
type NodeState string

const (
	NodeJoining   NodeState = "joining"
	NodeAlive     NodeState = "alive"
	NodeSuspect   NodeState = "suspect"
	NodeDead      NodeState = "dead"
	NodeDraining  NodeState = "draining"
)

// Node is a single member of the crew (master or worker).
type Node struct {
	ID            string            `json:"id"`
	Role          NodeRole          `json:"role"`
	State         NodeState         `json:"state"`
	Provider      string            `json:"provider"`
	Capabilities  []string          `json:"capabilities"`
	MaxInFlight   int               `json:"max_in_flight"`
	InFlight      int               `json:"in_flight"`
	Incarnation   uint64            `json:"incarnation"`
	LastHeartbeat time.Time         `json:"last_heartbeat"`
	Labels        map[string]string `json:"labels"`
}

// Healthy reports whether the node can still take work.
func (n Node) Healthy() bool {
	return n.State == NodeAlive && n.InFlight < n.MaxInFlight
}

// Registry is the master's view of the worker fleet. Thread-safe.
type Registry struct {
	mu    sync.RWMutex
	nodes map[string]*Node
}

func NewRegistry() *Registry {
	return &Registry{nodes: make(map[string]*Node)}
}

func (r *Registry) Join(n Node) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n.State = NodeAlive
	n.Incarnation++
	n.LastHeartbeat = time.Now()
	if n.MaxInFlight == 0 {
		n.MaxInFlight = 2
	}
	r.nodes[n.ID] = &n
}

func (r *Registry) Heartbeat(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if node, ok := r.nodes[id]; ok {
		node.LastHeartbeat = time.Now()
		node.State = NodeAlive
		return true
	}
	return false
}

// Reap marks nodes suspect/dead based on heartbeat staleness (SWIM-style).
func (r *Registry) Reap(suspectAfter, deadAfter time.Duration) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	dead := make([]string, 0)
	for id, n := range r.nodes {
		gap := now.Sub(n.LastHeartbeat)
		switch {
		case gap > deadAfter:
			n.State = NodeDead
			dead = append(dead, id)
		case gap > suspectAfter && n.State == NodeAlive:
			n.State = NodeSuspect
		}
	}
	return dead
}

// Workers returns a stable snapshot of all worker nodes.
func (r *Registry) Workers() []Node {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Node, 0, len(r.nodes))
	for _, n := range r.nodes {
		if n.Role == RoleWorker {
			out = append(out, *n)
		}
	}
	return out
}

// LeastLoaded picks the healthy worker with the most free capacity that also
// satisfies the requested capability. Returns "" when the fleet is saturated.
func (r *Registry) LeastLoaded(capability string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	best := ""
	bestFree := -1
	for id, n := range r.nodes {
		if n.Role != RoleWorker || !n.Healthy() {
			continue
		}
		if capability != "" && !hasCap(n.Capabilities, capability) {
			continue
		}
		free := n.MaxInFlight - n.InFlight
		if free > bestFree {
			bestFree, best = free, id
		}
	}
	return best, best != ""
}

func (r *Registry) Reserve(id string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok && n.Healthy() {
		n.InFlight++
		return true
	}
	return false
}

func (r *Registry) Release(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n, ok := r.nodes[id]; ok && n.InFlight > 0 {
		n.InFlight--
	}
}

func hasCap(caps []string, want string) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}
