package cmd

import "github.com/spf13/cobra"

var getURLsCmd = &cobra.Command{
	Use:   "urls",
	Short: "Get all targets that are URLs",
	RunE: func(cmd *cobra.Command, args []string) error {
		return getAndPrintTargets("urls", false)
	},
}

func init() {
	getCmd.AddCommand(getURLsCmd)
}
