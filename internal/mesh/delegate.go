package mesh

import (
	"fmt"
	"strings"
	"time"
)

// MaxDelegationDepth bounds how far a task may be passed along. Agents that can
// delegate to each other will, given the chance, delegate forever: a reviewer
// hands back to a coder which hands back to a reviewer. The chain is the guard.
const MaxDelegationDepth = 6

// Delegation is one agent handing a task to another. Unlike a master dispatch,
// the sender is a peer, not the coordinator.
type Delegation struct {
	// From is the delegating crew.
	From string `json:"from"`
	// To names an explicit target. Empty means "pick by capability".
	To string `json:"to,omitempty"`
	// Capability selects a target when To is empty.
	Capability string `json:"capability,omitempty"`
	TaskID     string `json:"task_id"`
	Prompt     string `json:"prompt"`
	// Reason is why the sender could not do the work itself. It rides along in
	// the prompt so the receiving agent has the context, not just the task.
	Reason string `json:"reason,omitempty"`
	// Chain is the delegation path so far, oldest first. It is what makes a
	// cycle detectable and what bounds the depth.
	Chain    []string  `json:"chain,omitempty"`
	Deadline time.Time `json:"deadline,omitempty"`
}

// DelegationResult reports where a delegation landed.
type DelegationResult struct {
	TaskID   string    `json:"task_id"`
	From     string    `json:"from"`
	To       string    `json:"to"`
	Chain    []string  `json:"chain"`
	Depth    int       `json:"depth"`
	Remote   bool      `json:"remote"`
	Accepted bool      `json:"accepted"`
	At       time.Time `json:"at"`
}

// Validate checks the delegation is well-formed and does not cycle.
func (d Delegation) Validate() error {
	if d.From == "" {
		return fmt.Errorf("delegation requires a From agent")
	}
	if d.TaskID == "" {
		return fmt.Errorf("delegation requires a task id")
	}
	if d.To == "" && d.Capability == "" {
		return fmt.Errorf("delegation requires either a To agent or a Capability to route by")
	}
	if d.To == d.From {
		return fmt.Errorf("agent %s cannot delegate to itself", d.From)
	}
	if len(d.Chain) >= MaxDelegationDepth {
		return fmt.Errorf("delegation chain for %s hit the depth limit of %d: %s",
			d.TaskID, MaxDelegationDepth, strings.Join(d.Chain, " -> "))
	}
	if d.To != "" {
		for _, hop := range d.Chain {
			if hop == d.To {
				return fmt.Errorf("delegation cycle for %s: %s already handled it (%s)",
					d.TaskID, d.To, strings.Join(append(append([]string{}, d.Chain...), d.To), " -> "))
			}
		}
	}
	return nil
}

// Envelope renders the delegation as a bus message. The chain travels in the
// headers so the next hop can extend it and the guard keeps working across
// process boundaries.
func (d Delegation) Envelope(target string) Message {
	chain := append(append([]string{}, d.Chain...), d.From)
	headers := map[string]string{
		"delegation-chain": strings.Join(chain, ","),
		"delegation-depth": fmt.Sprint(len(chain)),
	}
	if d.Reason != "" {
		headers["delegation-reason"] = d.Reason
	}
	if !d.Deadline.IsZero() {
		headers["delegation-deadline"] = d.Deadline.UTC().Format(time.RFC3339)
	}

	body := d.Prompt
	if d.Reason != "" {
		body = fmt.Sprintf("%s\n\n[delegated by %s: %s]", d.Prompt, d.From, d.Reason)
	}

	return Message{
		Kind:    MsgHandoff,
		From:    d.From,
		To:      target,
		TaskID:  d.TaskID,
		Body:    body,
		Headers: headers,
		TS:      time.Now(),
	}
}

// ChainFrom reads a delegation chain back out of a received message, so an
// agent that wants to delegate onward continues the existing chain instead of
// starting a fresh one and defeating the cycle guard.
func ChainFrom(m Message) []string {
	raw, ok := m.Headers["delegation-chain"]
	if !ok || raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Broker routes delegations across both the local bus and the webhook peers.
type Broker struct {
	Bus      *Bus
	Peers    *PeerRegistry
	Registry *Registry
	// SelfID names this process, so it is never chosen as its own delegate.
	SelfID string
}

func NewBroker(bus *Bus, peers *PeerRegistry, registry *Registry, selfID string) *Broker {
	return &Broker{Bus: bus, Peers: peers, Registry: registry, SelfID: selfID}
}

// Delegate validates, resolves a target, and puts the handoff on the wire.
//
// Resolution order is deliberate: an explicit target wins; otherwise a local
// worker is preferred over a remote peer, because an in-process handoff costs a
// channel send and a webhook costs a round trip.
func (b *Broker) Delegate(d Delegation) (*DelegationResult, error) {
	if err := d.Validate(); err != nil {
		return nil, err
	}

	target, remote, err := b.resolve(d)
	if err != nil {
		return nil, err
	}

	// Re-check the cycle guard: a capability-routed delegation only learns its
	// target here, after Validate could check it.
	for _, hop := range d.Chain {
		if hop == target {
			return nil, fmt.Errorf("delegation cycle for %s: %s already handled it (%s)",
				d.TaskID, target, strings.Join(append(append([]string{}, d.Chain...), target), " -> "))
		}
	}
	if target == d.From {
		return nil, fmt.Errorf("capability %q only resolves back to the sender %s", d.Capability, d.From)
	}

	if remote {
		if !b.Peers.Reserve(target) {
			return nil, fmt.Errorf("peer %s has no capacity", target)
		}
	} else if b.Registry != nil && !b.Registry.Reserve(target) {
		return nil, fmt.Errorf("worker %s has no capacity", target)
	}

	b.Bus.Publish(d.Envelope(target))

	chain := append(append([]string{}, d.Chain...), d.From)
	return &DelegationResult{
		TaskID:   d.TaskID,
		From:     d.From,
		To:       target,
		Chain:    chain,
		Depth:    len(chain),
		Remote:   remote,
		Accepted: true,
		At:       time.Now(),
	}, nil
}

func (b *Broker) resolve(d Delegation) (target string, remote bool, err error) {
	if d.To != "" {
		if b.Peers != nil && b.Peers.Known(d.To) {
			return d.To, true, nil
		}
		return d.To, false, nil
	}

	exclude := append(append([]string{}, d.Chain...), d.From, b.SelfID)

	if b.Registry != nil {
		if node, ok := b.Registry.LeastLoaded(d.Capability); ok && !contains(exclude, node) {
			return node, false, nil
		}
	}
	if b.Peers != nil {
		if peer, ok := b.Peers.LeastLoaded(d.Capability, exclude...); ok {
			return peer.ID, true, nil
		}
	}
	return "", false, fmt.Errorf("no agent available with capability %q (excluding %s)",
		d.Capability, strings.Join(exclude, ", "))
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
