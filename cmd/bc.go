package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/bugcrowd"
)

// bcCmd represents the bc command
var bcCmd = &cobra.Command{
	Use:   "bc",
	Short: "Bugcrowd",
	Long:  "Gathers data from Bugcrowd (https://bugcrowd.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")
		bbpOnly, _ := cmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := cmd.Flags().GetBool("pvtOnly")
		categories, _ := cmd.Flags().GetString("categories")
		urlsToo, _ := cmd.Flags().GetBool("urlsToo")
		concurrency, _ := cmd.Flags().GetInt("concurrency")
		list, _ := cmd.Flags().GetBool("list")

		if !list {
			bugcrowd.GetScope(token, bbpOnly, pvtOnly, categories, urlsToo, concurrency)
		} else {
			bugcrowd.ListPrograms(token, bbpOnly, pvtOnly)
		}
	},
}

func init() {
	rootCmd.AddCommand(bcCmd)
	bcCmd.Flags().StringP("token", "t", "", "Bugcrowd session token (_crowdcontrol_session cookie)")
	bcCmd.Flags().BoolP("bbpOnly", "b", false, "Only fetch programs offering monetary rewards")
	bcCmd.Flags().BoolP("pvtOnly", "p", false, "Only fetch data from private programs")
	bcCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, api, mobile, android, apple, other, hardware)")
	bcCmd.Flags().BoolP("urlsToo", "u", false, "Also print the program URL (on each line)")
	bcCmd.Flags().IntP("concurrency", "", 2, "Concurrency")
	bcCmd.Flags().BoolP("list", "l", false, "List programs instead of grabbing their scope")

}
