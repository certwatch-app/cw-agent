package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestRecordCAValidationResult_ValidCertificate(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "example.com"
	port := "443"
	validationMode := "chain"
	trustedRoot := "CN=Test Root CA"
	valid := true

	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, valid)

	// Check CA validation metric
	expected := `
		# HELP certwatch_certificate_ca_validation CA validation status (1=valid, 0=invalid)
		# TYPE certwatch_certificate_ca_validation gauge
		certwatch_certificate_ca_validation{hostname="example.com",port="443",validation_mode="chain"} 1
	`
	if err := testutil.CollectAndCompare(CertificateCAValidation, strings.NewReader(expected)); err != nil {
		t.Errorf("CA validation metric mismatch: %v", err)
	}

	// Check trusted root metric
	expectedRoot := `
		# HELP certwatch_certificate_trusted_root_info Trusted root CA information (always 1)
		# TYPE certwatch_certificate_trusted_root_info gauge
		certwatch_certificate_trusted_root_info{hostname="example.com",port="443",root_cn="CN=Test Root CA"} 1
	`
	if err := testutil.CollectAndCompare(CertificateTrustedRoot, strings.NewReader(expectedRoot)); err != nil {
		t.Errorf("Trusted root metric mismatch: %v", err)
	}
}

func TestRecordCAValidationResult_InvalidCertificate(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "internal.corp"
	port := "443"
	validationMode := "chain"
	trustedRoot := "" // Empty because validation failed
	valid := false

	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, valid)

	// Check CA validation metric shows invalid (0)
	expected := `
		# HELP certwatch_certificate_ca_validation CA validation status (1=valid, 0=invalid)
		# TYPE certwatch_certificate_ca_validation gauge
		certwatch_certificate_ca_validation{hostname="internal.corp",port="443",validation_mode="chain"} 0
	`
	if err := testutil.CollectAndCompare(CertificateCAValidation, strings.NewReader(expected)); err != nil {
		t.Errorf("CA validation metric mismatch: %v", err)
	}

	// Trusted root metric should not be set (empty trustedRoot)
	count := testutil.CollectAndCount(CertificateTrustedRoot)
	if count != 0 {
		t.Errorf("Expected 0 trusted root metrics, got %d", count)
	}
}

func TestRecordCAValidationResult_EmptyValidationMode(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "example.com"
	port := "443"
	validationMode := "" // Empty validation mode
	trustedRoot := "CN=Test Root CA"
	valid := true

	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, valid)

	// CA validation metric should not be set (empty validationMode)
	count := testutil.CollectAndCount(CertificateCAValidation)
	if count != 0 {
		t.Errorf("Expected 0 CA validation metrics with empty validation_mode, got %d", count)
	}

	// Trusted root metric should still be set
	expectedRoot := `
		# HELP certwatch_certificate_trusted_root_info Trusted root CA information (always 1)
		# TYPE certwatch_certificate_trusted_root_info gauge
		certwatch_certificate_trusted_root_info{hostname="example.com",port="443",root_cn="CN=Test Root CA"} 1
	`
	if err := testutil.CollectAndCompare(CertificateTrustedRoot, strings.NewReader(expectedRoot)); err != nil {
		t.Errorf("Trusted root metric mismatch: %v", err)
	}
}

func TestRecordCAValidationResult_EmptyTrustedRoot(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "dev.local"
	port := "8443"
	validationMode := "none"
	trustedRoot := "" // Empty because validation was skipped
	valid := true

	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, valid)

	// CA validation metric should be set
	expected := `
		# HELP certwatch_certificate_ca_validation CA validation status (1=valid, 0=invalid)
		# TYPE certwatch_certificate_ca_validation gauge
		certwatch_certificate_ca_validation{hostname="dev.local",port="8443",validation_mode="none"} 1
	`
	if err := testutil.CollectAndCompare(CertificateCAValidation, strings.NewReader(expected)); err != nil {
		t.Errorf("CA validation metric mismatch: %v", err)
	}

	// Trusted root metric should not be set (empty trustedRoot)
	count := testutil.CollectAndCount(CertificateTrustedRoot)
	if count != 0 {
		t.Errorf("Expected 0 trusted root metrics with empty trusted_root, got %d", count)
	}
}

