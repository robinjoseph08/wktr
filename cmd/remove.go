package cmd

import (
	"github.com/robinjoseph08/wktr/internal/workspace"
	"github.com/spf13/cobra"
)

var removeForce bool

var removeCmd = &cobra.Command{
	Use:   "remove [name]",
	Short: "Remove a worktree and its tmux window",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		opts := workspace.RemoveOpts{
			Force: removeForce,
		}
		if len(args) > 0 {
			opts.Name = args[0]
		}
		return workspace.Remove(opts)
	},
}

func init() {
	removeCmd.Flags().BoolVarP(&removeForce, "force", "f", false, "skip confirmation prompt")
	rootCmd.AddCommand(removeCmd)
}
