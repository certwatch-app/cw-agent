package ca

import (
	"context"
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestSecretSource_LoadSingleCA tests loading a single CA from Secret.
func TestSecretSource_LoadSingleCA(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "Secret Test CA")

	// Create Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(testCA),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "test-secret", "ca.crt", loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA from Secret: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	// Verify source metadata
	expectedID := "secret:default/test-secret#ca.crt"
	if source.ID() != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, source.ID())
	}

	if source.Type() != "secret" {
		t.Errorf("Expected type 'secret', got %s", source.Type())
	}

	if source.GetNamespace() != "default" {
		t.Errorf("Expected namespace 'default', got %s", source.GetNamespace())
	}

	if source.GetName() != "test-secret" {
		t.Errorf("Expected name 'test-secret', got %s", source.GetName())
	}
}

// TestSecretSource_LoadMultipleCAs tests loading multiple CAs from Secret.
func TestSecretSource_LoadMultipleCAs(t *testing.T) {
	// Generate multiple test CAs
	ca1 := generateTestCAHelper(t, "First CA")
	ca2 := generateTestCAHelper(t, "Second CA")
	bundleContent := ca1 + "\n" + ca2

	// Create Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca-bundle.crt": []byte(bundleContent),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "multi-secret", "ca-bundle.crt", loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA bundle from Secret: %v", err)
	}

	if len(certs) != 2 {
		t.Errorf("Expected 2 certificates, got %d", len(certs))
	}
}

// TestSecretSource_SecretNotFound tests error handling when Secret doesn't exist.
func TestSecretSource_SecretNotFound(t *testing.T) {
	// Create empty client
	fakeClient := newFakeK8sClient(t)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "missing-secret", "ca.crt", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for non-existent Secret, got nil")
	}

	// Error should mention the Secret
	if err != nil && err.Error()[:len("failed to fetch Secret")] != "failed to fetch Secret" {
		t.Errorf("Expected error about fetching Secret, got: %v", err)
	}
}

// TestSecretSource_KeyNotFound tests error handling when key doesn't exist in Secret.
func TestSecretSource_KeyNotFound(t *testing.T) {
	// Create Secret without the expected key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"other-key": []byte("some data"),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source looking for missing key
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "test-secret", "ca.crt", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for missing key, got nil")
	}

	// Error should mention the key
	if err != nil && err.Error()[:len("key ca.crt not found")] != "key ca.crt not found" {
		t.Errorf("Expected error about key not found, got: %v", err)
	}
}

// TestSecretSource_EmptyKeyValue tests error handling for empty key value.
func TestSecretSource_EmptyKeyValue(t *testing.T) {
	// Create Secret with empty key
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(""),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "empty-secret", "ca.crt", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for empty key value, got nil")
	}

	// Error should mention empty key
	if err != nil && err.Error()[:len("key ca.crt")] != "key ca.crt" {
		t.Errorf("Expected error about empty key, got: %v", err)
	}
}

// TestSecretSource_InvalidPEM tests error handling for invalid PEM data.
func TestSecretSource_InvalidPEM(t *testing.T) {
	invalidPEM := `-----BEGIN CERTIFICATE-----
Invalid base64 data!!!
-----END CERTIFICATE-----`

	// Create Secret with invalid PEM
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(invalidPEM),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "invalid-secret", "ca.crt", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for invalid PEM, got nil")
	}

	// Should be InvalidPEMError
	if !isErrorType(err, ErrInvalidPEM) {
		t.Errorf("Expected ErrInvalidPEM, got: %v", err)
	}
}

// TestSecretSource_BinaryDataHandling tests that binary data ([]byte) is handled correctly.
func TestSecretSource_BinaryDataHandling(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "Binary Secret CA")

	// Create Secret with binary data (as []byte, not string)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "binary-secret",
			Namespace: "default",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(testCA), // Secret.Data is map[string][]byte
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewSecretSource(fakeClient, "default", "binary-secret", "ca.crt", loader, logger)

	// Load should succeed
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA from binary Secret data: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}
}
