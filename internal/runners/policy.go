package runners

import "strings"

type ToolPolicy struct {
	Allow map[string]bool
}

func NewToolPolicy(tools []string) ToolPolicy {
	allow := make(map[string]bool, len(tools))
	for _, t := range tools {
		allow[strings.TrimSpace(t)] = true
	}
	return ToolPolicy{Allow: allow}
}

func (t ToolPolicy) Permits(toolName string) bool {
	if len(t.Allow) == 0 {
		return true
	}
	return t.Allow[toolName]
}
