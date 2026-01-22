// Package ca provides testing utilities for CA validation components.
// This file contains shared helpers used across multiple test files.
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
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"software.sslmate.com/src/go-pkcs12"
)

// generateTestCA creates a valid self-signed CA certificate for testing.
// It returns the PEM-encoded certificate as a string.
func generateTestCAHelper(t *testing.T, cn string) string {
	t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
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

// generateTestCAWithKey generates a CA certificate and returns both PEM-encoded cert and key.
// Used for PKCS12 testing where we need both cert and key.
func generateTestCAWithKey(t *testing.T, cn string) (certPEM string, keyPEM string, cert *x509.Certificate, key *rsa.PrivateKey) {
	t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().Unix()),
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

	// Parse certificate
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode key to PEM
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return string(certPEMBytes), string(keyPEMBytes), cert, key
}

// writeTestFileHelper writes content to a file with specified permissions.
// It explicitly sets permissions to override umask.
func writeTestFileHelper(t *testing.T, dir, filename, content string, perm os.FileMode) string {
	t.Helper()
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

// generateTestPKCS12 creates a PKCS12 archive for testing.
// It returns the PKCS12 data as bytes.
// If password is empty, the PKCS12 will not be encrypted.
func generateTestPKCS12(t *testing.T, password string) []byte {
	t.Helper()

	// Generate test CA with key
	_, _, cert, key := generateTestCAWithKey(t, "Test PKCS12 CA")

	// Create PKCS12 with password
	var p12Data []byte
	var err error

	if password == "" {
		// Modern encoder with no password
		p12Data, err = pkcs12.Modern.Encode(key, cert, nil, "")
	} else {
		// Modern encoder with password
		p12Data, err = pkcs12.Modern.Encode(key, cert, nil, password)
	}

	if err != nil {
		t.Fatalf("Failed to encode PKCS12: %v", err)
	}

	return p12Data
}

// generateTestPKCS12WithChain creates a PKCS12 archive containing cert + CA chain.
func generateTestPKCS12WithChain(t *testing.T, password string, caCerts []*x509.Certificate) []byte {
	t.Helper()

	// Generate test CA with key
	_, _, cert, key := generateTestCAWithKey(t, "Test PKCS12 CA")

	// Create PKCS12 with password and CA chain
	var p12Data []byte
	var err error

	if password == "" {
		p12Data, err = pkcs12.Modern.Encode(key, cert, caCerts, "")
	} else {
		p12Data, err = pkcs12.Modern.Encode(key, cert, caCerts, password)
	}

	if err != nil {
		t.Fatalf("Failed to encode PKCS12: %v", err)
	}

	return p12Data
}

// newFakeK8sClient creates a fake Kubernetes client for testing.
// It initializes the client with the provided runtime objects (ConfigMaps, Secrets, etc.)
func newFakeK8sClient(t *testing.T, objects ...client.Object) client.Client {
	t.Helper()

	// Create scheme and add core types
	scheme := metav1.NewScheme()
	_ = corev1.AddToScheme(scheme)

	// Build fake client
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()
}

// parseCertFromPEM parses a PEM-encoded certificate and returns the x509.Certificate.
// Used in tests to verify certificate content.
func parseCertFromPEM(t *testing.T, pemData string) *x509.Certificate {
	t.Helper()

	block, _ := pem.Decode([]byte(pemData))
	if block == nil {
		t.Fatal("Failed to decode PEM block")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}
