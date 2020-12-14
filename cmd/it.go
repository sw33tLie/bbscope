package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/intigriti"
)

// itCmd represents the it command
var itCmd = &cobra.Command{
	Use:   "it",
	Short: "Intigriti",
	Long:  "Gathers data from Intigriti (https://intigriti.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")
		bbpOnly, _ := cmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := cmd.Flags().GetBool("pvtOnly")
		categories, _ := cmd.Flags().GetString("categories")
		urlsToo, _ := cmd.Flags().GetBool("urlsToo")
		list, _ := cmd.Flags().GetBool("list")
		proxy, _ := cmd.Flags().GetString("proxy")

		if proxy != "" {
			proxyURL, err := url.Parse(proxy)
			if err != nil {
				log.Fatal("Invalid Proxy String")
			}
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			http.DefaultTransport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
		}

		if !list {
			intigriti.GetScope(token, bbpOnly, pvtOnly, categories, urlsToo)
		} else {
			intigriti.ListPrograms(token, bbpOnly, pvtOnly)
		}

	},
}

func init() {
	rootCmd.AddCommand(itCmd)
	itCmd.Flags().StringP("token", "t", "", "Intigriti Authentication Bearer Token (From api.intigriti.com)")
	itCmd.Flags().BoolP("bbpOnly", "b", false, "Only fetch programs offering monetary rewards")
	itCmd.Flags().BoolP("pvtOnly", "p", false, "Only fetch data from private programs")
	itCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, device, other)")
	itCmd.Flags().BoolP("urlsToo", "u", false, "Also print the program URL (on each line)")
	itCmd.Flags().BoolP("list", "l", false, "List programs instead of grabbing their scope")
	itCmd.Flags().StringP("proxy", "", "", "HTTP Proxy (Useful for debugging. Example: http://127.0.0.1:8080)")
}
