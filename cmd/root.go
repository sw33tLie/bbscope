package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/sw33tLie/bbscope/v2/internal/utils"

	homedir "github.com/mitchellh/go-homedir"
	"github.com/spf13/viper"
)

var cfgFile string

const (
	LOGO = `	 _     _                              
	| |__ | |__  ___  ___ ___  _ __   ___ 
	| '_ \| '_ \/ __|/ __/ _ \| '_ \ / _ \
	| |_) | |_) \__ \ (_| (_) | |_) |  __/
	|_.__/|_.__/|___/\___\___/| .__/ \___|
	                          |_|         
							  
`
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "bbscope",
	Short: "A powerful scope aggregator for bug bounty hunters.",
	Long: LOGO + `bbscope helps you manage bug bounty program scopes from HackerOne, Bugcrowd, Intigriti, YesWeHack, and Immunefi, right from your command line.

Visit https://bbscope.com for an hourly-updated list of public scopes!`,
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.bbscope.yaml)")

	// Global flags
	rootCmd.PersistentFlags().StringP("proxy", "", "", "HTTP Proxy (Useful for debugging. Example: http://127.0.0.1:8080)")
	rootCmd.PersistentFlags().StringP("loglevel", "l", "info", "Set log level. Available: debug, info, warn, error, fatal")

}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		viper.SetConfigFile(cfgFile)
	} else {
		home, err := homedir.Dir()
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		viper.AddConfigPath(home)
		viper.SetConfigName(".bbscope")
		viper.SetConfigType("yaml")
	}

	viper.AutomaticEnv()

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found; create it with defaults.
			home, _ := homedir.Dir()
			configPath := home + "/.bbscope.yaml"
			if err := viper.SafeWriteConfigAs(configPath); err != nil {
				fmt.Printf("Error creating config file: %s", err)
			}
		}
	}

	// Set default empty values for all keys
	viper.SetDefault("hackerone.username", "")
	viper.SetDefault("hackerone.token", "")
	viper.SetDefault("bugcrowd.email", "")
	viper.SetDefault("bugcrowd.password", "")
	viper.SetDefault("bugcrowd.otpsecret", "")
	viper.SetDefault("intigriti.token", "")
	viper.SetDefault("yeswehack.email", "")
	viper.SetDefault("yeswehack.password", "")
	viper.SetDefault("yeswehack.otpsecret", "")

	// Init log library
	levelString, _ := rootCmd.PersistentFlags().GetString("loglevel")
	utils.SetLogLevel(levelString)

}
