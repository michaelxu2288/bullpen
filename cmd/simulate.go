package cmd

import (
	"context"
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/plan"
	"github.com/spf13/cobra"
)

var simulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "Simulate a full multi-agent run for a goal",
	RunE: func(cmd *cobra.Command, args []string) error {
		goal, _ := cmd.Flags().GetString("goal")
		trace, _ := cmd.Flags().GetString("trace-id")

		engine := newEngine()
		res, err := engine.Run(context.Background(), plan.RunInput{Goal: goal, TraceID: trace})
		if err != nil {
			return err
		}

		fmt.Printf("strategy: %s\n", res.Strategy)
		for _, t := range res.Tasks {
			fmt.Printf("- %s (%s) -> %s\n", t.ID, t.Title, res.SessionByTask[t.ID])
		}

		messages := engine.Handoffs.Drain()
		fmt.Printf("handoff messages: %d\n", len(messages))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(simulateCmd)
	simulateCmd.Flags().String("goal", "deliver mvp with multi-agent handoffs and rag", "Goal to simulate")
	simulateCmd.Flags().String("trace-id", "trace-demo-001", "Trace identifier")
}
