package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/crew"
	"github.com/spf13/cobra"
)

var orchestrateCmd = &cobra.Command{
	Use:   "orchestrate",
	Short: "Launch an agent in an isolated git worktree + tmux session",
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		provider, _ := cmd.Flags().GetString("provider")
		program, _ := cmd.Flags().GetString("program")
		branchPrefix, _ := cmd.Flags().GetString("branch-prefix")

		mgr, err := crew.NewManager(repoPath)
		if err != nil {
			return err
		}

		sess, err := mgr.Launch(crew.LaunchOptions{
			Name:         name,
			Provider:     provider,
			Program:      program,
			BranchPrefix: branchPrefix,
		})
		if err != nil {
			return err
		}

		fmt.Printf("launched %s (%s)\n", sess.Name, sess.Provider)
		fmt.Printf("branch: %s\n", sess.Branch)
		fmt.Printf("worktree: %s\n", sess.Worktree)
		fmt.Printf("tmux: %s\n", sess.TmuxSession)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(orchestrateCmd)

	orchestrateCmd.Flags().String("name", "", "Unique session name")
	orchestrateCmd.Flags().String("provider", "claude", "Agent provider label (claude|codex|gemini|aider)")
	orchestrateCmd.Flags().String("program", "claude", "Command to run in the tmux session")
	orchestrateCmd.Flags().String("branch-prefix", "agent/", "Branch prefix for worktree branches")

	_ = orchestrateCmd.MarkFlagRequired("name")
	_ = orchestrateCmd.MarkFlagRequired("program")
}
