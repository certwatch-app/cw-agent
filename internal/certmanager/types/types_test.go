package types

import (
	"encoding/json"
	"testing"
	"time"
)

func TestCertificateStatus_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	notAfter := now.Add(90 * 24 * time.Hour)

	status := CertificateStatus{
		Namespace:      "default",
		Name:           "test-cert",
		SecretName:     "test-secret",
		CommonName:     "example.com",
		DNSNames:       []string{"example.com", "www.example.com"},
		KeyAlgorithm:   "RSA",
		KeySize:        4096,
		IssuerName:     "letsencrypt",
		IssuerKind:     "ClusterIssuer",
		IssuerGroup:    "cert-manager.io",
		Ready:          true,
		ReadyReason:    "Ready",
		ReadyMessage:   "Certificate is up to date",
		Issuing:        false,
		NotAfter:       &notAfter,
		Revision:       2,
		FailedAttempts: 0,
	}

	// Marshal to JSON
	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	// Unmarshal back
	var decoded CertificateStatus
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	// Verify fields
	if decoded.Namespace != status.Namespace {
		t.Errorf("Namespace = %v, want %v", decoded.Namespace, status.Namespace)
	}
	if decoded.Name != status.Name {
		t.Errorf("Name = %v, want %v", decoded.Name, status.Name)
	}
	if decoded.SecretName != status.SecretName {
		t.Errorf("SecretName = %v, want %v", decoded.SecretName, status.SecretName)
	}
	if decoded.CommonName != status.CommonName {
		t.Errorf("CommonName = %v, want %v", decoded.CommonName, status.CommonName)
	}
	if len(decoded.DNSNames) != len(status.DNSNames) {
		t.Errorf("len(DNSNames) = %v, want %v", len(decoded.DNSNames), len(status.DNSNames))
	}
	if decoded.KeyAlgorithm != status.KeyAlgorithm {
		t.Errorf("KeyAlgorithm = %v, want %v", decoded.KeyAlgorithm, status.KeyAlgorithm)
	}
	if decoded.KeySize != status.KeySize {
		t.Errorf("KeySize = %v, want %v", decoded.KeySize, status.KeySize)
	}
	if decoded.IssuerName != status.IssuerName {
		t.Errorf("IssuerName = %v, want %v", decoded.IssuerName, status.IssuerName)
	}
	if decoded.IssuerKind != status.IssuerKind {
		t.Errorf("IssuerKind = %v, want %v", decoded.IssuerKind, status.IssuerKind)
	}
	if decoded.IssuerGroup != status.IssuerGroup {
		t.Errorf("IssuerGroup = %v, want %v", decoded.IssuerGroup, status.IssuerGroup)
	}
	if decoded.Ready != status.Ready {
		t.Errorf("Ready = %v, want %v", decoded.Ready, status.Ready)
	}
	if decoded.ReadyReason != status.ReadyReason {
		t.Errorf("ReadyReason = %v, want %v", decoded.ReadyReason, status.ReadyReason)
	}
	if decoded.Issuing != status.Issuing {
		t.Errorf("Issuing = %v, want %v", decoded.Issuing, status.Issuing)
	}
	if decoded.Revision != status.Revision {
		t.Errorf("Revision = %v, want %v", decoded.Revision, status.Revision)
	}
	if decoded.FailedAttempts != status.FailedAttempts {
		t.Errorf("FailedAttempts = %v, want %v", decoded.FailedAttempts, status.FailedAttempts)
	}
}

func TestCertificateStatus_OmitEmptyFields(t *testing.T) {
	// Create a minimal status with only required fields
	status := CertificateStatus{
		Namespace:  "default",
		Name:       "test-cert",
		SecretName: "test-secret",
		IssuerName: "issuer",
		IssuerKind: "Issuer",
	}

	data, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	jsonStr := string(data)

	// These fields should be omitted (omitempty)
	omittedFields := []string{
		"common_name",
		"dns_names",
		"key_algorithm",
		"key_size",
		"issuer_group",
		"ready_reason",
		"ready_message",
		"issuing_reason",
		"not_before",
		"not_after",
		"renewal_time",
		"last_transition",
		"last_failure_time",
	}

	for _, field := range omittedFields {
		if contains(jsonStr, `"`+field+`"`) {
			t.Errorf("JSON should not contain %q for empty value", field)
		}
	}

	// These fields should always be present
	requiredFields := []string{
		"namespace",
		"name",
		"secret_name",
		"issuer_name",
		"issuer_kind",
		"ready",
		"issuing",
		"revision",
		"failed_attempts",
	}

	for _, field := range requiredFields {
		if !contains(jsonStr, `"`+field+`"`) {
			t.Errorf("JSON should contain %q", field)
		}
	}
}

func TestCertManagerSyncPayload_JSONSerialization(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	payload := CertManagerSyncPayload{
		EventType:   "certmanager.certificate_sync",
		Timestamp:   now,
		AgentID:     "agent-123",
		ClusterName: "production",
		Certificates: []CertificateStatus{
			{
				Namespace:  "default",
				Name:       "cert1",
				SecretName: "secret1",
				IssuerName: "issuer",
				IssuerKind: "Issuer",
				Ready:      true,
			},
			{
				Namespace:  "staging",
				Name:       "cert2",
				SecretName: "secret2",
				IssuerName: "issuer",
				IssuerKind: "ClusterIssuer",
				Ready:      false,
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded CertManagerSyncPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	if decoded.EventType != payload.EventType {
		t.Errorf("EventType = %v, want %v", decoded.EventType, payload.EventType)
	}
	if decoded.AgentID != payload.AgentID {
		t.Errorf("AgentID = %v, want %v", decoded.AgentID, payload.AgentID)
	}
	if decoded.ClusterName != payload.ClusterName {
		t.Errorf("ClusterName = %v, want %v", decoded.ClusterName, payload.ClusterName)
	}
	if len(decoded.Certificates) != 2 {
		t.Errorf("len(Certificates) = %v, want 2", len(decoded.Certificates))
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
