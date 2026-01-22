package ca

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// generateTestCA creates a valid self-signed CA certificate for testing
func generateTestCA(t *testing.T, cn string) string {
	t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: cn,
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return string(certPEM)
}

// Test CA certificate (self-signed root CA for testing)
const testRootCA = `-----BEGIN CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJAKL0UG+mRKUzMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
BAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTcwMjIwMTkzNTU1WhcNMjcwMjE4MTkzNTU1WjBF
MQswCQYDVQQGEwJVUzETMBEGA1UECAwKU29tZS1TdGF0ZTEhMB8GA1UECgwYSW50
ZXJuZXQgV2lkZ2l0cyBQdHkgTHRkMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIB
CgKCAQEAw8O5ERwNWWVZLTayPrgU+sQv6Br4xPKp1j4Gx9bS1kKQV0jZP1mHJ3Lx
VJPfL2vFsYxH3SQeU5L5qwJ3qVp1MuQyDhqQsQtWJ5dRWOo8B5+J3R3p8J7I3mBe
L1tKw6LxQyElKJ5V5R7NVNl0p4C4l1l5V8Y8wqQxLxP5L7YL0N5R7qWlL3qVJ8J3
R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL
0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R
7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJwIDAQABo1AwTjAdBgNVHQ4EFgQUJzQQU5YJ
B7X0P8vTZ7T0n0L5p0QwHwYDVR0jBBgwFoAUJzQQU5YJB7X0P8vTZ7T0n0L5p0Qw
DAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAQEAjKNBxp9P5qL2qL5LqWlL
3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ
8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R
7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0
N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7
qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL
3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJw==
-----END CERTIFICATE-----`

const testIntermediateCA = `-----BEGIN CERTIFICATE-----
MIIDYTCCAkmgAwIBAgIJAKL0UG+mRKU0MA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
BAYTAlVTMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQwHhcNMTcwMjIwMTkzNjAwWhcNMjcwMjE4MTkzNjAwWjBJ
MQswCQYDVQQGEwJVUzETMBEGA1UECAwKU29tZS1TdGF0ZTElMCMGA1UECgwcSW50
ZXJtZWRpYXRlIENBIFB0eSBMdGQwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEK
AoIBAQDD0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3
R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL
0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R
7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWl
L3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YLwIDAQABo1Aw
TjAdBgNVHQ4EFgQUKzQQU5YJB7X0P8vTZ7T0n0L5p0UwHwYDVR0jBBgwFoAUJzQQ
U5YJB7X0P8vTZ7T0n0L5p0QwDAYDVR0TBAUwAwEB/zANBgkqhkiG9w0BAQsFAAOC
AQEAkKNBxp9P5qL2qL5LqWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL
3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ
8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R
7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0
N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0N5R7qWlL3qVJ8J3R7YL0==
-----END CERTIFICATE-----`

const invalidPEM = `-----BEGIN CERTIFICATE-----
This is not a valid PEM certificate
-----END CERTIFICATE-----`

func setupTestDir(t *testing.T) string {
	tmpDir := t.TempDir()
	return tmpDir
}

func writeTestFile(t *testing.T, dir, filename, content string, perm os.FileMode) string {
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, []byte(content), perm); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	// Explicitly set permissions to override umask
	if err := os.Chmod(path, perm); err != nil {
		t.Fatalf("Failed to set file permissions: %v", err)
	}
	return path
}

func TestNewLoader(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	if loader == nil {
		t.Fatal("NewLoader returned nil")
	}
	if loader.logger == nil {
		t.Fatal("Loader logger is nil")
	}
}

func TestLoadSystemCAs(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)

	pool, err := loader.loadSystemCAs()
	if err != nil {
		t.Skipf("System CAs not available on this platform: %v", err)
	}

	if pool == nil {
		t.Fatal("loadSystemCAs returned nil pool without error")
	}
}

