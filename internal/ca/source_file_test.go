package ca

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

// TestFileSource_LoadSingleCA tests loading a single CA certificate from file.
func TestFileSource_LoadSingleCA(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate test CA
	testCA := generateTestCAHelper(t, "Test File CA")

	// Write CA file
	caFile := writeTestFileHelper(t, tmpDir, "single-ca.pem", testCA, 0644)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(caFile, loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA from file: %v", err)
	}

	if len(certs) != 1 {
		t.Errorf("Expected 1 certificate, got %d", len(certs))
	}

	// Verify source metadata
	expectedID := "file:" + caFile
	if source.ID() != expectedID {
		t.Errorf("Expected ID %s, got %s", expectedID, source.ID())
	}

	if source.Type() != "file" {
		t.Errorf("Expected type 'file', got %s", source.Type())
	}

	if source.GetPath() != caFile {
		t.Errorf("Expected path %s, got %s", caFile, source.GetPath())
	}
}

// TestFileSource_LoadMultipleCAs tests loading multiple CAs from a single file.
func TestFileSource_LoadMultipleCAs(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate multiple test CAs
	ca1 := generateTestCAHelper(t, "First CA")
	ca2 := generateTestCAHelper(t, "Second CA")
	ca3 := generateTestCAHelper(t, "Third CA")

	// Concatenate CAs
	bundleContent := ca1 + "\n" + ca2 + "\n" + ca3

	// Write bundle file
	bundleFile := writeTestFileHelper(t, tmpDir, "bundle.pem", bundleContent, 0644)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(bundleFile, loader, logger)

	// Load certificates
	ctx := context.Background()
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CA bundle: %v", err)
	}

	if len(certs) != 3 {
		t.Errorf("Expected 3 certificates, got %d", len(certs))
	}
}

// TestFileSource_FileNotFound tests error handling when file doesn't exist.
func TestFileSource_FileNotFound(t *testing.T) {
	nonExistentPath := "/tmp/nonexistent-ca-bundle.pem"

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(nonExistentPath, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for non-existent file, got nil")
	}

	// Should be FileNotFoundError
	if !isErrorType(err, ErrFileNotFound) {
		t.Errorf("Expected ErrFileNotFound, got: %v", err)
	}
}

// TestFileSource_EmptyFile tests error handling for empty files.
func TestFileSource_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Write empty file
	emptyFile := writeTestFileHelper(t, tmpDir, "empty.pem", "", 0644)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(emptyFile, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for empty file, got nil")
	}

	// Should be EmptyBundleError
	if !isErrorType(err, ErrEmptyBundle) {
		t.Errorf("Expected ErrEmptyBundle, got: %v", err)
	}
}

// TestFileSource_InvalidPEM tests error handling for invalid PEM data.
func TestFileSource_InvalidPEM(t *testing.T) {
	tmpDir := t.TempDir()

	// Invalid PEM content
	invalidPEM := `-----BEGIN CERTIFICATE-----
This is not valid base64 encoded data!
-----END CERTIFICATE-----`

	// Write invalid file
	invalidFile := writeTestFileHelper(t, tmpDir, "invalid.pem", invalidPEM, 0644)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(invalidFile, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for invalid PEM data, got nil")
	}

	// Should be InvalidPEMError or EmptyBundleError (if parse fails completely)
	if !isErrorType(err, ErrInvalidPEM) && !isErrorType(err, ErrEmptyBundle) {
		t.Errorf("Expected ErrInvalidPEM or ErrEmptyBundle, got: %v", err)
	}
}

// TestFileSource_WorldWritable tests rejection of world-writable files.
func TestFileSource_WorldWritable(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate test CA
	testCA := generateTestCAHelper(t, "Insecure CA")

	// Write file with world-writable permissions
	insecureFile := writeTestFileHelper(t, tmpDir, "insecure.pem", testCA, 0666)

	// Create source
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(insecureFile, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for world-writable file, got nil")
	}

	// Should be InsecurePermissionsError
	if !isErrorType(err, ErrInsecurePermissions) {
		t.Errorf("Expected ErrInsecurePermissions, got: %v", err)
	}
}

// TestFileSource_SymlinkRejection tests that symlinks are rejected.
func TestFileSource_SymlinkRejection(t *testing.T) {
	tmpDir := t.TempDir()

	// Generate test CA
	testCA := generateTestCAHelper(t, "Symlink CA")

	// Write real file
	realFile := writeTestFileHelper(t, tmpDir, "real.pem", testCA, 0644)

	// Create symlink
	symlinkPath := filepath.Join(tmpDir, "symlink.pem")
	if err := os.Symlink(realFile, symlinkPath); err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	// Create source pointing to symlink
	logger := zap.NewNop()
	loader := NewLoader(logger)
	source := NewFileSource(symlinkPath, loader, logger)

	// Load should fail
	ctx := context.Background()
	_, err := source.Load(ctx)
	if err == nil {
		t.Error("Expected error for symlink, got nil")
	}

	// Should be InvalidFileTypeError
	if !isErrorType(err, ErrInvalidFileType) {
		t.Errorf("Expected ErrInvalidFileType, got: %v", err)
	}
}
