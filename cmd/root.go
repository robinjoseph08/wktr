package cmd

import (
	"github.com/robinjoseph08/wktr/internal/version"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "wktr",
	Short:   "Manage git worktrees with terminal multiplexer integration",
	Long:    "A CLI tool to create, list, and remove git worktrees with automatic window and pane configuration in your terminal multiplexer (tmux or herdr).",
	Version: version.Version,
}

func init() {
	rootCmd.SetVersionTemplate(version.Version + "\n")
}

func Execute() error {
	return rootCmd.Execute()
}
