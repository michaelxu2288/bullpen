package cmd

import (
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active orchestrated agent sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := crew.NewManager(repoPath)
		if err != nil {
			return err
		}
		sessions, err := mgr.List()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no sessions found")
			return nil
		}

		for _, s := range sessions {
			fmt.Printf("- %s | %s | %s | %s | %s\n", s.Name, s.Provider, s.Branch, s.Program, s.CreatedAt.Format(time.RFC3339))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
