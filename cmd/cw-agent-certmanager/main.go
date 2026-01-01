package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/certwatch-app/cw-agent/internal/certmanager"
	"github.com/certwatch-app/cw-agent/internal/certmanager/config"
	"github.com/certwatch-app/cw-agent/internal/state"
	"github.com/certwatch-app/cw-agent/internal/version"
)

var cfgFile string

func main() {
	rootCmd := &cobra.Command{
		Use:   "cw-agent-certmanager",
		Short: "CertWatch cert-manager controller",
		Long: `CertWatch cert-manager controller watches Certificate CRDs in your
Kubernetes cluster and syncs certificate status to the CertWatch cloud dashboard.

This enables centralized monitoring of all cert-manager managed certificates
across multiple clusters, with unified alerting and visibility.`,
		RunE: run,
	}

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file path")

	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("cw-agent-certmanager %s\n", version.GetVersion())
		},
	}
	rootCmd.AddCommand(versionCmd)

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration without starting",
		RunE:  validate,
	}
	rootCmd.AddCommand(validateCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) error {
	// Load config
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	// Load state (agent ID persistence)
	stateManager := state.NewManager(cfgFile)
	if loadErr := stateManager.Load(); loadErr != nil {
		// Don't fail on state load errors - we'll create new state
		fmt.Printf("Note: %v (will create new state)\n", loadErr)
	}

	// Create agent
	agent, err := certmanager.New(cfg, stateManager)
	if err != nil {
		return fmt.Errorf("failed to create agent: %w", err)
	}

	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		fmt.Printf("\nReceived signal %v, shutting down...\n", sig)
		cancel()
	}()

	// Run agent
	return agent.Run(ctx)
}

func validate(cmd *cobra.Command, args []string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	fmt.Println("✓ Configuration valid")
	fmt.Printf("  Agent name: %s\n", cfg.Agent.Name)
	fmt.Printf("  Cluster name: %s\n", cfg.Agent.ClusterName)
	fmt.Printf("  API endpoint: %s\n", cfg.API.Endpoint)
	fmt.Printf("  Metrics port: %d\n", cfg.Agent.MetricsPort)
	fmt.Printf("  Sync interval: %s\n", cfg.Agent.SyncInterval)
	fmt.Printf("  Watch all namespaces: %v\n", cfg.Agent.WatchAllNS)
	if !cfg.Agent.WatchAllNS && len(cfg.Agent.Namespaces) > 0 {
		fmt.Printf("  Namespaces: %v\n", cfg.Agent.Namespaces)
	}

	return nil
}

func loadConfig() (*config.Config, error) {
	v := viper.New()

	// Environment variable support
	v.SetEnvPrefix("CW")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Load config file if provided
	if cfgFile != "" {
		v.SetConfigFile(cfgFile)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Load and validate config
	cfg, err := config.Load(v)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}
