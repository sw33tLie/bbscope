package cmd

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/sw33tLie/bbscope/pkg/platforms/yeswehack"
	"github.com/sw33tLie/bbscope/pkg/whttp"
)

// ywhCmd represents the ywh command
var ywhCmd = &cobra.Command{
	Use:   "ywh",
	Short: "YesWeHack",
	Long:  "Gathers data from YesWeHack (https://yeswehack.com/)",
	Run: func(cmd *cobra.Command, args []string) {
		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			// Use email+password+OTP login
			email := viper.GetViper().GetString("yeswehack-email")
			password := viper.GetViper().GetString("yeswehack-password")
			// not mandatory, 2FA can be off but some programs require it
			otpFetchCommand := viper.GetViper().GetString("yeswehack-otpcommand")

			if email == "" || password == "" {
				log.Fatal("Please provide your YesWeHack email and password. Or just provide a valid session token")
			}

			var err error
			token, err = yeswehack.Login(email, password, otpFetchCommand, proxy)
			if err != nil {
				log.Fatal(err)
			}

		}

		categories, _ := cmd.Flags().GetString("categories")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		bbpOnly, _ := rootCmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := rootCmd.Flags().GetBool("pvtOnly")

		yeswehack.GetAllProgramsScope(token, bbpOnly, pvtOnly, categories, outputFlags, delimiterCharacter, true)
	},
}

func init() {
	rootCmd.AddCommand(ywhCmd)
	ywhCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, mobile, android, apple, executable, other)")

	ywhCmd.Flags().StringP("token", "t", "", "YesWeHack Authorization Bearer Token (From api.yeswehack.com)")

	// Alternatively, we can use email and password to login
	ywhCmd.Flags().StringP("email", "E", "", "Login email")
	viper.BindPFlag("yeswehack-email", ywhCmd.Flags().Lookup("email"))

	ywhCmd.Flags().StringP("password", "P", "", "Login password")
	viper.BindPFlag("yeswehack-password", ywhCmd.Flags().Lookup("password"))

	ywhCmd.Flags().StringP("otpcommand", "O", "", "Bash command to fetch OTP. stdout should be the otp")
	viper.BindPFlag("yeswehack-otpcommand", ywhCmd.Flags().Lookup("otpcommand"))
}
