// Package initcmd provides the interactive init command wizard.
package initcmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/certwatch-app/cw-agent/internal/config"
)

// WizardState holds all collected input during the wizard.
type WizardState struct {
	// Output configuration
	ConfigPath    string
	OverwriteFile bool

	// API configuration
	APIKey      string
	APIEndpoint string
	APITimeout  string

	// Agent configuration
	AgentName    string
	SyncInterval string
	ScanInterval string
	LogLevel     string
	Concurrency  int

	// Certificate configuration
	Certificates []CertificateInput
	CurrentCert  CertificateInput
	AddAnother   bool
}

// CertificateInput represents user input for a certificate.
type CertificateInput struct {
	Hostname string
	PortStr  string
	Tags     string // comma-separated, parsed later
	Notes    string
}

// NewWizardState creates a new WizardState with sensible defaults.
func NewWizardState() *WizardState {
	return &WizardState{
		ConfigPath:   "./certwatch.yaml",
		APIEndpoint:  "https://api.certwatch.app",
		APITimeout:   "30s",
		AgentName:    "",
		SyncInterval: "5m",
		ScanInterval: "1m",
		LogLevel:     "info",
		Concurrency:  10,
		Certificates: make([]CertificateInput, 0),
		CurrentCert: CertificateInput{
			PortStr: "443",
		},
	}
}

// ToConfig converts the wizard state to a config.Config struct.
func (s *WizardState) ToConfig() (*config.Config, error) {
	// Parse API timeout
	timeout, err := time.ParseDuration(s.APITimeout)
	if err != nil {
		timeout = 30 * time.Second
	}

	// Parse sync interval
	syncInterval, err := time.ParseDuration(s.SyncInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid sync interval: %w", err)
	}

	// Parse scan interval
	scanInterval, err := time.ParseDuration(s.ScanInterval)
	if err != nil {
		return nil, fmt.Errorf("invalid scan interval: %w", err)
	}

	// Convert certificates
	certs := make([]config.CertificateConfig, 0, len(s.Certificates))
	for _, c := range s.Certificates {
		port := 443
		if c.PortStr != "" {
			p, err := strconv.Atoi(c.PortStr)
			if err == nil {
				port = p
			}
		}

		tags := parseTags(c.Tags)

		certs = append(certs, config.CertificateConfig{
			Hostname: c.Hostname,
			Port:     port,
			Tags:     tags,
			Notes:    strings.TrimSpace(c.Notes),
		})
	}

	cfg := &config.Config{
		API: config.APIConfig{
			Endpoint: s.APIEndpoint,
			Key:      s.APIKey,
			Timeout:  timeout,
		},
		Agent: config.AgentConfig{
			Name:         s.AgentName,
			LogLevel:     s.LogLevel,
			SyncInterval: syncInterval,
			ScanInterval: scanInterval,
			Concurrency:  s.Concurrency,
		},
		Certificates: certs,
	}

	return cfg, nil
}

// parseTags parses comma-separated tags into a slice.
func parseTags(tagsStr string) []string {
	if strings.TrimSpace(tagsStr) == "" {
		return nil
	}

	parts := strings.Split(tagsStr, ",")
	tags := make([]string, 0, len(parts))
	for _, p := range parts {
		tag := strings.TrimSpace(p)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// ResetCurrentCert resets the current certificate input for the next entry.
func (s *WizardState) ResetCurrentCert() {
	s.CurrentCert = CertificateInput{
		PortStr: "443",
	}
	s.AddAnother = false
}

// SaveCurrentCert saves the current certificate to the list.
func (s *WizardState) SaveCurrentCert() {
	if s.CurrentCert.Hostname != "" {
		s.Certificates = append(s.Certificates, s.CurrentCert)
	}
}