func TestLoadPEMFile(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Generate valid test certificates dynamically
	testRootCA := generateTestCA(t, "Test Root CA")
	testIntermediateCA := generateTestCA(t, "Test Intermediate CA")

	tests := []struct {
		name        string
		content     string
		perm        os.FileMode
		expectError bool
		errorType   error
	}{
		{
			name:        "valid single CA",
			content:     testRootCA,
			perm:        0644,
			expectError: false,
		},
		{
			name:        "valid multiple CAs",
			content:     testRootCA + "\n" + testIntermediateCA,
			perm:        0644,
			expectError: false,
		},
		{
			name:        "invalid PEM data",
			content:     invalidPEM,
			perm:        0644,
			expectError: true,
			errorType:   ErrEmptyBundle, // Invalid cert data results in empty bundle
		},
		{
			name:        "empty file",
			content:     "",
			perm:        0644,
			expectError: true,
			errorType:   ErrEmptyBundle,
		},
		{
			name:        "world-writable file",
			content:     testRootCA,
			perm:        0666,
			expectError: true,
			errorType:   ErrInsecurePermissions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeTestFile(t, tmpDir, tt.name+".pem", tt.content, tt.perm)
			pool := x509.NewCertPool()

			count, err := loader.loadPEMFile(pool, path)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if tt.errorType != nil && !isErrorType(err, tt.errorType) {
					t.Errorf("Expected error type %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if count == 0 {
					t.Error("Expected at least one certificate to be loaded")
				}
			}
		})
	}
}

func TestLoadPEMFileNotFound(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	pool := x509.NewCertPool()

	_, err := loader.loadPEMFile(pool, "/nonexistent/ca.pem")
	if err == nil {
		t.Fatal("Expected error for nonexistent file")
	}

	if !isErrorType(err, ErrFileNotFound) {
		t.Errorf("Expected ErrFileNotFound, got %v", err)
	}
}

func TestParseInlineCertificates(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)

	// Generate valid test certificates dynamically
	testRootCA := generateTestCA(t, "Test Root CA")
	testIntermediateCA := generateTestCA(t, "Test Intermediate CA")

	tests := []struct {
		name        string
		certPEM     string
		expectError bool
		errorType   error
	}{
		{
			name:        "valid single cert",
			certPEM:     testRootCA,
			expectError: false,
		},
		{
			name:        "valid multiple certs",
			certPEM:     testRootCA + "\n" + testIntermediateCA,
			expectError: false,
		},
		{
			name:        "invalid PEM",
			certPEM:     invalidPEM,
			expectError: true,
			errorType:   ErrInvalidPEM,
		},
		{
			name:        "empty string",
			certPEM:     "",
			expectError: true,
			errorType:   ErrEmptyBundle,
		},
		{
			name:        "no certificate blocks",
			certPEM:     "Some random text",
			expectError: true,
			errorType:   ErrInvalidPEM,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := x509.NewCertPool()
			count, err := loader.parseInlineCertificates(pool, tt.certPEM)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if tt.errorType != nil && !isErrorType(err, tt.errorType) {
					t.Errorf("Expected error type %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if count == 0 {
					t.Error("Expected at least one certificate to be loaded")
				}
			}
		})
	}
}

func TestValidateCABundle(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Generate valid test certificate dynamically
	testRootCA := generateTestCA(t, "Test Root CA")

	tests := []struct {
		name        string
		setup       func() string
		expectError bool
		errorType   error
	}{
		{
			name: "valid file",
			setup: func() string {
				return writeTestFile(t, tmpDir, "valid.pem", testRootCA, 0644)
			},
			expectError: false,
		},
		{
			name: "world-writable file",
			setup: func() string {
				return writeTestFile(t, tmpDir, "writable.pem", testRootCA, 0666)
			},
			expectError: true,
			errorType:   ErrInsecurePermissions,
		},
		{
			name: "directory instead of file",
			setup: func() string {
				dir := filepath.Join(tmpDir, "subdir")
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				return dir
			},
			expectError: true,
			errorType:   ErrInvalidFileType,
		},
		{
			name: "nonexistent file",
			setup: func() string {
				return filepath.Join(tmpDir, "nonexistent.pem")
			},
			expectError: true,
			errorType:   ErrFileNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.setup()
			err := loader.validateCABundle(path)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
				if tt.errorType != nil && !isErrorType(err, tt.errorType) {
					t.Errorf("Expected error type %v, got %v", tt.errorType, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			}
		})
	}
}

