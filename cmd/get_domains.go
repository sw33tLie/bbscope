package cmd

import "github.com/spf13/cobra"

var getDomainsCmd = &cobra.Command{
	Use:   "domains",
	Short: "Get all targets that are domains (including wildcards)",
	RunE: func(cmd *cobra.Command, args []string) error {
		aggressive, _ := cmd.Flags().GetBool("aggressive")
		return getAndPrintTargets("domains", aggressive)
	},
}

func init() {
	getDomainsCmd.Flags().BoolP("aggressive", "a", false, "Apply aggressive scope transformation")
	getCmd.AddCommand(getDomainsCmd)
}
