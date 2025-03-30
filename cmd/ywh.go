package cmd

import (
	"log"
	"os"

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
		config, _ := cmd.Flags().GetString("config")

		// Check if the config file exists and is a valid YAML file
		if config != "" {
			if _, err := os.Stat(config); err == nil {
				viper.SetConfigFile(config)
				if err := viper.ReadInConfig(); err != nil {
					log.Fatalf("Error reading config file: %v", err)
				}
				log.Println("Config file loaded successfully")

				// Check if the config file contains a "yeswehack" section
				if viper.IsSet("yeswehack") {
					yeswehackConfig := viper.Sub("yeswehack")
					if yeswehackConfig.IsSet("token") {
						cmd.Flags().Set("token", yeswehackConfig.GetString("token"))
					}
					if yeswehackConfig.IsSet("categories") {
						cmd.Flags().Set("categories", yeswehackConfig.GetString("categories"))
					}
					if yeswehackConfig.IsSet("email") {
						cmd.Flags().Set("email", yeswehackConfig.GetString("email"))
					}
					if yeswehackConfig.IsSet("password") {
						cmd.Flags().Set("password", yeswehackConfig.GetString("password"))
					}
					if yeswehackConfig.IsSet("otpcommand") {
						cmd.Flags().Set("otpcommand", yeswehackConfig.GetString("otpcommand"))
					}
				} else {
					log.Fatalf("Config file does not contain a 'yeswehack' section")
				}
			} else {
				log.Fatalf("Config file not found: %v", err)
			}
		}

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		token, _ := cmd.Flags().GetString("token")

		if token == "" {
			// Use email+password+OTP login
			email := viper.GetViper().GetString("yeswehack-email")
			password := viper.GetViper().GetString("yeswehack-password")
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
