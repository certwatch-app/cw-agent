package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/config"
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
}

// New creates a new sync Client with state manager for agent ID persistence
func New(cfg *config.Config, logger *zap.Logger, stateManager *state.Manager) *Client {
	return &Client{
		endpoint:          cfg.API.Endpoint,
		apiKey:            cfg.API.Key,
		agentName:         cfg.Agent.Name,
		stateManager:      stateManager,
		heartbeatInterval: cfg.Agent.HeartbeatInterval,
		httpClient: &http.Client{
			Timeout: cfg.API.Timeout,
		},
		logger: logger,
	}
}

// Sync sends certificate data to the CertWatch API
func (c *Client) Sync(ctx context.Context, certs []config.CertificateConfig, results []scanner.ScanResult) (*SyncResponse, error) {
	// Build request payload
	req := c.buildSyncRequest(certs, results)

	// Send request
	resp, err := c.doRequest(ctx, "POST", "/api/v1/agent/sync", req)
	if err != nil {
		return nil, err
	}

	// Persist agent ID and name for future restarts
	if resp.Success && resp.AgentID != "" {
		c.stateManager.SetAgentID(resp.AgentID)
		c.stateManager.SetAgentName(c.agentName)
		c.stateManager.SetLastSyncAt(resp.Data.SyncedAt)

		// Clear previous agent ID after successful migration
		if c.stateManager.GetPreviousAgentID() != "" && resp.Data.Migrated > 0 {
			c.stateManager.ClearPreviousAgentID()
		}

		if err := c.stateManager.Save(); err != nil {
			c.logger.Warn("failed to save state", zap.Error(err))
		}
	}

	return resp, nil
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

func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*SyncResponse, error) {
	url := c.endpoint + path

	var bodyReader io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonData)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	req.Header.Set("User-Agent", fmt.Sprintf("cw-agent/%s", version.GetVersion()))

	c.logger.Debug("sending sync request",
		zap.String("url", url),
		zap.String("method", method),
	)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	c.logger.Debug("received response",
		zap.Int("status", resp.StatusCode),
		zap.Int("body_length", len(respBody)),
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

// GetAgentID returns the persisted agent ID (empty if not yet synced)
func (c *Client) GetAgentID() string {
	return c.stateManager.GetAgentID()
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
