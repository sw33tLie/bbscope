package cmd

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/v2/internal/utils"
	"github.com/sw33tLie/bbscope/v2/pkg/platforms"
	ywhplatform "github.com/sw33tLie/bbscope/v2/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/v2/pkg/whttp"
)

// poll ywh: shorthand for YesWeHack
var pollYwhCmd = &cobra.Command{
	Use:   "ywh",
	Short: "Poll YesWeHack programs",
	RunE: func(cmd *cobra.Command, _ []string) error {
		token := viper.GetString("yeswehack.token")
		email := viper.GetString("yeswehack.email")
		password := viper.GetString("yeswehack.password")
		otpSecret := viper.GetString("yeswehack.otpsecret")
		proxy, _ := rootCmd.Flags().GetString("proxy")
		if proxy != "" {
			whttp.SetupProxy(proxy)
		}
		// Validate auth: require either token OR (email+password+otp-secret)
		if token == "" && (email == "" || password == "" || otpSecret == "") {
			utils.Log.Error("yeswehack requires either token or email+password+otp-secret")
			return nil
		}

		poller := &ywhplatform.Poller{}
		if err := poller.Authenticate(cmd.Context(), platforms.AuthConfig{Token: token, Email: email, Password: password, OtpSecret: otpSecret, Proxy: proxy}); err != nil {
			return err
		}
		return runPollWithPollers(cmd, []platforms.PlatformPoller{poller})
	},
}

func init() {
	pollCmd.AddCommand(pollYwhCmd)
	pollYwhCmd.Flags().String("token", "", "YesWeHack bearer token (optional if using email/password + otp secret)")
	pollYwhCmd.Flags().String("email", "", "YesWeHack login email")
	pollYwhCmd.Flags().String("password", "", "YesWeHack login password")
	pollYwhCmd.Flags().String("otp-secret", "", "YesWeHack TOTP secret (base32)")
	viper.BindPFlag("yeswehack.token", pollYwhCmd.Flags().Lookup("token"))
	viper.BindPFlag("yeswehack.email", pollYwhCmd.Flags().Lookup("email"))
	viper.BindPFlag("yeswehack.password", pollYwhCmd.Flags().Lookup("password"))
	viper.BindPFlag("yeswehack.otpsecret", pollYwhCmd.Flags().Lookup("otp-secret"))
}
