package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/certwatch-app/cw-agent/internal/agent"
	"github.com/certwatch-app/cw-agent/internal/config"
)

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the CertWatch monitoring agent",
	Long: `Start the CertWatch Agent to monitor configured certificates
and sync data to the CertWatch cloud platform.

Example:
  cw-agent start -c /path/to/certwatch.yaml
  cw-agent start --config certwatch.yaml`,
	RunE: runStart,
}

func init() {
	rootCmd.AddCommand(startCmd)
}

func runStart(cmd *cobra.Command, args []string) error {
	// Load and validate configuration
	cfg, err := config.Load(viper.GetViper())
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	if validationErr := cfg.Validate(); validationErr != nil {
		return fmt.Errorf("invalid configuration: %w", validationErr)
	}

	// Create agent
	a, err := agent.New(cfg)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		cancel()
	}()

	// Start the agent
	fmt.Printf("Starting CertWatch Agent '%s'...\n", cfg.Agent.Name)
	fmt.Printf("Monitoring %d certificate(s)\n", len(cfg.Certificates))
	fmt.Printf("Sync interval: %s\n", cfg.Agent.SyncInterval)

	if err := a.Run(ctx); err != nil && err != context.Canceled {
		return fmt.Errorf("agent error: %w", err)
	}

	fmt.Println("Agent stopped gracefully")
	return nil
}
