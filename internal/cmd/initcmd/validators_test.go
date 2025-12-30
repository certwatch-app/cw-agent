package initcmd

import (
	"testing"
)

func TestValidateAPIKey(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		wantErr bool
	}{
		{"valid key", "cw_live_xxxxxxxxxxxx", false},
		{"valid key long", "cw_test_abcdefghijklmnopqrstuvwxyz1234567890", false},
		{"empty key", "", true},
		{"missing prefix", "xxxxxxxxxxxx", true},
		{"wrong prefix", "sk_live_xxxxxxxxxxxx", true},
		{"too short", "cw_xx", true},
		{"just prefix", "cw_", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAPIKey(tt.key)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAPIKey(%q) error = %v, wantErr %v", tt.key, err, tt.wantErr)
			}
		})
	}
}

func TestValidateEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{"valid https", "https://api.certwatch.app", false},
		{"valid http", "http://localhost:3000", false},
		{"valid with path", "https://api.certwatch.app/v1", false},
		{"empty (uses default)", "", false},
		{"missing scheme", "api.certwatch.app", true},
		{"ftp scheme", "ftp://example.com", true},
		{"invalid url", "not a url", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpoint(tt.endpoint)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateEndpoint(%q) error = %v, wantErr %v", tt.endpoint, err, tt.wantErr)
			}
		})
	}
}

func TestValidateAgentName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid simple", "my-agent", false},
		{"valid with numbers", "prod-agent-01", false},
		{"valid with spaces", "Production Agent", false},
		{"empty", "", true},
		{"too long", string(make([]byte, 101)), true},
		{"exactly 100", string(make([]byte, 100)), false},
		{"with newline", "agent\nname", true},
		{"with tab", "agent\tname", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAgentName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAgentName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateHostname(t *testing.T) {
	tests := []struct {
		name     string
		hostname string
		wantErr  bool
	}{
		{"valid simple", "example.com", false},
		{"valid subdomain", "api.example.com", false},
		{"valid with hyphen", "my-api.example.com", false},
		{"valid wildcard", "*.example.com", false},
		{"valid ip-like", "192.168.1.1", false},
		{"empty", "", true},
		{"with spaces", "api example.com", true},
		{"with protocol", "https://example.com", true},
		{"with invalid char", "api@example.com", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateHostname(tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateHostname(%q) error = %v, wantErr %v", tt.hostname, err, tt.wantErr)
			}
		})
	}
}

func TestValidatePort(t *testing.T) {
	tests := []struct {
		name    string
		portStr string
		wantErr bool
	}{
		{"valid 443", "443", false},
		{"valid 8443", "8443", false},
		{"valid 1", "1", false},
		{"valid 65535", "65535", false},
		{"empty (default)", "", false},
		{"zero", "0", true},
		{"negative", "-1", true},
		{"too high", "65536", true},
		{"not a number", "abc", true},
		{"float", "443.5", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePort(tt.portStr)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePort(%q) error = %v, wantErr %v", tt.portStr, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTags(t *testing.T) {
	tests := []struct {
		name    string
		tags    string
		wantErr bool
	}{
		{"valid single", "production", false},
		{"valid multiple", "production, api, critical", false},
		{"empty", "", false},
		{"with spaces", "  production  ,  api  ", false},
		{"tag too long", string(make([]byte, 51)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTags(tt.tags)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateTags(%q) error = %v, wantErr %v", tt.tags, err, tt.wantErr)
			}
		})
	}
}

func TestValidateNotes(t *testing.T) {
	tests := []struct {
		name    string
		notes   string
		wantErr bool
	}{
		{"valid short", "Main API endpoint", false},
		{"valid long", string(make([]byte, 500)), false},
		{"empty", "", false},
		{"too long", string(make([]byte, 501)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotes(tt.notes)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNotes(%q) error = %v, wantErr %v", tt.notes, err, tt.wantErr)
			}
		})
	}
}

func TestValidateConfigPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"valid relative", "./certwatch.yaml", false},
		{"valid current dir", "certwatch.yaml", false},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfigPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateConfigPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
