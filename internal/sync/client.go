package sync

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/metrics"
	"github.com/certwatch-app/cw-agent/internal/scanner"
	"github.com/certwatch-app/cw-agent/internal/state"
	"github.com/certwatch-app/cw-agent/internal/version"
)

// Client handles communication with the CertWatch API
type Client struct {
	endpoint          string
	apiKey            string
	httpClient        *http.Client
	logger            *zap.Logger
	agentName         string
	stateManager      *state.Manager
	heartbeatInterval time.Duration
	retryConfig       config.RetryConfig
	circuitBreaker    *CircuitBreaker
}

// New creates a new sync Client with state manager for agent ID persistence
func New(cfg *config.Config, logger *zap.Logger, stateManager *state.Manager) *Client {
	client := &Client{
		endpoint:          cfg.API.Endpoint,
		apiKey:            cfg.API.Key,
		agentName:         cfg.Agent.Name,
		stateManager:      stateManager,
		heartbeatInterval: cfg.Agent.HeartbeatInterval,
		retryConfig:       cfg.API.Retry,
		httpClient: &http.Client{
			Timeout: cfg.API.Timeout,
		},
		logger: logger,
	}

	// Initialize circuit breaker if enabled
	if cfg.API.CircuitBreaker.Enabled {
		client.circuitBreaker = NewCircuitBreaker(
			cfg.API.CircuitBreaker.MaxFailures,
			cfg.API.CircuitBreaker.Timeout,
			logger.With(zap.String("component", "circuit_breaker")),
		)
	}

	return client
}

// Sync sends certificate data to the CertWatch API with retry logic and circuit breaker
func (c *Client) Sync(ctx context.Context, certs []config.CertificateConfig, results []scanner.ScanResult) (*SyncResponse, error) {
	// Wrap with circuit breaker if enabled
	if c.circuitBreaker != nil {
		var resp *SyncResponse
		err := c.circuitBreaker.Call(ctx, func() error {
			var syncErr error
			if c.retryConfig.MaxRetries > 0 {
				resp, syncErr = c.syncWithRetry(ctx, certs, results)
			} else {
				resp, syncErr = c.syncInternal(ctx, certs, results)
			}
			return syncErr
		})
		return resp, err
	}

	// No circuit breaker - use retry logic if configured
	if c.retryConfig.MaxRetries > 0 {
		return c.syncWithRetry(ctx, certs, results)
	}

	return c.syncInternal(ctx, certs, results)
}

// syncInternal performs a single sync operation without retry or circuit breaker
func (c *Client) syncInternal(ctx context.Context, certs []config.CertificateConfig, results []scanner.ScanResult) (*SyncResponse, error) {
	startTime := time.Now()

	// Generate correlation ID for request tracking
	correlationID := uuid.New().String()

	c.logger.Info("starting sync",
		zap.String("correlation_id", correlationID),
		zap.Int("certificate_count", len(certs)),
		zap.String("agent_id", c.stateManager.GetAgentID()),
	)

	// Build request payload
	req := c.buildSyncRequest(certs, results)

	// Send request with correlation ID
	resp, err := c.doRequestWithCorrelation(ctx, "POST", "/api/v1/agent/sync", req, correlationID)
	if err != nil {
		c.logger.Error("sync failed",
			zap.String("correlation_id", correlationID),
			zap.Duration("duration", time.Since(startTime)),
			zap.Error(err),
		)
		return nil, err
	}

	// Persist agent ID and name for future restarts
	if resp.Success && resp.AgentID != "" {
		c.persistAgentState(resp)
	}

	duration := time.Since(startTime).Seconds()

	// Record metrics
	metrics.RecordSyncSuccess(duration, resp.Data.Created, resp.Data.Updated, resp.Data.Orphaned)

	c.logger.Info("sync completed",
		zap.String("correlation_id", correlationID),
		zap.String("agent_id", resp.AgentID),
		zap.Int("created", resp.Data.Created),
		zap.Int("updated", resp.Data.Updated),
		zap.Int("unchanged", resp.Data.Unchanged),
		zap.Duration("duration", time.Since(startTime)),
	)

	return resp, nil
}

