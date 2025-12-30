package cmd

import (
	"github.com/spf13/cobra"

	"github.com/certwatch-app/cw-agent/internal/cmd/initcmd"
)

var (
	initOutputPath     string
	initNonInteractive bool
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize a new CertWatch Agent configuration",
	Long: `Interactively create a new CertWatch Agent configuration file.

The wizard will guide you through setting up:
  • API connection settings (API key, endpoint)
  • Agent behavior configuration (name, sync/scan intervals)
  • Certificates to monitor (hostnames, ports, tags)

Examples:
  # Interactive mode (default)
  cw-agent init

  # Specify output path
  cw-agent init -o /etc/certwatch/certwatch.yaml

  # Non-interactive mode (for CI/scripting)
  CW_API_KEY=cw_xxx CW_AGENT_NAME=prod CW_CERTIFICATES=api.example.com cw-agent init --non-interactive

Environment variables for non-interactive mode:
  CW_API_KEY        (required) Your CertWatch API key
  CW_API_ENDPOINT   (optional) API endpoint (default: https://api.certwatch.app)
  CW_AGENT_NAME     (optional) Agent name (default: default-agent)
  CW_SYNC_INTERVAL  (optional) Sync interval (default: 5m)
  CW_SCAN_INTERVAL  (optional) Scan interval (default: 1m)
  CW_LOG_LEVEL      (optional) Log level (default: info)
  CW_CERTIFICATES   (required) Comma-separated hostnames to monitor`,
	RunE: runInit,
}

func init() {
	rootCmd.AddCommand(initCmd)

	initCmd.Flags().StringVarP(&initOutputPath, "output", "o", "./certwatch.yaml",
		"Output path for the configuration file")
	initCmd.Flags().BoolVar(&initNonInteractive, "non-interactive", false,
		"Run in non-interactive mode using environment variables")
}

func runInit(_ *cobra.Command, _ []string) error {
	if initNonInteractive {
		return initcmd.RunNonInteractive(initOutputPath)
	}

	wizard := initcmd.NewWizard()
	wizard.SetOutputPath(initOutputPath)
	return wizard.Run()
}
