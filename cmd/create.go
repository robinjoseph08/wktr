package cmd

import (
	"github.com/robinjoseph08/wktr/internal/workspace"
	"github.com/spf13/cobra"
)

var createFrom string

var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new worktree with a tmux window",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := workspace.CreateOpts{
			From: createFrom,
		}
		if len(args) > 0 {
			opts.Name = args[0]
		}
		return workspace.Create(opts)
	},
}

func init() {
	createCmd.Flags().StringVar(&createFrom, "from", "", "base ref to branch from (default: current HEAD)")
	rootCmd.AddCommand(createCmd)
}
