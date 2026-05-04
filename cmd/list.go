package cmd

import (
	"fmt"

	"github.com/robinjoseph08/wktr/internal/workspace"
	"github.com/spf13/cobra"
)

var listAll bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List active worktrees",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		infos, err := workspace.List(workspace.ListOpts{All: listAll})
		if err != nil {
			return err
		}

		if len(infos) == 0 {
			fmt.Println("No active worktrees")
			return nil
		}

		currentOrg := ""
		for _, info := range infos {
			if listAll && info.OrgRepo != currentOrg {
				if currentOrg != "" {
					fmt.Println()
				}
				fmt.Printf("%s:\n", info.OrgRepo)
				currentOrg = info.OrgRepo
			}

			status := "[no window]"
			if info.HasWindow {
				status = "[active]"
			}

			fmt.Printf("  %s %s\n", info.Name, status)
			fmt.Printf("    branch: %s\n", info.Branch)
			fmt.Printf("    dir:    %s\n", info.Dir)
		}

		return nil
	},
}

func init() {
	listCmd.Flags().BoolVar(&listAll, "all", false, "list worktrees for all repos")
	rootCmd.AddCommand(listCmd)
}
