package memory

import "strings"

type ContextWindow struct {
	MaxChars int
}

func (c ContextWindow) Build(messages []Message) string {
	if c.MaxChars <= 0 {
		c.MaxChars = 6000
	}
	var b strings.Builder
	for i := len(messages) - 1; i >= 0; i-- {
		line := messages[i].Role + ": " + messages[i].Content + "\n"
		if b.Len()+len(line) > c.MaxChars {
			break
		}
		b.WriteString(line)
	}
	return b.String()
}
