package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// ywhCmd represents the ywh command
var ywhCmd = &cobra.Command{
	Use:   "ywh",
	Short: "YesWeHack",
	Long:  "Gathers data from YesWeHack (https://yeswehack.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")

		categories, _ := cmd.Flags().GetString("categories")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		yeswehack.PrintAllScope(token, bbpOnly, pvtOnly, categories, outputFlags, delimiterCharacter)
	},
}

func init() {
	rootCmd.AddCommand(ywhCmd)
	ywhCmd.Flags().StringP("token", "t", "", "YesWeHack Authorization Bearer Token (From api.yeswehack.com)")
	ywhCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, mobile, android, apple, executable, other)")
}
