package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/michaelxu2288/bullpen/internal/crew"
)

type sessionItem struct {
	session crew.Session
}

func (s sessionItem) Title() string       { return s.session.Name }
func (s sessionItem) Description() string { return fmt.Sprintf("%s | %s", s.session.Provider, s.session.Branch) }
func (s sessionItem) FilterValue() string { return s.session.Name }

type Model struct {
	list     list.Model
	sessions []crew.Session
}

func NewModel(sessions []crew.Session) Model {
	items := make([]list.Item, 0, len(sessions))
	for _, sess := range sessions {
		items = append(items, sessionItem{session: sess})
	}
	l := list.New(items, list.NewDefaultDelegate(), 90, 24)
	l.Title = "Multi Agent Sessions"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(true)
	return Model{list: l, sessions: sessions}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.list.SetWidth(msg.Width - 2)
		m.list.SetHeight(msg.Height - 4)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	header := lipgloss.NewStyle().Bold(true).Render("Claude Code / Codex Multi-Agent Orchestration")
	note := lipgloss.NewStyle().Faint(true).Render("q to quit | / to filter sessions")
	return strings.Join([]string{header, note, m.list.View()}, "\n")
}

func Run(sessions []crew.Session) error {
	p := tea.NewProgram(NewModel(sessions), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