func TestLoadCertPool(t *testing.T) {
	logger := zap.NewNop() // Use nop logger to avoid test output noise
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Generate valid test certificates dynamically
	testRootCA := generateTestCA(t, "Test Root CA")
	testIntermediateCA := generateTestCA(t, "Test Intermediate CA")

	// Create test CA files
	caFile1 := writeTestFile(t, tmpDir, "ca1.pem", testRootCA, 0644)
	caFile2 := writeTestFile(t, tmpDir, "ca2.pem", testIntermediateCA, 0644)

	tests := []struct {
		name        string
		trustMode   string
		caBundles   []string
		inlineCerts []string
		expectError bool
	}{
		{
			name:        "system trust mode",
			trustMode:   "system",
			caBundles:   nil,
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "custom trust mode with file",
			trustMode:   "custom",
			caBundles:   []string{caFile1},
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "custom trust mode with multiple files",
			trustMode:   "custom",
			caBundles:   []string{caFile1, caFile2},
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "custom trust mode with inline cert",
			trustMode:   "custom",
			caBundles:   nil,
			inlineCerts: []string{testRootCA},
			expectError: false,
		},
		{
			name:        "custom trust mode with file and inline",
			trustMode:   "custom",
			caBundles:   []string{caFile1},
			inlineCerts: []string{testIntermediateCA},
			expectError: false,
		},
		{
			name:        "combined trust mode",
			trustMode:   "combined",
			caBundles:   []string{caFile1},
			inlineCerts: nil,
			expectError: false,
		},
		{
			name:        "custom trust mode without CAs",
			trustMode:   "custom",
			caBundles:   nil,
			inlineCerts: nil,
			expectError: true,
		},
		{
			name:        "invalid trust mode",
			trustMode:   "invalid",
			caBundles:   nil,
			inlineCerts: nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := loader.LoadCertPool(tt.trustMode, tt.caBundles, tt.inlineCerts)

			if tt.expectError {
				if err == nil {
					t.Fatal("Expected error but got none")
				}
			} else {
				if err != nil {
					// Skip if system CAs are unavailable (some test environments)
					if tt.trustMode == "system" && isErrorType(err, ErrSystemCAUnavailable) {
						t.Skipf("System CAs not available: %v", err)
					}
					t.Fatalf("Unexpected error: %v", err)
				}
				if pool == nil {
					t.Fatal("Expected non-nil cert pool")
				}
			}
		})
	}
}

// TestValidateCABundle_Symlink tests that symlinks are rejected
// Based on patterns from Go's os/stat_test.go
func TestValidateCABundle_Symlink(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Generate valid test certificate
	testRootCA := generateTestCA(t, "Test Root CA")

	// Create a real CA file
	caFile := writeTestFile(t, tmpDir, "ca.pem", testRootCA, 0644)

	// Create a symlink to the CA file
	symlinkPath := filepath.Join(tmpDir, "ca-symlink.pem")
	err := os.Symlink(caFile, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink (may not be supported): %v", err)
	}

	// Validate should reject the symlink
	err = loader.validateCABundle(symlinkPath)
	if err == nil {
		t.Error("Expected error for symlink, got none")
	}

	if !isErrorType(err, ErrInvalidFileType) {
		t.Errorf("Expected ErrInvalidFileType, got %v", err)
	}

	// Verify error message mentions symlink
	if err != nil && !contains(err.Error(), "symlink") {
		t.Errorf("Error message should mention 'symlink', got: %s", err.Error())
	}
}

