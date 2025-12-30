// Package cmd provides CLI commands for the CertWatch Agent.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/certwatch-app/cw-agent/internal/version"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "cw-agent",
	Short: "CertWatch Agent - SSL/TLS certificate monitoring agent",
	Long: `CertWatch Agent monitors SSL/TLS certificates on your infrastructure
and syncs the data to the CertWatch cloud platform.

Configure certificates to monitor in certwatch.yaml and run:
  cw-agent start -c /path/to/certwatch.yaml

For more information, visit: https://certwatch.app/docs/agent`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)

	// Global flags
	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default: ./certwatch.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable verbose output")

	// Bind flags to viper
	//nolint:errcheck // error is ignored because the flag is guaranteed to exist
	viper.BindPFlag("verbose", rootCmd.PersistentFlags().Lookup("verbose"))
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag
		viper.SetConfigFile(cfgFile)
	} else {
		// Search for config in current directory
		viper.AddConfigPath(".")
		viper.AddConfigPath("/etc/certwatch")
		viper.SetConfigType("yaml")
		viper.SetConfigName("certwatch")
	}

	// Read environment variables with CW_ prefix
	viper.SetEnvPrefix("CW")
	viper.AutomaticEnv()

	// If a config file is found, read it in
	if err := viper.ReadInConfig(); err == nil {
		if verbose {
			fmt.Fprintln(os.Stderr, "Using config file:", viper.ConfigFileUsed())
		}
	}
}

// GetVersion returns the version information
func GetVersion() string {
	return version.GetVersion()
}
