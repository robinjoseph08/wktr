package cmd

import (
	"github.com/robinjoseph08/wktr/internal/workspace"
	"github.com/spf13/cobra"
)

var resumeCmd = &cobra.Command{
	Use:   "resume [name]",
	Short: "Open a tmux window for an existing worktree",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var opts workspace.ResumeOpts
		if len(args) > 0 {
			opts.Name = args[0]
		}
		return workspace.Resume(opts)
	},
}

func init() {
	rootCmd.AddCommand(resumeCmd)
}
