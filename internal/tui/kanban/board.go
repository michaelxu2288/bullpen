// Package kanban renders a terminal kanban board: crew tasks flow across
// backlog -> running -> review -> done, one card per task, color-coded by the
// worker agent that owns it. Vim-style navigation (h/j/k/l), space to advance.
package kanban

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/michaelxu2288/bullpen/internal/domain"
)

var columns = []domain.TaskState{
	domain.TaskQueued,
	domain.TaskRunning,
	domain.TaskReviewing,
	domain.TaskDone,
}

var titles = map[domain.TaskState]string{
	domain.TaskQueued:    "BACKLOG",
	domain.TaskRunning:   "RUNNING",
	domain.TaskReviewing: "REVIEW",
	domain.TaskDone:      "DONE",
}

// Card is a board-friendly projection of a crew task.
type Card struct {
	ID    string
	Title string
	Owner string
	State domain.TaskState
}

type Model struct {
	cards   []Card
	col     int
	row     int
	width   int
	height  int
}

func NewModel(cards []Card) Model { return Model{cards: cards} }

func (m Model) Init() tea.Cmd { return nil }

func (m Model) column(s domain.TaskState) []int {
	idx := []int{}
	for i, c := range m.cards {
		if c.State == s {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "l", "right":
			if m.col < len(columns)-1 {
				m.col++
				m.row = 0
			}
		case "h", "left":
			if m.col > 0 {
				m.col--
				m.row = 0
			}
		case "j", "down":
			if n := len(m.column(columns[m.col])); m.row < n-1 {
				m.row++
			}
		case "k", "up":
			if m.row > 0 {
				m.row--
			}
		case " ", "enter":
			m.advance()
		}
	}
	return m, nil
}

// advance pushes the selected card to the next column (kanban flow).
func (m *Model) advance() {
	col := m.column(columns[m.col])
	if m.row >= len(col) {
		return
	}
	ci := col[m.row]
	cur := m.cards[ci].State
	for i, st := range columns {
		if st == cur {
			if i < len(columns)-1 {
				m.cards[ci].State = columns[i+1]
			}
			return
		}
	}
}

func (m Model) View() string {
	colW := 26
	if m.width > 0 {
		colW = (m.width / len(columns)) - 2
	}
	rendered := make([]string, len(columns))
	for ci, st := range columns {
		head := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63")).Render(titles[st])
		body := []string{head, ""}
		for ri, idx := range m.column(st) {
			c := m.cards[idx]
			sel := ci == m.col && ri == m.row
			body = append(body, card(c, colW, sel))
		}
		rendered[ci] = lipgloss.NewStyle().Width(colW).Padding(0, 1).Render(strings.Join(body, "\n"))
	}
	board := lipgloss.JoinHorizontal(lipgloss.Top, rendered...)
	footer := lipgloss.NewStyle().Faint(true).Render("h/j/k/l move - space advance - q quit")
	return board + "\n\n" + footer
}

func card(c Card, w int, sel bool) string {
	st := lipgloss.NewStyle().Width(w-4).Border(lipgloss.RoundedBorder()).Padding(0, 1)
	if sel {
		st = st.BorderForeground(lipgloss.Color("205"))
	}
	return st.Render(fmt.Sprintf("%s\n@%s", c.Title, c.Owner))
}

func Run(cards []Card) error {
	_, err := tea.NewProgram(NewModel(cards), tea.WithAltScreen()).Run()
	return err
}
