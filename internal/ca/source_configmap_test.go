package ca

import (
	"context"
	"testing"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestConfigMapSource_LoadSingleCA tests loading a single CA from ConfigMap.
func TestConfigMapSource_LoadSingleCA(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "ConfigMap Test CA")

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.crt": testCA,
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "test-cm", "ca.crt", loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA from ConfigMap: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	// Verify source metadata
	expectedID := "configmap:default/test-cm#ca.crt"
	if source.ID() != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, source.ID())
	}

	if source.Type() != "configmap" {
		t.Errorf("Expected type 'configmap', got %s", source.Type())
	}

	if source.GetNamespace() != "default" {
		t.Errorf("Expected namespace 'default', got %s", source.GetNamespace())
	}

	if source.GetName() != "test-cm" {
		t.Errorf("Expected name 'test-cm', got %s", source.GetName())
	}
}

// TestConfigMapSource_LoadMultipleCAs tests loading multiple CAs from ConfigMap.
func TestConfigMapSource_LoadMultipleCAs(t *testing.T) {
	// Generate multiple test CAs
	ca1 := generateTestCAHelper(t, "First CA")
	ca2 := generateTestCAHelper(t, "Second CA")
	ca3 := generateTestCAHelper(t, "Third CA")
	bundleContent := ca1 + "\n" + ca2 + "\n" + ca3

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca-bundle.crt": bundleContent,
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "multi-cm", "ca-bundle.crt", loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA bundle from ConfigMap: %v", err)
	}

	if len(certs) != 3 {
		t.Errorf("Expected 3 certificates, got %d", len(certs))
	}
}

// TestConfigMapSource_ConfigMapNotFound tests error handling when ConfigMap doesn't exist.
func TestConfigMapSource_ConfigMapNotFound(t *testing.T) {
	// Create empty client
	fakeClient := newFakeK8sClient(t)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "missing-cm", "ca.crt", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for non-existent ConfigMap, got nil")
	}

	// Error should mention the ConfigMap
	if err != nil && err.Error()[:len("failed to fetch ConfigMap")] != "failed to fetch ConfigMap" {
		t.Errorf("Expected error about fetching ConfigMap, got: %v", err)
	}
}

// TestConfigMapSource_KeyNotFound tests error handling when key doesn't exist in ConfigMap.
func TestConfigMapSource_KeyNotFound(t *testing.T) {
	// Create ConfigMap without the expected key
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"other-key": "some data",
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	// Create source looking for missing key
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "test-cm", "ca.crt", loader, logger)

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

// TestConfigMapSource_EmptyKeyValue tests error handling for empty key value.
func TestConfigMapSource_EmptyKeyValue(t *testing.T) {
	// Create ConfigMap with empty key
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.crt": "",
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "empty-cm", "ca.crt", loader, logger)

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

// TestConfigMapSource_InvalidPEM tests error handling for invalid PEM data.
func TestConfigMapSource_InvalidPEM(t *testing.T) {
	invalidPEM := `-----BEGIN CERTIFICATE-----
Invalid base64 data!!!
-----END CERTIFICATE-----`

	// Create ConfigMap with invalid PEM
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.crt": invalidPEM,
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "default", "invalid-cm", "ca.crt", loader, logger)

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

// TestConfigMapSource_IDFormat tests the ID format for ConfigMap sources.
func TestConfigMapSource_IDFormat(t *testing.T) {
	testCA := generateTestCAHelper(t, "ID Format Test CA")

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-configmap",
			Namespace: "my-namespace",
		},
		Data: map[string]string{
			"my-key.crt": testCA,
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewConfigMapSource(fakeClient, "my-namespace", "my-configmap", "my-key.crt", loader, logger)

	// Verify ID format: configmap:namespace/name#key
	expectedID := "configmap:my-namespace/my-configmap#my-key.crt"
	if source.ID() != expectedID {
		t.Errorf("Expected ID format %s, got %s", expectedID, source.ID())
	}

	// Verify Load works
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}
}