func TestRecordCAValidationResult_MultipleCertificates(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	// Record multiple certificates
	testCases := []struct {
		hostname       string
		port           string
		validationMode string
		trustedRoot    string
		valid          bool
	}{
		{"example.com", "443", "chain", "CN=Public Root CA", true},
		{"internal.corp", "443", "chain", "CN=Internal Root CA", true},
		{"untrusted.local", "443", "chain", "", false},
		{"dev.local", "8443", "none", "", true},
	}

	for _, tc := range testCases {
		RecordCAValidationResult(tc.hostname, tc.port, tc.validationMode, tc.trustedRoot, tc.valid)
	}

	// Check CA validation metrics
	expectedCA := `
		# HELP certwatch_certificate_ca_validation CA validation status (1=valid, 0=invalid)
		# TYPE certwatch_certificate_ca_validation gauge
		certwatch_certificate_ca_validation{hostname="dev.local",port="8443",validation_mode="none"} 1
		certwatch_certificate_ca_validation{hostname="example.com",port="443",validation_mode="chain"} 1
		certwatch_certificate_ca_validation{hostname="internal.corp",port="443",validation_mode="chain"} 1
		certwatch_certificate_ca_validation{hostname="untrusted.local",port="443",validation_mode="chain"} 0
	`
	if err := testutil.CollectAndCompare(CertificateCAValidation, strings.NewReader(expectedCA)); err != nil {
		t.Errorf("CA validation metrics mismatch: %v", err)
	}

	// Check trusted root metrics (only 2, since untrusted.local and dev.local have empty roots)
	expectedRoot := `
		# HELP certwatch_certificate_trusted_root_info Trusted root CA information (always 1)
		# TYPE certwatch_certificate_trusted_root_info gauge
		certwatch_certificate_trusted_root_info{hostname="example.com",port="443",root_cn="CN=Public Root CA"} 1
		certwatch_certificate_trusted_root_info{hostname="internal.corp",port="443",root_cn="CN=Internal Root CA"} 1
	`
	if err := testutil.CollectAndCompare(CertificateTrustedRoot, strings.NewReader(expectedRoot)); err != nil {
		t.Errorf("Trusted root metrics mismatch: %v", err)
	}
}

func TestRecordCAValidationResult_LabelsCorrect(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "test.example.com"
	port := "8443"
	validationMode := "basic"
	trustedRoot := "CN=Test Root CA, O=Test Org, C=US"
	valid := true

	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, valid)

	// Verify CA validation metric has correct labels
	metric := CertificateCAValidation.WithLabelValues(hostname, port, validationMode)
	value := testutil.ToFloat64(metric)
	if value != 1 {
		t.Errorf("Expected CA validation metric value 1, got %f", value)
	}

	// Verify trusted root metric has correct labels
	rootMetric := CertificateTrustedRoot.WithLabelValues(hostname, port, trustedRoot)
	rootValue := testutil.ToFloat64(rootMetric)
	if rootValue != 1 {
		t.Errorf("Expected trusted root metric value 1, got %f", rootValue)
	}
}

func TestRecordCAValidationResult_UpdatesExistingMetrics(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()
	CertificateTrustedRoot.Reset()

	hostname := "example.com"
	port := "443"
	validationMode := "chain"
	trustedRoot := "CN=Test Root CA"

	// First record: valid
	RecordCAValidationResult(hostname, port, validationMode, trustedRoot, true)

	metric := CertificateCAValidation.WithLabelValues(hostname, port, validationMode)
	value := testutil.ToFloat64(metric)
	if value != 1 {
		t.Errorf("Expected initial CA validation metric value 1, got %f", value)
	}

	// Second record: invalid (simulating certificate expiry or CA change)
	RecordCAValidationResult(hostname, port, validationMode, "", false)

	value = testutil.ToFloat64(metric)
	if value != 0 {
		t.Errorf("Expected updated CA validation metric value 0, got %f", value)
	}
}

func TestRecordCAValidationResult_DifferentValidationModes(t *testing.T) {
	// Reset metrics before test
	CertificateCAValidation.Reset()

	hostname := "example.com"
	port := "443"

	// Record with different validation modes
	RecordCAValidationResult(hostname, port, "none", "", true)
	RecordCAValidationResult(hostname, port, "basic", "CN=Root", true)
	RecordCAValidationResult(hostname, port, "chain", "CN=Root", true)

	// All three should be recorded as separate metrics
	expectedCA := `
		# HELP certwatch_certificate_ca_validation CA validation status (1=valid, 0=invalid)
		# TYPE certwatch_certificate_ca_validation gauge
		certwatch_certificate_ca_validation{hostname="example.com",port="443",validation_mode="basic"} 1
		certwatch_certificate_ca_validation{hostname="example.com",port="443",validation_mode="chain"} 1
		certwatch_certificate_ca_validation{hostname="example.com",port="443",validation_mode="none"} 1
	`
	if err := testutil.CollectAndCompare(CertificateCAValidation, strings.NewReader(expectedCA)); err != nil {
		t.Errorf("CA validation metrics mismatch for different modes: %v", err)
	}
}

func TestCertificateCAValidationMetricRegistered(t *testing.T) {
	if CertificateCAValidation == nil {
		t.Fatal("CertificateCAValidation metric is nil")
	}

	// Verify metric can be collected (it's registered)
	count := testutil.CollectAndCount(CertificateCAValidation)
	if count < 0 {
		t.Error("Failed to collect CertificateCAValidation metric")
	}
}

func TestCertificateTrustedRootMetricRegistered(t *testing.T) {
	if CertificateTrustedRoot == nil {
		t.Fatal("CertificateTrustedRoot metric is nil")
	}

	count := testutil.CollectAndCount(CertificateTrustedRoot)
	if count < 0 {
		t.Error("Failed to collect CertificateTrustedRoot metric")
	}
}
