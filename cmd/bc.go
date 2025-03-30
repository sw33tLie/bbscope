package cmd

import (
	"os"

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
		var err error
		config, _ := cmd.Flags().GetString("config")

		// Check if the config file exists and is a valid YAML file
		if config != "" {
			if _, err := os.Stat(config); err == nil {
				viper.SetConfigFile(config)
				if err := viper.ReadInConfig(); err != nil {
					utils.Log.Fatalf("Error reading config file: %v", err)
				}

				// Check if the config file contains a "bugcrowd" section
				if viper.IsSet("bugcrowd") {
					bugcrowdConfig := viper.Sub("bugcrowd")
					if bugcrowdConfig.IsSet("token") {
						cmd.Flags().Set("token", bugcrowdConfig.GetString("token"))
					}
					if bugcrowdConfig.IsSet("categories") {
						cmd.Flags().Set("categories", bugcrowdConfig.GetString("categories"))
					}
					if bugcrowdConfig.IsSet("concurrency") {
						cmd.Flags().Set("concurrency", bugcrowdConfig.GetString("concurrency"))
					}
					if bugcrowdConfig.IsSet("email") {
						cmd.Flags().Set("email", bugcrowdConfig.GetString("email"))
					}
					if bugcrowdConfig.IsSet("password") {
						cmd.Flags().Set("password", bugcrowdConfig.GetString("password"))
					}
				}
			} else {
				utils.Log.Fatalf("Config file not found: %v", err)
			}
		}

		token, _ := cmd.Flags().GetString("token")
		categories, _ := cmd.Flags().GetString("categories")
		concurrency, _ := cmd.Flags().GetInt("concurrency")

		outputFlags, _ := rootCmd.PersistentFlags().GetString("output")
		delimiterCharacter, _ := rootCmd.PersistentFlags().GetString("delimiter")
		includeOOS, _ := rootCmd.PersistentFlags().GetBool("oos")

		proxy, _ := rootCmd.PersistentFlags().GetString("proxy")
		bbpOnly, _ := cmd.Flags().GetBool("bbpOnly")
		pvtOnly, _ := cmd.Flags().GetBool("pvtOnly")

		email := viper.GetViper().GetString("bugcrowd-email")
		password := viper.GetViper().GetString("bugcrowd-password")

		if proxy != "" {
			whttp.SetupProxy(proxy)
		}

		if email != "" && password != "" && token == "" {
			token, err = bugcrowd.Login(email, password, proxy)
			if err != nil {
				utils.Log.Fatal("[bc] ", err)
			}
		}

		_, err = bugcrowd.GetAllProgramsScope(token, bbpOnly, pvtOnly, categories, outputFlags, concurrency, delimiterCharacter, includeOOS, true, nil)

		if err != nil {
			utils.Log.Fatal("[bc] ", err)
		}

		utils.Log.Info("bbscope run successfully")
	},
}

func init() {
	rootCmd.AddCommand(bcCmd)
	bcCmd.Flags().StringP("token", "t", "", "Bugcrowd session token (_bugcrowd_session cookie)")
	bcCmd.Flags().StringP("categories", "c", "all", "Scope categories, comma separated (Available: all, url, api, mobile, android, apple, other, hardware)")

	// Useless as of now since we're forcing 1 http request per second due to Bugcrowd's WAF
	bcCmd.Flags().IntP("concurrency", "", 1, "Concurrency threshold")

	bcCmd.Flags().StringP("email", "E", "", "Login email")
	viper.BindPFlag("bugcrowd-email", bcCmd.Flags().Lookup("email"))

	bcCmd.Flags().StringP("password", "P", "", "Login password")
	viper.BindPFlag("bugcrowd-password", bcCmd.Flags().Lookup("password"))

	bcCmd.Flags().StringP("limit", "L", "", "Limit the number of programs fetched")
}
