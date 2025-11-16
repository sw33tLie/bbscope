package cmd

import "github.com/spf13/cobra"

var getIPsCmd = &cobra.Command{
	Use:   "ips",
	Short: "Get all targets that are IP addresses",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrintTargets("ips", false)
	},
}

func init() {
	getCmd.AddCommand(getIPsCmd)
}
