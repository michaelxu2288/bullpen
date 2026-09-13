package mesh

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// WorkItem is a unit of work the master dispatches to a worker.
type WorkItem struct {
	TaskID     string
	Capability string
	Prompt     string
}

// Assignment records which worker won a work item.
type Assignment struct {
	TaskID string
	NodeID string
	At     time.Time
}

// Master is the single coordinator. It owns the registry, dispatches work to
// the least-loaded capable worker, and runs failure detection over the bus.
type Master struct {
	ID       string
	Registry *Registry
	Bus      *Bus
	seq      uint64

	mu       sync.Mutex
	assigned map[string]Assignment
}

func NewMaster(id string) *Master {
	return NewMasterWith(id, NewBus(), NewRegistry())
}

// NewMasterWith builds a master over an existing bus and registry, so the
// coordinator can share the fabric the HTTP mesh and the board already use.
func NewMasterWith(id string, bus *Bus, registry *Registry) *Master {
	if id == "" {
		id = "master-0"
	}
	m := &Master{
		ID:       id,
		Registry: registry,
		Bus:      bus,
		assigned: make(map[string]Assignment),
	}
	m.Bus.Subscribe(id)
	return m
}

// Assigned reports which worker currently holds a task, if any.
func (m *Master) Assigned(taskID string) (Assignment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.assigned[taskID]
	return a, ok
}

// Register adds a worker to the fleet and announces it on the broadcast wire.
func (m *Master) Register(n Node) {
	n.Role = RoleWorker
	m.Registry.Join(n)
	m.Bus.Publish(Message{
		Kind: MsgGossip, From: m.ID, To: "*",
		Body: fmt.Sprintf("node %s joined caps=%v", n.ID, n.Capabilities),
	})
}

// Dispatch picks a worker and records the assignment. Returns the node id.
func (m *Master) Dispatch(w WorkItem) (string, error) {
	node, ok := m.Registry.LeastLoaded(w.Capability)
	if !ok {
		return "", fmt.Errorf("no healthy worker for capability %q", w.Capability)
	}
	if !m.Registry.Reserve(node) {
		return "", fmt.Errorf("worker %s lost the reservation race", node)
	}
	id := atomic.AddUint64(&m.seq, 1)
	m.mu.Lock()
	m.assigned[w.TaskID] = Assignment{TaskID: w.TaskID, NodeID: node, At: time.Now()}
	m.mu.Unlock()
	m.Bus.Publish(Message{
		ID: fmt.Sprintf("a%d", id), Kind: MsgAssign, From: m.ID, To: node,
		TaskID: w.TaskID, Body: w.Prompt,
		Headers: map[string]string{"capability": w.Capability},
	})
	return node, nil
}

// Complete releases capacity and clears the assignment.
func (m *Master) Complete(taskID string) {
	m.mu.Lock()
	a, ok := m.assigned[taskID]
	delete(m.assigned, taskID)
	m.mu.Unlock()
	if ok {
		m.Registry.Release(a.NodeID)
	}
}

// Run starts the failure detector: reaps dead workers and requeues their work.
func (m *Master) Run(ctx context.Context, tick time.Duration, requeue func(WorkItem)) {
	if tick <= 0 {
		tick = time.Second
	}
	t := time.NewTicker(tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			dead := m.Registry.Reap(3*tick, 6*tick)
			for _, id := range dead {
				m.requeueFor(id, requeue)
			}
		}
	}
}

func (m *Master) requeueFor(node string, requeue func(WorkItem)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for tid, a := range m.assigned {
		if a.NodeID == node {
			delete(m.assigned, tid)
			m.Registry.Release(node)
			if requeue != nil {
				requeue(WorkItem{TaskID: tid, Prompt: "requeued after worker " + node + " died"})
			}
		}
	}
}
