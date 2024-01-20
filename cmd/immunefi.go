package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/platforms/immunefi"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// immunefiCmd represents the immunefi command
var immunefiCmd = &cobra.Command{
	Use:   "immunefi",
	Short: "Immunefi",
	Long:  "Gathers data from Immunefi (https://immunefi.com/explore)",
	Run: func(cmd *cobra.Command, args []string) {
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		categories, _ := cmd.Flags().GetString("categories")
		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

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

		immunefi.PrintAllScope(categories, outputFlags, delimiterCharacter, concurrency)
	},
}

func init() {
	rootCmd.AddCommand(immunefiCmd)
	immunefiCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, web, contracts)")
	immunefiCmd.Flags().IntP("concurrency", "", 5, "Concurrency threshold")
}
