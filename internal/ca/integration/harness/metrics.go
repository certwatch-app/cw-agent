// Package harness provides test infrastructure for CA validation integration tests.
package harness

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// MetricsScraper queries Prometheus metrics endpoint and parses metrics.
// It provides helpers for verifying CA validation metrics in integration tests.
type MetricsScraper struct {
	BaseURL string // e.g., http://localhost:8080
	t       *testing.T
}

// NewMetricsScraper creates a new metrics scraper for the given metrics port.
func NewMetricsScraper(t *testing.T, metricsPort int) *MetricsScraper {
	return &MetricsScraper{
		BaseURL: fmt.Sprintf("http://localhost:%d", metricsPort),
		t:       t,
	}
}

// ScrapeMetrics fetches all metrics from the /metrics endpoint.
// Returns the raw Prometheus text format metrics.
func (m *MetricsScraper) ScrapeMetrics() (string, error) {
	resp, err := http.Get(m.BaseURL + "/metrics")
	if err != nil {
		return "", fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read metrics body: %w", err)
	}

	return string(body), nil
}

// GetCAValidationStatus retrieves the CA validation status metric for a specific hostname:port.
// Returns 1.0 for valid, 0.0 for invalid.
func (m *MetricsScraper) GetCAValidationStatus(hostname string, port int) (float64, error) {
	metrics, err := m.ScrapeMetrics()
	if err != nil {
		return 0, err
	}

	// Parse Prometheus metrics
	// Looking for: certwatch_certificate_ca_validation{hostname="...",port="...",validation_mode="..."} 1
	metricName := "certwatch_certificate_ca_validation"
	value, found := m.findMetric(metrics, metricName, map[string]string{
		"hostname": hostname,
		"port":     fmt.Sprintf("%d", port),
	})

	if !found {
		return 0, fmt.Errorf("metric not found: %s{hostname=%s,port=%d}", metricName, hostname, port)
	}

	return value, nil
}

// GetTrustedRootCN retrieves the trusted root CA Common Name for a specific hostname:port.
func (m *MetricsScraper) GetTrustedRootCN(hostname string, port int) (string, error) {
	metrics, err := m.ScrapeMetrics()
	if err != nil {
		return "", err
	}

	// Looking for: certwatch_certificate_trusted_root_info{hostname="...",port="...",root_cn="..."} 1
	metricName := "certwatch_certificate_trusted_root_info"
	labels, found := m.findMetricLabels(metrics, metricName, map[string]string{
		"hostname": hostname,
		"port":     fmt.Sprintf("%d", port),
	})

	if !found {
		return "", fmt.Errorf("metric not found: %s{hostname=%s,port=%d}", metricName, hostname, port)
	}

	rootCN, ok := labels["root_cn"]
	if !ok {
		return "", fmt.Errorf("root_cn label not found in metric")
	}

	return rootCN, nil
}

// GetValidationMode retrieves the validation mode for a specific hostname:port.
func (m *MetricsScraper) GetValidationMode(hostname string, port int) (string, error) {
	metrics, err := m.ScrapeMetrics()
	if err != nil {
		return "", err
	}

	// Looking for validation_mode label in certwatch_certificate_ca_validation metric
	metricName := "certwatch_certificate_ca_validation"
	labels, found := m.findMetricLabels(metrics, metricName, map[string]string{
		"hostname": hostname,
		"port":     fmt.Sprintf("%d", port),
	})

	if !found {
		return "", fmt.Errorf("metric not found: %s{hostname=%s,port=%d}", metricName, hostname, port)
	}

	validationMode, ok := labels["validation_mode"]
	if !ok {
		return "", fmt.Errorf("validation_mode label not found in metric")
	}

	return validationMode, nil
}

