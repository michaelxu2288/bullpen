package workflow

import (
	"fmt"
	"sort"
)

type ExecutionStep struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	Status   string `json:"status"`
	Note     string `json:"note"`
}

type ExecutionResult struct {
	Workflow string          `json:"workflow"`
	Steps    []ExecutionStep `json:"steps"`
}

type Executor struct{}

func (e Executor) Execute(g Graph) (*ExecutionResult, error) {
	if err := g.Validate(); err != nil {
		return nil, err
	}

	ordered := topoSort(g.Nodes)
	steps := make([]ExecutionStep, 0, len(ordered))
	for _, n := range ordered {
		steps = append(steps, ExecutionStep{
			NodeID:   n.ID,
			NodeName: n.Name,
			Status:   "completed",
			Note:     fmt.Sprintf("executed %s node", n.Type),
		})
	}
	return &ExecutionResult{Workflow: g.Name, Steps: steps}, nil
}

func topoSort(nodes []Node) []Node {
	copyNodes := make([]Node, len(nodes))
	copy(copyNodes, nodes)
	sort.SliceStable(copyNodes, func(i, j int) bool {
		if len(copyNodes[i].Depends) == len(copyNodes[j].Depends) {
			return copyNodes[i].ID < copyNodes[j].ID
		}
		return len(copyNodes[i].Depends) < len(copyNodes[j].Depends)
	})
	return copyNodes
}
