package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "wktr",
	Short: "Manage git worktrees with tmux integration",
	Long:  "A CLI tool to create, list, and remove git worktrees with automatic tmux window and pane configuration.",
}

func Execute() error {
	return rootCmd.Execute()
}
