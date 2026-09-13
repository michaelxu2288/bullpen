package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/workflow"
	"github.com/spf13/cobra"
)

var pipelineCmd = &cobra.Command{
	Use:   "pipeline",
	Short: "Execute a LangGraph-style workflow template",
	RunE: func(cmd *cobra.Command, args []string) error {
		g := workflow.ClaudeCodexReviewTemplate()
		exec := workflow.Executor{}
		res, err := exec.Execute(g)
		if err != nil {
			return err
		}
		fmt.Printf("workflow: %s\n", res.Workflow)
		for _, s := range res.Steps {
			fmt.Printf("- %s: %s (%s)\n", s.NodeID, s.NodeName, s.Status)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pipelineCmd)
}