// syncWithRetry sends certificate data with exponential backoff retry logic
func (c *Client) syncWithRetry(ctx context.Context, certs []config.CertificateConfig, results []scanner.ScanResult) (*SyncResponse, error) {
	var lastErr error
	backoff := c.retryConfig.InitialBackoff

	for attempt := 0; attempt <= c.retryConfig.MaxRetries; attempt++ {
		// Perform sync
		resp, err := c.syncInternal(ctx, certs, results)
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on client errors (4xx) or context cancellation
		if isClientError(err) || ctx.Err() != nil {
			return nil, err
		}

		// Don't sleep on the last attempt
		if attempt < c.retryConfig.MaxRetries {
			// Record retry metric
			metrics.RecordSyncRetry(attempt + 1)

			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Float64() * float64(backoff) * 0.1)
			sleepDuration := backoff + jitter

			c.logger.Warn("sync failed, retrying",
				zap.Int("attempt", attempt+1),
				zap.Int("maxRetries", c.retryConfig.MaxRetries),
				zap.Duration("backoff", sleepDuration),
				zap.Error(err),
			)

			// Sleep with context cancellation support
			select {
			case <-time.After(sleepDuration):
				// Continue to next attempt
			case <-ctx.Done():
				return nil, ctx.Err()
			}

			// Exponential backoff with cap
			backoff = time.Duration(float64(backoff) * c.retryConfig.Multiplier)
			if backoff > c.retryConfig.MaxBackoff {
				backoff = c.retryConfig.MaxBackoff
			}
		}
	}

	return nil, fmt.Errorf("sync failed after %d retries: %w", c.retryConfig.MaxRetries, lastErr)
}

// persistAgentState saves agent ID and related metadata to state
func (c *Client) persistAgentState(resp *SyncResponse) {
	c.stateManager.SetAgentID(resp.AgentID)
	c.stateManager.SetAgentName(c.agentName)
	c.stateManager.SetLastSyncAt(resp.Data.SyncedAt) // SyncedAt is time.Time

	// Clear previous agent ID after successful migration
	if c.stateManager.GetPreviousAgentID() != "" && resp.Data.Migrated > 0 {
		c.stateManager.ClearPreviousAgentID()
	}

	if err := c.stateManager.Save(); err != nil {
		c.logger.Warn("failed to save state", zap.Error(err))
	}
}

// isClientError returns true for 4xx HTTP errors that should not be retried
func isClientError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	// Check for common client error status codes
	return contains(errMsg, "status 400") ||
		contains(errMsg, "status 401") ||
		contains(errMsg, "status 403") ||
		contains(errMsg, "status 404") ||
		contains(errMsg, "status 409") ||
		contains(errMsg, "status 422")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Heartbeat sends a heartbeat to the CertWatch API
func (c *Client) Heartbeat(ctx context.Context, certCount int, lastScan, lastSync time.Time) error {
	agentID := c.stateManager.GetAgentID()
	if agentID == "" {
		// No agent ID yet, skip heartbeat until first sync
		return nil
	}

	req := &HeartbeatRequest{
		AgentID:          agentID,
		AgentName:        c.agentName,
		AgentVersion:     version.GetVersion(),
		CertificateCount: certCount,
		Status:           "healthy",
	}

	// Add last scan time if available
	if !lastScan.IsZero() {
		req.LastScanAt = &lastScan
	}

	// Add last sync time if available
	if !lastSync.IsZero() {
		req.LastSyncAt = &lastSync
	}

	_, err := c.doHeartbeatRequest(ctx, req)
	return err
}

// ErrAgentNotFound is returned when the agent ID is no longer valid on the server
var ErrAgentNotFound = fmt.Errorf("agent not found")

