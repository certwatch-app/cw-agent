package certmanager

import (
	"context"
	"fmt"
	"time"

	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/certwatch-app/cw-agent/internal/certmanager/config"
	"github.com/certwatch-app/cw-agent/internal/certmanager/controller"
	"github.com/certwatch-app/cw-agent/internal/certmanager/metrics"
	"github.com/certwatch-app/cw-agent/internal/certmanager/types"
	"github.com/certwatch-app/cw-agent/internal/state"
	"github.com/certwatch-app/cw-agent/internal/sync"
	"github.com/certwatch-app/cw-agent/internal/version"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(cmapi.AddToScheme(scheme))
}

// Agent is the main cert-manager controller agent
type Agent struct {
	config       *config.Config
	logger       *zap.Logger
	syncClient   *sync.Client
	stateManager *state.Manager
	reconciler   *controller.CertificateReconciler
}

// New creates a new cert-manager agent
func New(cfg *config.Config, stateManager *state.Manager) (*Agent, error) {
	logger := setupLogger(cfg.Agent.LogLevel)

	// Create sync client using the config adapter
	syncCfg := &sync.ClientConfig{
		Endpoint: cfg.API.Endpoint,
		APIKey:   cfg.API.Key,
		Timeout:  cfg.API.Timeout,
	}
	syncClient := sync.NewWithConfig(syncCfg, cfg.Agent.Name, logger, stateManager)

	return &Agent{
		config:       cfg,
		logger:       logger,
		syncClient:   syncClient,
		stateManager: stateManager,
	}, nil
}

// Run starts the agent
func (a *Agent) Run(ctx context.Context) error {
	a.logger.Info("starting cert-manager agent",
		zap.String("version", version.GetVersion()),
		zap.String("cluster", a.config.Agent.ClusterName),
		zap.String("agent_name", a.config.Agent.Name),
	)

	// Set agent info metric
	metrics.AgentInfo.WithLabelValues(version.GetVersion(), a.config.Agent.ClusterName).Set(1)

	// Build manager options
	mgrOpts := ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: fmt.Sprintf(":%d", a.config.Agent.MetricsPort),
		},
		HealthProbeBindAddress: fmt.Sprintf(":%d", a.config.Agent.MetricsPort+1), // Use next port for health
	}

	// Create manager
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		return fmt.Errorf("failed to create manager: %w", err)
	}

	// Add health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add healthz check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to add readyz check: %w", err)
	}

	// Create and register reconciler
	a.reconciler = controller.NewCertificateReconciler(
		mgr.GetClient(),
		mgr.GetScheme(),
		a.logger,
	)
	if err := a.reconciler.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to setup certificate reconciler: %w", err)
	}

	// Start sync loop in background
	go a.syncLoop(ctx)

	// Start heartbeat loop in background if enabled
	if a.config.Agent.HeartbeatInterval > 0 {
		go a.heartbeatLoop(ctx)
	}

	// Start manager (blocking)
	a.logger.Info("starting controller manager",
		zap.Int("metrics_port", a.config.Agent.MetricsPort),
		zap.Int("health_port", a.config.Agent.MetricsPort+1),
		zap.Duration("sync_interval", a.config.Agent.SyncInterval),
	)
	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("manager failed: %w", err)
	}

	return nil
}

func (a *Agent) syncLoop(ctx context.Context) {
	ticker := time.NewTicker(a.config.Agent.SyncInterval)
	defer ticker.Stop()

	// Initial sync after short delay for controller to populate
	time.Sleep(10 * time.Second)
	a.doSync(ctx)

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("sync loop stopped")
			return
		case <-ticker.C:
			a.doSync(ctx)
		}
	}
}

func (a *Agent) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(a.config.Agent.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			a.logger.Info("heartbeat loop stopped")
			return
		case <-ticker.C:
			a.doHeartbeat(ctx)
		}
	}
}

func (a *Agent) doSync(ctx context.Context) {
	start := time.Now()
	certs := a.reconciler.GetCertificates()

	if len(certs) == 0 {
		a.logger.Debug("no certificates to sync")
		return
	}

	// Convert to sync format
	syncCerts := make([]sync.CertManagerCertificate, 0, len(certs))
	for i := range certs {
		syncCerts = append(syncCerts, convertToSyncCert(certs[i]))
	}

	resp, err := a.syncClient.SyncCertManagerCertificates(ctx, a.config.Agent.ClusterName, syncCerts)
	if err != nil {
		a.logger.Error("sync failed", zap.Error(err))
		metrics.SyncTotal.WithLabelValues("error").Inc()
		return
	}

	a.logger.Info("sync completed",
		zap.Int("certificates", len(certs)),
		zap.Int("created", resp.Data.Created),
		zap.Int("updated", resp.Data.Updated),
		zap.Int("unchanged", resp.Data.Unchanged),
		zap.Duration("duration", time.Since(start)),
	)
	metrics.SyncTotal.WithLabelValues("success").Inc()
	metrics.SyncDuration.Observe(time.Since(start).Seconds())
}

func (a *Agent) doHeartbeat(ctx context.Context) {
	certCount := a.reconciler.CertificateCount()
	lastSync := a.stateManager.GetLastSyncAt()

	err := a.syncClient.Heartbeat(ctx, certCount, time.Time{}, lastSync)
	if err != nil {
		a.logger.Warn("heartbeat failed", zap.Error(err))
		metrics.HeartbeatTotal.WithLabelValues("error").Inc()
		return
	}

	a.logger.Debug("heartbeat sent", zap.Int("certificate_count", certCount))
	metrics.HeartbeatTotal.WithLabelValues("success").Inc()
}

func convertToSyncCert(c types.CertificateStatus) sync.CertManagerCertificate {
	return sync.CertManagerCertificate{
		Namespace:      c.Namespace,
		Name:           c.Name,
		SecretName:     c.SecretName,
		CommonName:     c.CommonName,
		DNSNames:       c.DNSNames,
		IssuerName:     c.IssuerName,
		IssuerKind:     c.IssuerKind,
		IssuerGroup:    c.IssuerGroup,
		Ready:          c.Ready,
		ReadyReason:    c.ReadyReason,
		Issuing:        c.Issuing,
		NotBefore:      c.NotBefore,
		NotAfter:       c.NotAfter,
		RenewalTime:    c.RenewalTime,
		Revision:       c.Revision,
		FailedAttempts: c.FailedAttempts,
	}
}

func setupLogger(level string) *zap.Logger {
	var zapLevel zapcore.Level
	switch level {
	case "debug":
		zapLevel = zapcore.DebugLevel
	case "warn":
		zapLevel = zapcore.WarnLevel
	case "error":
		zapLevel = zapcore.ErrorLevel
	default:
		zapLevel = zapcore.InfoLevel
	}

	zapConfig := zap.Config{
		Level:            zap.NewAtomicLevelAt(zapLevel),
		Encoding:         "console",
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		EncoderConfig: zapcore.EncoderConfig{
			TimeKey:        "time",
			LevelKey:       "level",
			NameKey:        "logger",
			CallerKey:      "caller",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeLevel:    zapcore.LowercaseLevelEncoder,
			EncodeDuration: zapcore.SecondsDurationEncoder,
			EncodeCaller:   zapcore.ShortCallerEncoder,
		},
	}

	logger, err := zapConfig.Build()
	if err != nil {
		// Fall back to a basic production logger if config fails
		logger, _ = zap.NewProduction() //nolint:errcheck // fallback logger
	}
	return logger
}
