package initcmd

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ValidateConfigPath validates the output file path.
func ValidateConfigPath(path string) error {
	if path == "" {
		return fmt.Errorf("config path is required")
	}

	// Check if directory exists or can be created
	dir := filepath.Dir(path)
	if dir != "." && dir != "" {
		info, err := os.Stat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				// Directory doesn't exist, check if we can create it
				return nil // We'll create it during write
			}
			return fmt.Errorf("cannot access directory: %w", err)
		}
		if !info.IsDir() {
			return fmt.Errorf("'%s' is not a directory", dir)
		}
	}

	return nil
}

// ValidateAPIKey validates the API key format.
func ValidateAPIKey(key string) error {
	if key == "" {
		return fmt.Errorf("API key is required")
	}

	if !strings.HasPrefix(key, "cw_") {
		return fmt.Errorf("API key must start with 'cw_'")
	}

	if len(key) < 10 {
		return fmt.Errorf("API key appears too short")
	}

	return nil
}

// ValidateEndpoint validates the API endpoint URL.
func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil // Will use default
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("URL must use http or https")
	}

	if u.Host == "" {
		return fmt.Errorf("URL must include a host")
	}

	return nil
}

// ValidateAgentName validates the agent name.
func ValidateAgentName(name string) error {
	if name == "" {
		return fmt.Errorf("agent name is required")
	}

	if len(name) > 100 {
		return fmt.Errorf("name must be at most 100 characters")
	}

	// Check for invalid characters
	if strings.ContainsAny(name, "\n\r\t") {
		return fmt.Errorf("name cannot contain newlines or tabs")
	}

	return nil
}

// ValidateHostname validates a certificate hostname.
func ValidateHostname(hostname string) error {
	if hostname == "" {
		return fmt.Errorf("hostname is required")
	}

	// Basic hostname validation
	if strings.Contains(hostname, " ") {
		return fmt.Errorf("hostname cannot contain spaces")
	}

	if strings.Contains(hostname, "://") {
		return fmt.Errorf("hostname should not include protocol (use 'example.com' not 'https://example.com')")
	}

	// Check for valid hostname characters
	hostname = strings.ToLower(hostname)
	for _, c := range hostname {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '*') {
			return fmt.Errorf("hostname contains invalid character: '%c'", c)
		}
	}

	return nil
}

// ValidatePort validates a port number string.
func ValidatePort(portStr string) error {
	if portStr == "" {
		return nil // Will use default 443
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("port must be a number")
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535")
	}

	return nil
}

// ValidateTags validates the tags input.
func ValidateTags(tagsStr string) error {
	if tagsStr == "" {
		return nil // Tags are optional
	}

	parts := strings.Split(tagsStr, ",")
	for _, p := range parts {
		tag := strings.TrimSpace(p)
		if len(tag) > 50 {
			return fmt.Errorf("each tag must be at most 50 characters")
		}
	}

	return nil
}

// ValidateNotes validates the notes input.
func ValidateNotes(notes string) error {
	if len(notes) > 500 {
		return fmt.Errorf("notes must be at most 500 characters")
	}
	return nil
}
