package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
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
		proxy, _ := cmd.Flags().GetString("proxy")

		email := viper.GetViper().GetString("bugcrowd-email")
		password := viper.GetViper().GetString("bugcrowd-password")

		if proxy != "" {
			proxyURL, err := url.Parse(proxy)
			if err != nil {
				log.Fatal("Invalid Proxy String")
			}
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			http.DefaultTransport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
		}

		if email != "" && password != "" && token == "" {
			token = bugcrowd.Login(email, password)
		}

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
	bcCmd.Flags().StringP("proxy", "", "", "HTTP Proxy (Useful for debugging. Example: http://127.0.0.1:8080)")

	bcCmd.Flags().StringP("email", "E", "", "Login email")
	viper.BindPFlag("bugcrowd-email", bcCmd.Flags().Lookup("email"))

	bcCmd.Flags().StringP("password", "P", "", "Login password")
	viper.BindPFlag("bugcrowd-password", bcCmd.Flags().Lookup("password"))

}
