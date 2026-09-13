package cmd

import (
	"fmt"

	"github.com/michaelxu2288/bullpen/internal/pty"
	"github.com/spf13/cobra"
)

var ptyCmd = &cobra.Command{
	Use:   "pty",
	Short: "Manage PTY/tmux-backed agent terminal sessions",
}

var ptyListCmd = &cobra.Command{
	Use:   "ls",
	Short: "List live PTY sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		b := pty.TmuxBackend{}
		sessions, err := b.List()
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("no live sessions")
			return nil
		}
		for _, s := range sessions {
			fmt.Println(s)
		}
		return nil
	},
}

var ptyCaptureCmd = &cobra.Command{
	Use:   "capture <name>",
	Short: "Scrape the last N lines from a session pane",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		lines, _ := cmd.Flags().GetInt("lines")
		out, err := pty.TmuxBackend{}.Capture(args[0], lines)
		if err != nil {
			return err
		}
		fmt.Print(out)
		return nil
	},
}

func init() {
	ptyCaptureCmd.Flags().Int("lines", 40, "lines to capture")
	ptyCmd.AddCommand(ptyListCmd, ptyCaptureCmd)
	rootCmd.AddCommand(ptyCmd)
}
