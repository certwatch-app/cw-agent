package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/certwatch-app/cw-agent/internal/config"
)

var validateCmd = &cobra.Command{
	Use:   "validate",
	Short: "Validate the configuration file",
	Long: `Validate the CertWatch Agent configuration file without starting the agent.

Example:
  cw-agent validate -c /path/to/certwatch.yaml`,
	RunE: runValidate,
}

func init() {
	rootCmd.AddCommand(validateCmd)
}

func runValidate(cmd *cobra.Command, args []string) error {
	// Load configuration
	cfg, err := config.Load(viper.GetViper())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	fmt.Println("Configuration is valid!")
	fmt.Printf("  Agent name: %s\n", cfg.Agent.Name)
	fmt.Printf("  API endpoint: %s\n", cfg.API.Endpoint)
	fmt.Printf("  Certificates: %d\n", len(cfg.Certificates))
	fmt.Printf("  Sync interval: %s\n", cfg.Agent.SyncInterval)
	fmt.Printf("  Scan interval: %s\n", cfg.Agent.ScanInterval)

	return nil
}
