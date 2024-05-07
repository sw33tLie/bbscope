package cmd

import (
	"crypto/tls"
	"log"
	"net/http"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/internal/utils"
	"github.com/sw33tLie/bbscope/pkg/platforms/bugcrowd"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// bcCmd represents the bc command
var bcCmd = &cobra.Command{
	Use:   "bc",
	Short: "Bugcrowd",
	Long:  "Gathers data from Bugcrowd (https://bugcrowd.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		token, _ := cmd.Flags().GetString("token")
		categories, _ := cmd.Flags().GetString("categories")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		email := viper.GetViper().GetString("bugcrowd-email")
		password := viper.GetViper().GetString("bugcrowd-password")

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

		if email != "" && password != "" && token == "" {
			token = bugcrowd.Login(email, password, proxy)
		}

		bugcrowd.GetAllProgramsScope(token, bbpOnly, pvtOnly, categories, outputFlags, concurrency, delimiterCharacter, includeOOS, true)
		utils.Log.Info("bbscope run successfully")
	},
}

func init() {
	rootCmd.AddCommand(bcCmd)
	bcCmd.Flags().StringP("token", "t", "", "Bugcrowd session token (_bugcrowd_session cookie)")
	bcCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, api, mobile, android, apple, other, hardware)")
	bcCmd.Flags().IntP("concurrency", "", 1, "Concurrency threshold") // Bugcrowd returns 406 after a while if we go faster

	bcCmd.Flags().StringP("email", "E", "", "Login email")
	viper.BindPFlag("bugcrowd-email", bcCmd.Flags().Lookup("email"))

	bcCmd.Flags().StringP("password", "P", "", "Login password")
	viper.BindPFlag("bugcrowd-password", bcCmd.Flags().Lookup("password"))

}