func (c *Client) doHeartbeatRequest(ctx context.Context, body *HeartbeatRequest) (*HeartbeatResponse, error) {
	url := c.endpoint + "/api/v1/agent/heartbeat"

	jsonData, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal heartbeat request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create heartbeat request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", fmt.Sprintf("cw-agent/%s", version.GetVersion()))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("heartbeat request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read heartbeat response: %w", err)
	}

	// Handle 404 - agent was deleted from server
	if resp.StatusCode == 404 {
		return nil, ErrAgentNotFound
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("heartbeat API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var heartbeatResp HeartbeatResponse
	if err := json.Unmarshal(respBody, &heartbeatResp); err != nil {
		return nil, fmt.Errorf("failed to parse heartbeat response: %w", err)
	}

	return &heartbeatResp, nil
}

func (c *Client) buildSyncRequest(certs []config.CertificateConfig, results []scanner.ScanResult) *SyncRequest {
	// Build a map of scan results by hostname:port
	resultMap := make(map[string]*scanner.ScanResult)
	for i := range results {
		key := fmt.Sprintf("%s:%d", results[i].Hostname, results[i].Port)
		resultMap[key] = &results[i]
	}

	// Build certificate sync data
	certData := make([]CertificateSyncData, 0, len(certs))
	for _, cert := range certs {
		key := fmt.Sprintf("%s:%d", cert.Hostname, cert.Port)
		data := CertificateSyncData{
			Hostname: cert.Hostname,
			Port:     cert.Port,
			Tags:     cert.Tags,
			Notes:    cert.Notes,
		}

		// Add scan results if available
		if result, ok := resultMap[key]; ok {
			scannedAt := result.ScannedAt
			data.LastCheckAt = &scannedAt

			if result.Success && result.Certificate != nil {
				info := result.Certificate
				data.Subject = info.Subject
				data.Issuer = info.Issuer
				data.IssuerOrg = info.IssuerOrg
				data.SerialNumber = info.SerialNumber
				data.FingerprintSHA256 = info.FingerprintSHA256
				data.NotBefore = &info.NotBefore
				data.NotAfter = &info.NotAfter
				data.SANList = info.SANList

				if result.Chain != nil {
					data.ChainValid = &result.Chain.Valid
					for _, issue := range result.Chain.Issues {
						data.ChainIssues = append(data.ChainIssues, ChainIssueData{
							Type:             issue.Type,
							Message:          issue.Message,
							CertificateIndex: issue.CertificateIndex,
						})
					}

					// Add CA validation results if available
					if result.Chain.ValidationMode != "" {
						data.ValidationMode = result.Chain.ValidationMode
					}
					if result.Chain.ValidationError != "" {
						data.ValidationError = result.Chain.ValidationError
					}
					if result.Chain.TrustedRoot != "" {
						data.TrustedRoot = result.Chain.TrustedRoot
					}
				}
			} else if result.Error != "" {
				data.LastError = result.Error
			}
		}

		certData = append(certData, data)
	}

	hostname := getHostname()

	// Calculate heartbeat interval in seconds (0 if disabled)
	heartbeatSeconds := 0
	if c.heartbeatInterval > 0 {
		heartbeatSeconds = int(c.heartbeatInterval.Seconds())
	}

	return &SyncRequest{
		AgentID:                  c.stateManager.GetAgentID(),
		PreviousAgentID:          c.stateManager.GetPreviousAgentID(),
		AgentName:                c.agentName,
		AgentVersion:             version.GetVersion(),
		AgentHost:                hostname,
		HeartbeatIntervalSeconds: heartbeatSeconds,
		Certificates:             certData,
	}
}

// doRequestWithCorrelation sends a request with correlation ID for distributed tracing
func (c *Client) doRequestWithCorrelation(ctx context.Context, method, path string, body interface{}, correlationID string) (*SyncResponse, error) {
	return c.doRequestInternal(ctx, method, path, body, correlationID)
}

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*SyncResponse, error) {
	return c.doRequestInternal(ctx, method, path, body, "")
}

