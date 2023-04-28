package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/pkg/platforms/immunefi"
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
			http.DefaultTransport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			http.DefaultTransport.(*http.Transport).Proxy = http.ProxyURL(proxyURL)
		}

		immunefi.PrintAllScope(categories, outputFlags, delimiterCharacter, concurrency)
	},
}

func init() {
	rootCmd.AddCommand(immunefiCmd)
	immunefiCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, web, contracts)")
	immunefiCmd.Flags().IntP("concurrency", "", 5, "Concurrency threshold")

}
