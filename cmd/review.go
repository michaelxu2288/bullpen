package cmd

import (
	"fmt"
	"time"

	"github.com/michaelxu2288/bullpen/internal/review"
	"github.com/spf13/cobra"
)

var reviewCmd = &cobra.Command{
	Use:   "review",
	Short: "Simulate HITL review gate decisioning",
	RunE: func(cmd *cobra.Command, args []string) error {
		confidence, _ := cmd.Flags().GetFloat64("confidence")
		taskID, _ := cmd.Flags().GetString("task-id")
		gate := review.HITLGate{Threshold: 0.90}
		needs := gate.NeedsHuman(confidence)

		req := review.ReviewRequest{
			TaskID:      taskID,
			Session:     "reviewer-claude",
			Summary:     "Generated patch and summarized impact",
			DiffPreview: "+120 -16",
			Confidence:  confidence,
			CreatedAt:   time.Now(),
		}
		fmt.Printf("review request: %+v\n", req)
		fmt.Printf("needs_human: %t\n", needs)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(reviewCmd)
	reviewCmd.Flags().String("task-id", "task-1", "Task id")
	reviewCmd.Flags().Float64("confidence", 0.82, "Model confidence")
}