func (c *Client) doRequestInternal(ctx context.Context, method, path string, body interface{}, correlationID string) (*SyncResponse, error) {
	url := c.endpoint + path

	var bodyReader io.Reader
	compressed := false
	uncompressedSize := 0

	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		uncompressedSize = len(jsonData)

		// Record payload size metric
		metrics.RecordSyncPayloadSize(uncompressedSize)

		// Compress if payload > 1KB
		if len(jsonData) > 1024 {
			var buf bytes.Buffer
			gzWriter := gzip.NewWriter(&buf)
			if _, writeErr := gzWriter.Write(jsonData); writeErr == nil {
				if closeErr := gzWriter.Close(); closeErr == nil {
					bodyReader = &buf
					compressed = true
					c.logger.Debug("compressed request body",
						zap.Int("uncompressed_bytes", uncompressedSize),
						zap.Int("compressed_bytes", buf.Len()),
						zap.Float64("ratio", float64(buf.Len())/float64(uncompressedSize)),
					)
				}
			}
			// If compression fails, fall back to uncompressed
			if !compressed {
				bodyReader = bytes.NewReader(jsonData)
			}
		} else {
			bodyReader = bytes.NewReader(jsonData)
		}
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", fmt.Sprintf("cw-agent/%s", version.GetVersion()))
	req.Header.Set("Accept-Encoding", "gzip") // Tell server we accept gzip responses

	// Add correlation ID for distributed tracing
	if correlationID != "" {
		req.Header.Set("X-Correlation-ID", correlationID)
	}

	if compressed {
		req.Header.Set("Content-Encoding", "gzip")
	}

	c.logger.Debug("sending sync request",
		zap.String("url", url),
		zap.String("method", method),
		zap.Bool("compressed", compressed),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle gzip response
	var reader io.Reader = resp.Body
	if resp.Header.Get("Content-Encoding") == "gzip" {
		gzReader, gzErr := gzip.NewReader(resp.Body)
		if gzErr != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", gzErr)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	respBody, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("received response",
		zap.Int("status", resp.StatusCode),
		zap.Int("body_length", len(respBody)),
		zap.Bool("response_compressed", resp.Header.Get("Content-Encoding") == "gzip"),
	)

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   *APIError `json:"error"`
			Success bool      `json:"success"`
		}
		if unmarshalErr := json.Unmarshal(respBody, &errResp); unmarshalErr == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var syncResp SyncResponse
	if err := json.Unmarshal(respBody, &syncResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &syncResp, nil
}

// MaxCertificatesPerBatch is the maximum number of certificates to sync in a single request
const MaxCertificatesPerBatch = 100

// AggregatedSyncResponse contains aggregated results from multiple batch syncs
type AggregatedSyncResponse struct {
	AgentID        string
	TotalCreated   int
	TotalUpdated   int
	TotalUnchanged int
	TotalOrphaned  int
	TotalMigrated  int
	Errors         []SyncError
	SyncedAt       time.Time
}

// SyncInBatches splits certificates into batches and syncs them separately
// This is useful for large deployments with hundreds of certificates
func (c *Client) SyncInBatches(ctx context.Context, certs []config.CertificateConfig, results []scanner.ScanResult) (*AggregatedSyncResponse, error) {
	totalCerts := len(certs)
	if totalCerts <= MaxCertificatesPerBatch {
		// No batching needed - use regular sync
		resp, err := c.Sync(ctx, certs, results)
		if err != nil {
			return nil, err
		}
		return &AggregatedSyncResponse{
			AgentID:        resp.AgentID,
			TotalCreated:   resp.Data.Created,
			TotalUpdated:   resp.Data.Updated,
			TotalUnchanged: resp.Data.Unchanged,
			TotalOrphaned:  resp.Data.Orphaned,
			TotalMigrated:  resp.Data.Migrated,
			Errors:         resp.Data.Errors,
			SyncedAt:       resp.Data.SyncedAt,
		}, nil
	}

	batches := (totalCerts + MaxCertificatesPerBatch - 1) / MaxCertificatesPerBatch
	c.logger.Info("syncing in batches",
		zap.Int("total_certificates", totalCerts),
		zap.Int("batch_size", MaxCertificatesPerBatch),
		zap.Int("num_batches", batches),
	)

	aggregated := &AggregatedSyncResponse{
		Errors: []SyncError{},
	}

	// Build result map for quick lookup
	resultMap := make(map[string]*scanner.ScanResult)
	for i := range results {
		key := fmt.Sprintf("%s:%d", results[i].Hostname, results[i].Port)
		resultMap[key] = &results[i]
	}

	for i := 0; i < batches; i++ {
		start := i * MaxCertificatesPerBatch
		end := start + MaxCertificatesPerBatch
		if end > totalCerts {
			end = totalCerts
		}

		batch := certs[start:end]

		// Filter results for this batch
		batchResults := make([]scanner.ScanResult, 0, len(batch))
		for _, cert := range batch {
			key := fmt.Sprintf("%s:%d", cert.Hostname, cert.Port)
			if result, ok := resultMap[key]; ok {
				batchResults = append(batchResults, *result)
			}
		}

		c.logger.Info("syncing batch",
			zap.Int("batch", i+1),
			zap.Int("total_batches", batches),
			zap.Int("batch_size", len(batch)),
		)

		resp, err := c.Sync(ctx, batch, batchResults)
		if err != nil {
			return nil, fmt.Errorf("batch %d/%d failed: %w", i+1, batches, err)
		}

		// Aggregate results
		if aggregated.AgentID == "" {
			aggregated.AgentID = resp.AgentID
		}
		aggregated.TotalCreated += resp.Data.Created
		aggregated.TotalUpdated += resp.Data.Updated
		aggregated.TotalUnchanged += resp.Data.Unchanged
		aggregated.TotalMigrated += resp.Data.Migrated
		aggregated.Errors = append(aggregated.Errors, resp.Data.Errors...)
		aggregated.SyncedAt = resp.Data.SyncedAt

		// Note: Orphaning only happens on the last batch
		if i == batches-1 {
			aggregated.TotalOrphaned = resp.Data.Orphaned
		}

		// Small delay between batches to avoid overwhelming API
		if i < batches-1 {
			select {
			case <-time.After(100 * time.Millisecond):
				// Continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	c.logger.Info("batch sync completed",
		zap.Int("total_created", aggregated.TotalCreated),
		zap.Int("total_updated", aggregated.TotalUpdated),
		zap.Int("total_unchanged", aggregated.TotalUnchanged),
		zap.Int("total_errors", len(aggregated.Errors)),
	)

	return aggregated, nil
}

// GetAgentID returns the persisted agent ID (empty if not yet synced)
func (c *Client) GetAgentID() string {
	return c.stateManager.GetAgentID()
}

// ClearAgentID removes the stored agent ID (used when agent is deleted from server)
func (c *Client) ClearAgentID() error {
	c.stateManager.ClearAgentID()
	return c.stateManager.Save()
}

func getHostname() string {
	// Try to get hostname from environment or OS
	// This is a simplified version - could be enhanced
	return ""
}

// ClientConfig holds configuration for creating a sync client without the full config package
type ClientConfig struct {
	Endpoint string
	APIKey   string
	Timeout  time.Duration
}

// NewWithConfig creates a new sync Client with explicit configuration
// This is used by the cert-manager controller which has its own config package
func NewWithConfig(cfg *ClientConfig, agentName string, logger *zap.Logger, stateManager *state.Manager) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &Client{
		endpoint:     cfg.Endpoint,
		apiKey:       cfg.APIKey,
		agentName:    agentName,
		stateManager: stateManager,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// SyncCertManagerCertificates syncs cert-manager certificates to the API
func (c *Client) SyncCertManagerCertificates(ctx context.Context, clusterName string, certs []CertManagerCertificate) (*CertManagerSyncResponse, error) {
	req := &CertManagerSyncRequest{
		AgentID:      c.stateManager.GetAgentID(),
		AgentName:    c.agentName,
		AgentVersion: version.GetVersion(),
		ClusterName:  clusterName,
		Certificates: certs,
	}

	url := c.endpoint + "/api/v1/agent/certmanager/sync"

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("User-Agent", fmt.Sprintf("cw-agent-certmanager/%s", version.GetVersion()))

	c.logger.Debug("sending certmanager sync request",
		zap.String("url", url),
		zap.Int("certificates", len(certs)),
		zap.Bool("api_key_present", c.apiKey != ""),
		zap.Int("api_key_length", len(c.apiKey)),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("received response",
		zap.Int("status", resp.StatusCode),
		zap.Int("body_length", len(body)),
	)

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   *APIError `json:"error"`
			Success bool      `json:"success"`
		}
		if unmarshalErr := json.Unmarshal(body, &errResp); unmarshalErr == nil && errResp.Error != nil {
			return nil, fmt.Errorf("API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	var syncResp CertManagerSyncResponse
	if err := json.Unmarshal(body, &syncResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Persist agent ID for future syncs
	if syncResp.Success && syncResp.AgentID != "" {
		c.stateManager.SetAgentID(syncResp.AgentID)
		c.stateManager.SetAgentName(c.agentName)
		c.stateManager.SetLastSyncAt(syncResp.Data.SyncedAt)
		if err := c.stateManager.Save(); err != nil {
			c.logger.Warn("failed to save state", zap.Error(err))
		}
	}

	return &syncResp, nil
}

// SyncCertManagerEvents syncs cert-manager events to the API (Phase 2)
func (c *Client) SyncCertManagerEvents(ctx context.Context, clusterName string, events []CertManagerEvent) error {
	if len(events) == 0 {
		return nil
	}

	req := &CertManagerEventSyncRequest{
		AgentID:     c.stateManager.GetAgentID(),
		AgentName:   c.agentName,
		ClusterName: clusterName,
		Events:      events,
	}

	url := c.endpoint + "/api/v1/agent/certmanager/events"

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("User-Agent", fmt.Sprintf("cw-agent-certmanager/%s", version.GetVersion()))

	c.logger.Debug("sending certmanager event sync request",
		zap.String("url", url),
		zap.Int("events", len(events)),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("received response",
		zap.Int("status", resp.StatusCode),
		zap.Int("body_length", len(body)),
	)

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   *APIError `json:"error"`
			Success bool      `json:"success"`
		}
		if unmarshalErr := json.Unmarshal(body, &errResp); unmarshalErr == nil && errResp.Error != nil {
			return fmt.Errorf("API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// SyncCertManagerRequests syncs cert-manager CertificateRequests to the API (Phase 2)
func (c *Client) SyncCertManagerRequests(ctx context.Context, clusterName string, requests []CertManagerRequest) error {
	if len(requests) == 0 {
		return nil
	}

	req := &CertManagerRequestSyncRequest{
		AgentID:     c.stateManager.GetAgentID(),
		AgentName:   c.agentName,
		ClusterName: clusterName,
		Requests:    requests,
	}

	url := c.endpoint + "/api/v1/agent/certmanager/requests"

	jsonData, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Key", c.apiKey)
	httpReq.Header.Set("User-Agent", fmt.Sprintf("cw-agent-certmanager/%s", version.GetVersion()))

	c.logger.Debug("sending certmanager request sync",
		zap.String("url", url),
		zap.Int("requests", len(requests)),
	)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	c.logger.Debug("received response",
		zap.Int("status", resp.StatusCode),
		zap.Int("body_length", len(body)),
	)

	if resp.StatusCode >= 400 {
		var errResp struct {
			Error   *APIError `json:"error"`
			Success bool      `json:"success"`
		}
		if unmarshalErr := json.Unmarshal(body, &errResp); unmarshalErr == nil && errResp.Error != nil {
			return fmt.Errorf("API error (%s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
