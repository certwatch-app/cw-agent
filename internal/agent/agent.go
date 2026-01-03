// Package agent provides the main orchestrator for the CertWatch Agent.
package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/metrics"
	"github.com/certwatch-app/cw-agent/internal/scanner"
	"github.com/certwatch-app/cw-agent/internal/server"
	"github.com/certwatch-app/cw-agent/internal/state"
	"github.com/certwatch-app/cw-agent/internal/sync"
	"github.com/certwatch-app/cw-agent/internal/version"
)

// Agent orchestrates certificate scanning and syncing
type Agent struct {
	config       *config.Config
	scanner      *scanner.Scanner
	client       *sync.Client
	stateManager *state.Manager
	logger       *zap.Logger
	server       *server.Server
	lastScan     []scanner.ScanResult
}

// New creates a new Agent with the given configuration and state manager
func New(cfg *config.Config, stateManager *state.Manager) (*Agent, error) {
	// Setup logger
	logger, err := setupLogger(cfg.Agent.LogLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to setup logger: %w", err)
	}

	// Create scanner
	s := scanner.New(cfg.API.Timeout, cfg.Agent.Concurrency, logger)

	// Create sync client with state manager
	client := sync.New(cfg, logger, stateManager)

	// Create metrics/health server if enabled
	var srv *server.Server
	if cfg.Agent.MetricsPort > 0 {
		srv = server.New(cfg.Agent.MetricsPort, logger)
	}

	return &Agent{
		config:       cfg,
		scanner:      s,
		client:       client,
		stateManager: stateManager,
		logger:       logger,
		server:       srv,
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

	// Set initial metrics
	metrics.SetCertificatesConfigured(len(a.config.Certificates))

	// Start metrics/health server if enabled
	if a.server != nil {
		a.server.Start()
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.server.Shutdown(shutdownCtx); err != nil {
				a.logger.Error("failed to shutdown metrics server", zap.Error(err))
			}
		}()
	}

	// Perform initial scan and sync
	if err := a.scanAndSync(ctx); err != nil {
		a.logger.Error("initial sync failed", zap.Error(err))
		// Continue running even if initial sync fails
	}

	// Mark agent as ready after initial sync
	server.SetReady(true)

	// Set agent info metric (after first sync we have agent_id)
	agentID := a.stateManager.GetAgentID()
	if agentID == "" {
		agentID = "unknown"
	}
	metrics.SetAgentInfo(version.GetVersion(), a.config.Agent.Name, agentID)

	// Setup tickers
	syncTicker := time.NewTicker(a.config.Agent.SyncInterval)
	scanTicker := time.NewTicker(a.config.Agent.ScanInterval)
	defer syncTicker.Stop()
	defer scanTicker.Stop()

	// Setup heartbeat ticker if enabled
	var heartbeatTicker *time.Ticker
	var heartbeatChan <-chan time.Time
	if a.config.Agent.HeartbeatInterval > 0 {
		heartbeatTicker = time.NewTicker(a.config.Agent.HeartbeatInterval)
		heartbeatChan = heartbeatTicker.C
		defer heartbeatTicker.Stop()
		a.logger.Info("heartbeat enabled", zap.Duration("interval", a.config.Agent.HeartbeatInterval))
	}

	// Start uptime counter
	go a.trackUptime(ctx)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("agent stopping")
			server.SetReady(false)
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

		case <-heartbeatChan:
			a.logger.Debug("heartbeat interval triggered")
			if err := a.sendHeartbeat(ctx); err != nil {
				a.logger.Error("heartbeat failed", zap.Error(err))
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

	// Count successes and failures, update metrics
	successCount := 0
	failCount := 0
	scanDuration := time.Since(start).Seconds() / float64(len(a.config.Certificates))

	for _, r := range results {
		portStr := strconv.Itoa(r.Port)

		if r.Success {
			successCount++
			metrics.RecordScanSuccess(r.Hostname, scanDuration)

			// Update certificate metrics
			if r.Certificate != nil {
				daysUntilExpiry := float64(r.Certificate.DaysUntilExpiry)
				expiryTimestamp := float64(r.Certificate.NotAfter.Unix())

				// Determine validity: certificate is valid if it hasn't expired
				valid := r.Certificate.DaysUntilExpiry >= 0

				// Determine chain validity
				chainValid := r.Chain != nil && r.Chain.Valid

				metrics.RecordCertificateMetrics(
					r.Hostname,
					portStr,
					daysUntilExpiry,
					expiryTimestamp,
					valid,
					chainValid,
				)
			}
		} else {
			failCount++
			metrics.RecordScanFailure(r.Hostname, scanDuration)
		}
	}

	// Record scan time for health checks
	server.RecordScan()

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
	duration := time.Since(start).Seconds()

	if err != nil {
		metrics.RecordSyncFailure(duration)
		return err
	}

	// Record successful sync metrics
	metrics.RecordSyncSuccess(duration, resp.Data.Created, resp.Data.Updated, resp.Data.Orphaned)
	server.RecordSync()

	a.logger.Info("sync complete",
		zap.Duration("duration", time.Since(start)),
		zap.String("agent_id", resp.AgentID),
		zap.Int("created", resp.Data.Created),
		zap.Int("updated", resp.Data.Updated),
		zap.Int("unchanged", resp.Data.Unchanged),
		zap.Int("orphaned", resp.Data.Orphaned),
		zap.Int("migrated", resp.Data.Migrated),
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

// sendHeartbeat sends a heartbeat to the CertWatch API
func (a *Agent) sendHeartbeat(ctx context.Context) error {
	start := time.Now()

	// Get last scan and sync times for heartbeat status
	lastScan, _ := server.GetLastScan()
	lastSync, _ := server.GetLastSync()

	err := a.client.Heartbeat(ctx, len(a.config.Certificates), lastScan, lastSync)
	duration := time.Since(start).Seconds()

	if err != nil {
		// Check if agent was deleted from server
		if errors.Is(err, sync.ErrAgentNotFound) {
			a.logger.Warn("agent not found on server (may have been deleted), re-registering")

			// Clear the stale agent ID
			if clearErr := a.client.ClearAgentID(); clearErr != nil {
				a.logger.Error("failed to clear agent ID", zap.Error(clearErr))
			}

			// Trigger a full re-sync to re-register the agent
			if syncErr := a.scanAndSync(ctx); syncErr != nil {
				a.logger.Error("failed to re-register agent", zap.Error(syncErr))
				metrics.RecordHeartbeatFailure(duration)
				return fmt.Errorf("agent deleted, re-registration failed: %w", syncErr)
			}

			a.logger.Info("agent re-registered successfully after deletion",
				zap.String("new_agent_id", a.stateManager.GetAgentID()),
			)

			// Update agent info metric with new agent_id
			metrics.SetAgentInfo(version.GetVersion(), a.config.Agent.Name, a.stateManager.GetAgentID())

			metrics.RecordHeartbeatSuccess(duration)
			return nil
		}

		metrics.RecordHeartbeatFailure(duration)
		return err
	}

	metrics.RecordHeartbeatSuccess(duration)
	a.logger.Debug("heartbeat sent successfully")
	return nil
}

// trackUptime increments the uptime counter every second
func (a *Agent) trackUptime(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			metrics.AgentUptime.Inc()
		}
	}
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
