package cmd

import "github.com/spf13/cobra"

var getCIDRsCmd = &cobra.Command{
	Use:   "cidrs",
	Short: "Get all targets that are CIDR ranges or IP ranges",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrintTargets("cidrs", false)
	},
}

func init() {
	getCmd.AddCommand(getCIDRsCmd)
}
