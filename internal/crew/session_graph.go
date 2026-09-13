package crew

type SessionEdge struct {
	From string
	To   string
	Mode string
}

type SessionGraph struct {
	Nodes []Session
	Edges []SessionEdge
}

func (g *SessionGraph) AddSession(s Session) {
	g.Nodes = append(g.Nodes, s)
}

func (g *SessionGraph) AddEdge(from, to, mode string) {
	g.Edges = append(g.Edges, SessionEdge{From: from, To: to, Mode: mode})
}

func (g SessionGraph) Outbound(name string) []SessionEdge {
	out := make([]SessionEdge, 0)
	for _, e := range g.Edges {
		if e.From == name {
			out = append(out, e)
		}
	}
	return out
}