// TestValidateCABundle_BrokenSymlink tests broken symlink detection
// Based on Go issue #53437 patterns
func TestValidateCABundle_BrokenSymlink(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Create a symlink pointing to a non-existent file
	nonExistentPath := filepath.Join(tmpDir, "does-not-exist.pem")
	symlinkPath := filepath.Join(tmpDir, "broken-symlink.pem")
	err := os.Symlink(nonExistentPath, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	// Validate should detect it as a symlink (not a "file not found")
	err = loader.validateCABundle(symlinkPath)
	if err == nil {
		t.Error("Expected error for broken symlink")
	}

	// Should be InvalidFileType, not FileNotFound
	// os.Lstat succeeds on broken symlinks
	if !isErrorType(err, ErrInvalidFileType) {
		t.Errorf("Expected ErrInvalidFileType for broken symlink, got %T: %v", err, err)
	}
}

// TestValidateCABundle_SymlinkToDirectory tests symlink to directory
func TestValidateCABundle_SymlinkToDirectory(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Create a directory
	dirPath := filepath.Join(tmpDir, "ca-dir")
	if err := os.Mkdir(dirPath, 0755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	// Create a symlink to the directory
	symlinkPath := filepath.Join(tmpDir, "dir-symlink")
	err := os.Symlink(dirPath, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	// Should be rejected as symlink (detected before directory check)
	err = loader.validateCABundle(symlinkPath)
	if err == nil {
		t.Error("Expected error for symlink to directory")
	}

	if !isErrorType(err, ErrInvalidFileType) {
		t.Errorf("Expected ErrInvalidFileType, got %v", err)
	}
}

// TestLoadSystemCAs_ErrorHandling tests SystemCertPool error handling
// Based on Go's verify_test.go TestSystemRootsError pattern
func TestLoadSystemCAs_ErrorHandling(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)

	// Try to load system CAs
	pool, err := loader.loadSystemCAs()

	// On most platforms, this should succeed
	// On some platforms (like certain Windows configurations), it may fail
	if err != nil {
		// Verify error is wrapped correctly
		if !isErrorType(err, ErrSystemCAUnavailable) {
			t.Errorf("Expected ErrSystemCAUnavailable, got %v", err)
		}
		t.Logf("System CAs unavailable (expected on some platforms): %v", err)
		return
	}

	// If successful, pool should not be nil
	if pool == nil {
		t.Error("SystemCertPool succeeded but returned nil pool")
	}
}

// TestLoadCertPool_SystemCAFallback tests behavior when system CAs fail
func TestLoadCertPool_SystemCAFallback(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)

	// Test with system trust mode
	trustMode := "system"

	pool, err := loader.LoadCertPool(trustMode, nil, nil)

	// Should either succeed or fail gracefully
	if err != nil {
		// If system CAs are unavailable, error should be clear
		if !isErrorType(err, ErrSystemCAUnavailable) {
			t.Errorf("Expected ErrSystemCAUnavailable on failure, got %v", err)
		}
		t.Logf("System CAs unavailable: %v", err)
	} else if pool == nil {
		t.Error("LoadCertPool succeeded but returned nil pool")
	}
}

// TestLoadPEMFile_SymlinkRejection tests that loadPEMFile rejects symlinks
func TestLoadPEMFile_SymlinkRejection(t *testing.T) {
	logger := zaptest.NewLogger(t)
	loader := NewLoader(logger)
	tmpDir := setupTestDir(t)

	// Generate valid test certificate
	testRootCA := generateTestCA(t, "Test Root CA")

	// Create a real CA file
	caFile := writeTestFile(t, tmpDir, "real-ca.pem", testRootCA, 0644)

	// Create a symlink
	symlinkPath := filepath.Join(tmpDir, "symlink-ca.pem")
	err := os.Symlink(caFile, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlink: %v", err)
	}

	pool := x509.NewCertPool()
	_, err = loader.loadPEMFile(pool, symlinkPath)

	if err == nil {
		t.Error("Expected loadPEMFile to reject symlink")
	}

	if !isErrorType(err, ErrInvalidFileType) {
		t.Errorf("Expected ErrInvalidFileType, got %v", err)
	}
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// isErrorType checks if an error wraps a specific error type
func isErrorType(err, target error) bool {
	if err == nil || target == nil {
		return false
	}

	// Direct comparison
	if err == target {
		return true
	}

	// Check if error wraps the target using errors.As pattern
	type unwrapper interface {
		Unwrap() error
	}

	for err != nil {
		if err == target {
			return true
		}
		if u, ok := err.(unwrapper); ok {
			err = u.Unwrap()
		} else {
			break
		}
	}

	return false
}
