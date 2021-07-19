package cmd

import (
	"crypto/tls"
	b64 "encoding/base64"
	"log"
	"net/http"
	"net/url"

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
		username, _ := cmd.Flags().GetString("username")
		categories, _ := cmd.Flags().GetString("categories")
		publicOnly, _ := cmd.Flags().GetBool("public-only")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		if username == "" {
			log.Fatal("Please provide your HackerOne username (-u flag)")
		}

		if token == "" {
			log.Fatal("Please provide your HackerOne API token (-t flag)")
		}

		if pvtOnly && publicOnly {
			log.Fatal("Both public programs only and privates only flag true")
		}

		if proxy != "" {
			proxyURL, err := url.Parse(proxy)
			if err != nil {
				log.Fatal("Invalid Proxy String")
			}
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			http.DefaultTransport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
		}

		hackerone.PrintAllScope(b64.StdEncoding.EncodeToString([]byte(username+":"+token)), bbpOnly, pvtOnly, publicOnly, categories, outputFlags, delimiterCharacter)
	},
}

func init() {
	rootCmd.AddCommand(h1Cmd)
	h1Cmd.Flags().StringP("username", "u", "", "HackerOne username")
	h1Cmd.Flags().StringP("token", "t", "", "HackerOne API token, get it here: https://hackerone.com/settings/api_token/edit")
	h1Cmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, other, hardware, code, executable)")
	h1Cmd.Flags().BoolP("public-only", "", false, "Only print scope for public programs")

}
