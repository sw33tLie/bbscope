package cmd

import (
	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/hackerone"
)

// h1Cmd represents the h1 command
var h1Cmd = &cobra.Command{
	Use:   "h1",
	Short: "HackerOne",
	Long:  "Gathers data from HackerOne (https://hackerone.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")
		bbpOnly, _ := cmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := cmd.Flags().GetBool("pvtOnly")
		categories, _ := cmd.Flags().GetString("categories")
		descToo, _ := cmd.Flags().GetBool("descToo")
		urlsToo, _ := cmd.Flags().GetBool("urlsToo")
		noToken, _ := cmd.Flags().GetBool("noToken")
		list, _ := cmd.Flags().GetBool("list")

		if !list {
			hackerone.PrintScope(token, bbpOnly, pvtOnly, categories, descToo, urlsToo, noToken)
		} else {
			hackerone.ListPrograms(token, bbpOnly, pvtOnly, categories, noToken)
		}
	},
}

func init() {
	rootCmd.AddCommand(h1Cmd)
	h1Cmd.Flags().StringP("token", "t", "", "HackerOne session token (__Host-session cookie)")
	h1Cmd.Flags().BoolP("bbpOnly", "b", false, "Only fetch programs offering monetary rewards")
	h1Cmd.Flags().BoolP("pvtOnly", "p", false, "Only fetch data from private programs")
	h1Cmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, other, hardware, code, executable)")
	h1Cmd.Flags().BoolP("descToo", "d", false, "Also print the scope description (some URLs might be here)")
	h1Cmd.Flags().BoolP("urlsToo", "u", false, "Also print the program URL (on each line)")
	h1Cmd.Flags().BoolP("noToken", "", false, "Don't use a session token (aka public programs only)")
	h1Cmd.Flags().BoolP("list", "l", false, "List programs instead of grabbing their scope")
}
