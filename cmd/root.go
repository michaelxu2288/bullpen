package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	repoPath string
)

var rootCmd = &cobra.Command{
	Use:   "bullpen",
	Short: "Control plane for an autonomous multi-agent crew",
	Long: `bullpen is a terminal-native control plane for orchestrating crews of
coding agents across isolated git worktrees, PTY/tmux sessions, a master/worker
scheduler, an internal gossip bus, and a kanban TUI.

Every worker runs in its own git worktree and PTY session, reports over the bus,
and its work is reviewed by a different worker before a ticket can close.`,
}

func Execute() {
	silenceUsageOnRuntimeErrors(rootCmd)
	// Cobra has already written the error (and usage, when the failure was a
	// usage mistake). Printing it again here duplicated every message.
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// silenceUsageOnRuntimeErrors flips SilenceUsage on at the top of every RunE.
//
// Cobra checks required flags *after* PersistentPreRun (command.go: PreRun at
// ~993, ValidateRequiredFlags at ~1007), so setting the flag there would also
// swallow usage for real usage mistakes. Doing it inside RunE means validation
// has already passed: a missing flag still prints the flag table, while a
// runtime failure — no capable agent, a delegation cycle, an unreachable
// control plane — prints just its message instead of thirty lines of flags.
func silenceUsageOnRuntimeErrors(cmd *cobra.Command) {
	for _, child := range cmd.Commands() {
		silenceUsageOnRuntimeErrors(child)
	}
	if cmd.RunE == nil {
		return
	}
	inner := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		c.SilenceUsage = true
		return inner(c, args)
	}
}

func init() {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}

	rootCmd.PersistentFlags().StringVar(&repoPath, "repo", cwd, "Path to the git repository used for orchestration")
}
