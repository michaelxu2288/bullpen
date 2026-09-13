package workflow

import "fmt"

type NodeType string

const (
	NodePrompt NodeType = "prompt"
	NodeTool   NodeType = "tool"
	NodeHITL   NodeType = "hitl"
	NodeGate   NodeType = "gate"
)

type Node struct {
	ID      string            `json:"id"`
	Type    NodeType          `json:"type"`
	Name    string            `json:"name"`
	Config  map[string]string `json:"config"`
	Depends []string          `json:"depends"`
}

type Graph struct {
	Name  string `json:"name"`
	Nodes []Node `json:"nodes"`
}

func (g Graph) Validate() error {
	index := map[string]bool{}
	for _, n := range g.Nodes {
		if n.ID == "" {
			return fmt.Errorf("workflow node id cannot be empty")
		}
		index[n.ID] = true
	}
	for _, n := range g.Nodes {
		for _, dep := range n.Depends {
			if !index[dep] {
				return fmt.Errorf("node %s depends on missing node %s", n.ID, dep)
			}
		}
	}
	return nil
}
