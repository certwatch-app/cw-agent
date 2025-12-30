package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
	"github.com/certwatch-app/cw-agent/internal/version"
)

// Client handles communication with the CertWatch API
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
	logger     *zap.Logger
	agentName  string
	agentID    string // Cached after first sync
}

// New creates a new sync Client
func New(cfg *config.Config, logger *zap.Logger) *Client {
	return &Client{
		endpoint:  cfg.API.Endpoint,
		apiKey:    cfg.API.Key,
		agentName: cfg.Agent.Name,
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

	// Cache agent ID for future requests
	if resp.Success && resp.AgentID != "" {
		c.agentID = resp.AgentID
	}

	return resp, nil
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

	return &SyncRequest{
		AgentID:      c.agentID,
		AgentName:    c.agentName,
		AgentVersion: version.GetVersion(),
		AgentHost:    hostname,
		Certificates: certData,
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

// GetAgentID returns the cached agent ID (empty if not yet synced)
func (c *Client) GetAgentID() string {
	return c.agentID
}

func getHostname() string {
	// Try to get hostname from environment or OS
	// This is a simplified version - could be enhanced
	return ""
}
