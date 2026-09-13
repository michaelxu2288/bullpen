package cmd

import (
	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/michaelxu2288/bullpen/internal/domain"
	"github.com/michaelxu2288/bullpen/internal/tui/kanban"
	"github.com/spf13/cobra"
)

var kanbanCmd = &cobra.Command{
	Use:   "kanban",
	Short: "Open the crew kanban board (backlog/running/review/done)",
	Long:  "Renders live crew tasks as cards flowing across backlog -> running -> review -> done. Vim keys h/j/k/l navigate, space advances a card.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cards := seedCards()
		mgr, err := crew.NewManager(repoPath)
		if err == nil {
			if sessions, err := mgr.List(); err == nil {
				for i, s := range sessions {
					state := domain.TaskRunning
					if i%3 == 2 {
						state = domain.TaskReviewing
					}
					cards = append(cards, kanban.Card{ID: s.Name, Title: s.Name, Owner: s.Provider, State: state})
				}
			}
		}
		return kanban.Run(cards)
	},
}

// seedCards gives the board content out-of-the-box for the POC demo.
func seedCards() []kanban.Card {
	return []kanban.Card{
		{ID: "t1", Title: "decompose epic", Owner: "planner", State: domain.TaskQueued},
		{ID: "t2", Title: "scaffold api", Owner: "coder-1", State: domain.TaskRunning},
		{ID: "t3", Title: "wire rag tools", Owner: "coder-2", State: domain.TaskRunning},
		{ID: "t4", Title: "review handoff", Owner: "reviewer", State: domain.TaskReviewing},
		{ID: "t5", Title: "merge worktree", Owner: "master", State: domain.TaskDone},
	}
}

func init() { rootCmd.AddCommand(kanbanCmd) }
