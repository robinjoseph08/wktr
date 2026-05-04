package cmd

import (
	"github.com/robinjoseph08/wktr/internal/initialize"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize wktr configuration",
	Long:  "Set up global and per-repo configuration for wktr interactively.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return initialize.Run()
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