// WaitForMetric polls the metrics endpoint until the specified metric appears with the expected value.
// Returns error if the timeout expires.
func (m *MetricsScraper) WaitForMetric(metricName string, labels map[string]string, expectedValue float64, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		metrics, err := m.ScrapeMetrics()
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		value, found := m.findMetric(metrics, metricName, labels)
		if found && value == expectedValue {
			return nil // Success
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("metric %s with labels %v did not reach expected value %.1f within %v", metricName, labels, expectedValue, timeout)
}

// WaitForCAValidation waits for CA validation status metric to appear.
// This is a convenience wrapper around WaitForMetric for the most common case.
func (m *MetricsScraper) WaitForCAValidation(hostname string, port int, expectedValid bool, timeout time.Duration) error {
	expectedValue := 0.0
	if expectedValid {
		expectedValue = 1.0
	}

	return m.WaitForMetric("certwatch_certificate_ca_validation", map[string]string{
		"hostname": hostname,
		"port":     fmt.Sprintf("%d", port),
	}, expectedValue, timeout)
}

// findMetric searches for a metric with specific labels and returns its value.
// Returns (value, true) if found, (0, false) if not found.
func (m *MetricsScraper) findMetric(metrics string, metricName string, requiredLabels map[string]string) (float64, bool) {
	scanner := bufio.NewScanner(strings.NewReader(metrics))

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Check if line starts with metric name
		if !strings.HasPrefix(line, metricName) {
			continue
		}

		// Parse metric line: metric_name{label1="value1",label2="value2"} 1.0
		if matches := m.parseMetricLine(line, metricName, requiredLabels); matches {
			// Extract value (everything after the last space)
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			value, err := strconv.ParseFloat(parts[len(parts)-1], 64)
			if err != nil {
				continue
			}

			return value, true
		}
	}

	return 0, false
}

// findMetricLabels searches for a metric and returns all its labels.
func (m *MetricsScraper) findMetricLabels(metrics string, metricName string, requiredLabels map[string]string) (map[string]string, bool) {
	scanner := bufio.NewScanner(strings.NewReader(metrics))

	for scanner.Scan() {
		line := scanner.Text()

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		// Check if line starts with metric name
		if !strings.HasPrefix(line, metricName) {
			continue
		}

		// Extract labels
		labels := m.extractLabels(line)
		if labels == nil {
			continue
		}

		// Check if all required labels match
		allMatch := true
		for key, expectedValue := range requiredLabels {
			if actualValue, ok := labels[key]; !ok || actualValue != expectedValue {
				allMatch = false
				break
			}
		}

		if allMatch {
			return labels, true
		}
	}

	return nil, false
}

// parseMetricLine checks if a metric line matches the required labels.
func (m *MetricsScraper) parseMetricLine(line string, metricName string, requiredLabels map[string]string) bool {
	// Extract labels section between { and }
	startIdx := strings.Index(line, "{")
	endIdx := strings.Index(line, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return false
	}

	labelsStr := line[startIdx+1 : endIdx]
	labels := m.parseLabels(labelsStr)

	// Check if all required labels match
	for key, expectedValue := range requiredLabels {
		if actualValue, ok := labels[key]; !ok || actualValue != expectedValue {
			return false
		}
	}

	return true
}

// extractLabels extracts all labels from a metric line.
func (m *MetricsScraper) extractLabels(line string) map[string]string {
	startIdx := strings.Index(line, "{")
	endIdx := strings.Index(line, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil
	}

	labelsStr := line[startIdx+1 : endIdx]
	return m.parseLabels(labelsStr)
}

// parseLabels parses Prometheus label string into a map.
// Input format: label1="value1",label2="value2"
func (m *MetricsScraper) parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)

	// Split by comma
	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		// Split by =
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.Trim(strings.TrimSpace(parts[1]), "\"")
		labels[key] = value
	}

	return labels
}

// AssertMetricEquals asserts that a metric has the expected value (test helper).
func (m *MetricsScraper) AssertMetricEquals(metricName string, labels map[string]string, expectedValue float64) {
	m.t.Helper()

	value, found := m.findMetric(mustScrape(m), metricName, labels)
	if !found {
		m.t.Fatalf("Metric not found: %s with labels %v", metricName, labels)
	}

	if value != expectedValue {
		m.t.Fatalf("Metric %s: expected %.1f, got %.1f", metricName, expectedValue, value)
	}
}

// mustScrape scrapes metrics and fails the test on error.
func mustScrape(m *MetricsScraper) string {
	metrics, err := m.ScrapeMetrics()
	if err != nil {
		m.t.Fatalf("Failed to scrape metrics: %v", err)
	}
	return metrics
}
