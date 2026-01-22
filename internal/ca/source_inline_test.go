package ca

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"go.uber.org/zap"
)

// TestInlineSource_LoadSingleCA tests loading a single CA from inline PEM.
func TestInlineSource_LoadSingleCA(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "Inline Test CA")

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewInlineSource(testCA, loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA from inline PEM: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	// Verify source metadata
	if source.Type() != "inline" {
		t.Errorf("Expected type 'inline', got %s", source.Type())
	}

	// Verify ID is hash-based
	if source.ID()[:7] != "inline:" {
		t.Errorf("Expected ID to start with 'inline:', got %s", source.ID())
	}
}

// TestInlineSource_LoadMultipleCAs tests loading multiple CAs from inline PEM.
func TestInlineSource_LoadMultipleCAs(t *testing.T) {
	// Generate multiple test CAs
	ca1 := generateTestCAHelper(t, "First Inline CA")
	ca2 := generateTestCAHelper(t, "Second Inline CA")
	ca3 := generateTestCAHelper(t, "Third Inline CA")
	bundleContent := ca1 + "\n" + ca2 + "\n" + ca3

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewInlineSource(bundleContent, loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA bundle from inline PEM: %v", err)
	}

	if len(certs) != 3 {
		t.Errorf("Expected 3 certificates, got %d", len(certs))
	}
}

// TestInlineSource_EmptyString tests error handling for empty PEM data.
func TestInlineSource_EmptyString(t *testing.T) {
	// Create source with empty string
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewInlineSource("", loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for empty inline PEM, got nil")
	}

	// Should be EmptyBundleError
	if !isErrorType(err, ErrEmptyBundle) {
		t.Errorf("Expected ErrEmptyBundle, got: %v", err)
	}
}

// TestInlineSource_InvalidPEM tests error handling for invalid PEM data.
func TestInlineSource_InvalidPEM(t *testing.T) {
	invalidPEM := `-----BEGIN CERTIFICATE-----
Invalid base64 encoded data!!!
-----END CERTIFICATE-----`

	// Create source with invalid PEM
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewInlineSource(invalidPEM, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for invalid inline PEM, got nil")
	}

	// Should be InvalidPEMError
	if !isErrorType(err, ErrInvalidPEM) {
		t.Errorf("Expected ErrInvalidPEM, got: %v", err)
	}
}

// TestInlineSource_IDUniqueness tests that same PEM produces same ID (hash consistency).
func TestInlineSource_IDUniqueness(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "ID Test CA")

	logger := zap.NewNop()
	loader := NewLoader(logger)

	// Create two sources with same PEM
	source1 := NewInlineSource(testCA, loader, logger)
	source2 := NewInlineSource(testCA, loader, logger)

	// Should have same ID (deterministic hash)
	if source1.ID() != source2.ID() {
		t.Errorf("Expected same ID for same PEM data\nGot:\n  %s\n  %s", source1.ID(), source2.ID())
	}

	// Generate different CA
	differentCA := generateTestCAHelper(t, "Different CA")
	source3 := NewInlineSource(differentCA, loader, logger)

	// Should have different ID
	if source1.ID() == source3.ID() {
		t.Error("Expected different IDs for different PEM data")
	}

	// Verify ID format matches expected pattern
	hash := sha256.Sum256([]byte(testCA))
	expectedID := fmt.Sprintf("inline:%x", hash[:8])

	if source1.ID() != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, source1.ID())
	}
}

// TestInlineSource_WhitespaceHandling tests that trailing whitespace is handled correctly.
func TestInlineSource_WhitespaceHandling(t *testing.T) {
	// Generate test CA
	testCA := generateTestCAHelper(t, "Whitespace Test CA")

	// Add trailing whitespace (PEM parser handles trailing whitespace)
	testCAWithWhitespace := testCA + "\n\n  "

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewInlineSource(testCAWithWhitespace, loader, logger)

	// Load should succeed (PEM parser handles trailing whitespace)
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA with trailing whitespace: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	// Note: ID will be different due to whitespace in hash
	// This is expected behavior - ID is based on exact content
}
