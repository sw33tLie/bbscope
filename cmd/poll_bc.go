package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	bcplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/bugcrowd"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll bc: Bugcrowd
var pollBcCmd = &cobra.Command{
	Use:   "bc",
	Short: "Poll Bugcrowd programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token, _ := cmd.Flags().GetString("token") // Token is CLI-only, not from config
		email := viper.GetString("bugcrowd.email")
		password := viper.GetString("bugcrowd.password")
		otpSecret := viper.GetString("bugcrowd.otpsecret")
		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		// Validate auth: require either token OR (email+password+otp-secret)
		if token == "" && (email == "" || password == "" || otpSecret == "") {
			utils.Log.Error("bugcrowd requires either token or email+password+otp-secret.")
			return nil
		}

		poller := &bcplatform.Poller{}
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Email: email, Password: password, OtpSecret: otpSecret, Proxy: proxy}); err != nil {
			return err
		}
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollBcCmd)
	pollBcCmd.Flags().String("token", "", "Bugcrowd _bugcrowd_session cookie value")
	pollBcCmd.Flags().String("email", "", "Bugcrowd login email")
	pollBcCmd.Flags().String("password", "", "Bugcrowd login password")
	pollBcCmd.Flags().String("otp-secret", "", "Bugcrowd TOTP secret (base32)")
	viper.BindPFlag("bugcrowd.email", pollBcCmd.Flags().Lookup("email"))
	viper.BindPFlag("bugcrowd.password", pollBcCmd.Flags().Lookup("password"))
	viper.BindPFlag("bugcrowd.otpsecret", pollBcCmd.Flags().Lookup("otp-secret"))
}
