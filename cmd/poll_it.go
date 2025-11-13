package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	itplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/intigriti"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll it: Intigriti
var pollItCmd = &cobra.Command{
	Use:   "it",
	Short: "Poll Intigriti programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token := viper.GetString("intigriti.token")
		if token == "" {
			utils.Log.Error("intigriti requires a token")
			return nil
		}

		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		poller := itplatform.NewPoller()
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Proxy: proxy}); err != nil {
			return err
		}

		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollItCmd)
	pollItCmd.Flags().StringP("token", "t", "", "Intigriti authorization token (Bearer)")
	viper.BindPFlag("intigriti.token", pollItCmd.Flags().Lookup("token"))
}
