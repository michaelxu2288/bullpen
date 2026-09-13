package crew

import (
	"fmt"
	"strings"

	"github.com/michaelxu2288/bullpen/internal/hooks"
)

func (m *Manager) PipePrompt(source, target string, mode hooks.Mode, maxLines int) error {
	if maxLines <= 0 {
		maxLines = 80
	}

	raw, err := m.Capture(source)
	if err != nil {
		return fmt.Errorf("capture failed for %s: %w", source, err)
	}

	payload := tail(raw, maxLines)
	prompt := hooks.BuildHandoffPrompt(mode, source, payload)
	if prompt == "" {
		return nil
	}
	return m.SendPrompt(target, prompt)
}

func tail(input string, n int) string {
	lines := strings.Split(input, "\n")
	if len(lines) <= n {
		return strings.TrimSpace(input)
	}
	return strings.TrimSpace(strings.Join(lines[len(lines)-n:], "\n"))
}
