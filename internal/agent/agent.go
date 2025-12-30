// Package agent provides the main orchestrator for the CertWatch Agent.
package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
	"github.com/certwatch-app/cw-agent/internal/sync"
)

// Agent orchestrates certificate scanning and syncing
type Agent struct {
	config   *config.Config
	scanner  *scanner.Scanner
	client   *sync.Client
	logger   *zap.Logger
	lastScan []scanner.ScanResult
}

// New creates a new Agent
func New(cfg *config.Config) (*Agent, error) {
	// Setup logger
	logger, err := setupLogger(cfg.Agent.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger: %w", err)
	}

	// Create scanner
	s := scanner.New(cfg.API.Timeout, cfg.Agent.Concurrency, logger)

	// Create sync client
	client := sync.New(cfg, logger)

	return &Agent{
		config:  cfg,
		scanner: s,
		client:  client,
		logger:  logger,
	}, nil
}

// Run starts the agent main loop
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("agent starting",
		zap.String("name", a.config.Agent.Name),
		zap.Int("certificates", len(a.config.Certificates)),
		zap.Duration("sync_interval", a.config.Agent.SyncInterval),
		zap.Duration("scan_interval", a.config.Agent.ScanInterval),
	)

	// Perform initial scan and sync
	if err := a.scanAndSync(ctx); err != nil {
		a.logger.Error("initial sync failed", zap.Error(err))
		// Continue running even if initial sync fails
	}

	// Setup tickers
	syncTicker := time.NewTicker(a.config.Agent.SyncInterval)
	scanTicker := time.NewTicker(a.config.Agent.ScanInterval)
	defer syncTicker.Stop()
	defer scanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("agent stopping")
			return ctx.Err()

		case <-scanTicker.C:
			a.logger.Debug("scan interval triggered")
			if err := a.scan(ctx); err != nil {
				a.logger.Error("scan failed", zap.Error(err))
			}

		case <-syncTicker.C:
			a.logger.Debug("sync interval triggered")
			if err := a.syncWithCloud(ctx); err != nil {
				a.logger.Error("sync failed", zap.Error(err))
			}
		}
	}
}

// scanAndSync performs a scan and immediately syncs results
func (a *Agent) scanAndSync(ctx context.Context) error {
	if err := a.scan(ctx); err != nil {
		return fmt.Errorf("scan failed: %w", err)
	}

	if err := a.syncWithCloud(ctx); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	return nil
}

// scan performs certificate scanning
func (a *Agent) scan(ctx context.Context) error {
	start := time.Now()
	a.logger.Info("starting certificate scan",
		zap.Int("certificates", len(a.config.Certificates)),
	)

	results := a.scanner.ScanAll(ctx, a.config.Certificates)
	a.lastScan = results

	// Count successes and failures
	successCount := 0
	failCount := 0
	for _, r := range results {
		if r.Success {
			successCount++
		} else {
			failCount++
		}
	}

	a.logger.Info("scan complete",
		zap.Duration("duration", time.Since(start)),
		zap.Int("success", successCount),
		zap.Int("failed", failCount),
	)

	return nil
}

// syncWithCloud sends scan results to the CertWatch API
func (a *Agent) syncWithCloud(ctx context.Context) error {
	if a.lastScan == nil {
		a.logger.Debug("no scan results to sync, performing scan first")
		if err := a.scan(ctx); err != nil {
			return err
		}
	}

	start := time.Now()
	a.logger.Info("syncing with cloud")

	resp, err := a.client.Sync(ctx, a.config.Certificates, a.lastScan)
	if err != nil {
		return err
	}

	a.logger.Info("sync complete",
		zap.Duration("duration", time.Since(start)),
		zap.String("agent_id", resp.AgentID),
		zap.Int("created", resp.Data.Created),
		zap.Int("updated", resp.Data.Updated),
		zap.Int("unchanged", resp.Data.Unchanged),
		zap.Int("orphaned", resp.Data.Orphaned),
		zap.Int("errors", len(resp.Data.Errors)),
	)

	// Log any sync errors
	for _, syncErr := range resp.Data.Errors {
		a.logger.Warn("certificate sync error",
			zap.String("hostname", syncErr.Hostname),
			zap.Int("port", syncErr.Port),
			zap.String("error", syncErr.Error),
		)
	}

	return nil
}

// setupLogger creates a configured zap logger
func setupLogger(level string) (*zap.Logger, error) {
	// Parse log level
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "info":
		zapLevel = zapcore.InfoLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	// Create encoder config
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create core
	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		zapcore.AddSync(os.Stdout),
		zapLevel,
	)

	return zap.New(core), nil
}
