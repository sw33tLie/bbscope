package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/platforms/intigriti"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// itCmd represents the it command
var itCmd = &cobra.Command{
	Use:   "it",
	Short: "Intigriti",
	Long:  "Gathers data from Intigriti (https://intigriti.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")

		categories, _ := cmd.Flags().GetString("categories")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		intigriti.GetAllProgramsScope(token, bbpOnly, pvtOnly, categories, outputFlags, delimiterCharacter, includeOOS, true)
	},
}

func init() {
	rootCmd.AddCommand(itCmd)
	itCmd.Flags().StringP("token", "t", "", "Intigriti API token")
	itCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, device, other, wildcard)")
}
