package cmd

import (
	"crypto/tls"
	b64 "encoding/base64"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/platforms/hackerone"
	"github.com/sw33tLie/bbscope/pkg/whttp"
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
		active, _ := cmd.Flags().GetBool("active-only")

		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")
		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

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

			client := whttp.GetDefaultClient()
			client.HTTPClient.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					CipherSuites: []uint16{
						tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
						tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					},
					PreferServerCipherSuites: true,
					MinVersion:               tls.VersionTLS11,
					MaxVersion:               tls.VersionTLS11},
			}
		}

		hackerone.GetAllProgramsScope(b64.StdEncoding.EncodeToString([]byte(username+":"+token)), bbpOnly, pvtOnly, publicOnly, categories, active, concurrency, true, outputFlags, delimiterCharacter, includeOOS)
	},
}

func init() {
	rootCmd.AddCommand(h1Cmd)
	h1Cmd.AddCommand(hacktivityCmd)

	h1Cmd.Flags().StringP("username", "u", "", "HackerOne username")
	h1Cmd.Flags().StringP("token", "t", "", "HackerOne API token, get it here: https://hackerone.com/settings/api_token/edit")
	h1Cmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, cidr, mobile, android, apple, other, hardware, code, executable)")
	h1Cmd.Flags().BoolP("public-only", "", false, "Only print scope for public programs")
	h1Cmd.Flags().BoolP("active-only", "a", false, "Show only active programs")
	h1Cmd.Flags().IntP("concurrency", "", 3, "Concurrency of HTTP requests sent for fetching data")

	hacktivityCmd.Flags().IntP("pages", "", 100, "Pages to fetch. From most recent to older pages")

}

var hacktivityCmd = &cobra.Command{
	Use:   "hacktivity",
	Short: "HackerOne Activity",
	Long:  "Displays activity data from HackerOne",
	Run: func(cmd *cobra.Command, args []string) {
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		pages, _ := cmd.Flags().GetInt("pages")

		if proxy != "" {
			proxyURL, err := url.Parse(proxy)
			if err != nil {
				log.Fatal("Invalid Proxy String")
			}

			client := whttp.GetDefaultClient()
			client.HTTPClient.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
				TLSClientConfig: &tls.Config{
					InsecureSkipVerify: true,
					CipherSuites: []uint16{
						tls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
						tls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
					},
					PreferServerCipherSuites: true,
					MinVersion:               tls.VersionTLS11,
					MaxVersion:               tls.VersionTLS11},
			}
		}

		hackerone.HacktivityMonitor(pages)
	},
}
