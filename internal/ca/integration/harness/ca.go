// Package harness provides test infrastructure for CA validation integration tests.
package harness

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestCAHarness generates a complete PKI hierarchy for integration testing.
// It provides root CA, intermediate CA, and evil CA certificates for comprehensive testing.
type TestCAHarness struct {
	// Root CA
	RootCert *x509.Certificate
	RootKey  *rsa.PrivateKey
	RootPEM  []byte

	// Intermediate CA (signed by Root)
	IntermediateCert *x509.Certificate
	IntermediateKey  *rsa.PrivateKey
	IntermediatePEM  []byte

	// Alternative Root (for MITM simulation)
	EvilRootCert *x509.Certificate
	EvilRootKey  *rsa.PrivateKey
	EvilRootPEM  []byte

	TempDir string // t.TempDir() location
	t       *testing.T
}

// NewTestCAHarness creates a new test CA harness with a complete PKI hierarchy.
// It generates:
// - Root CA (self-signed)
// - Intermediate CA (signed by Root)
// - Evil Root CA (for MITM testing)
func NewTestCAHarness(t *testing.T) *TestCAHarness {
	t.Helper()

	h := &TestCAHarness{
		TempDir: t.TempDir(),
		t:       t,
	}

	// Generate Root CA
	h.RootCert, h.RootKey, h.RootPEM = h.GenerateCA("Test Root CA", nil, nil)

	// Generate Intermediate CA (signed by Root)
	h.IntermediateCert, h.IntermediateKey, h.IntermediatePEM = h.GenerateCA("Test Intermediate CA", h.RootCert, h.RootKey)

	// Generate Evil Root CA (for MITM testing)
	h.EvilRootCert, h.EvilRootKey, h.EvilRootPEM = h.GenerateCA("Evil Root CA", nil, nil)

	return h
}

// GenerateCA generates a CA certificate (exported for advanced test scenarios).
// If signerCert and signerKey are nil, creates a self-signed CA (root).
// Otherwise, creates an intermediate CA signed by the provided signer.
func (h *TestCAHarness) GenerateCA(cn string, signerCert *x509.Certificate, signerKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	h.t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.t.Fatalf("Failed to generate CA key: %v", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		h.t.Fatalf("Failed to generate serial number: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"CertWatch Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
	}

	// Determine signer (self-sign if no signer provided)
	var parent *x509.Certificate
	var parentKey *rsa.PrivateKey
	if signerCert == nil {
		parent = template // self-signed
		parentKey = key
	} else {
		parent = signerCert
		parentKey = signerKey
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, parentKey)
	if err != nil {
		h.t.Fatalf("Failed to create CA certificate: %v", err)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		h.t.Fatalf("Failed to parse CA certificate: %v", err)
	}

	// Encode to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	return cert, key, certPEM
}

// GenerateServerCert generates a server certificate signed by the specified CA.
// hostname: Common Name (CN) for the certificate
// sans: Subject Alternative Names (optional)
// signerCert: CA certificate to sign with
// signerKey: CA private key to sign with
//
// Returns: certificate, private key, certificate PEM, key PEM
func (h *TestCAHarness) GenerateServerCert(hostname string, sans []string, signerCert *x509.Certificate, signerKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, []byte, []byte) {
	h.t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.t.Fatalf("Failed to generate server key: %v", err)
	}

	// Create certificate template
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		h.t.Fatalf("Failed to generate serial number: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{"CertWatch Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(90 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Process SANs: separate IP addresses from DNS names
	var dnsNames []string
	var ipAddresses []net.IP

	// Add SANs
	for _, san := range sans {
		if ip := net.ParseIP(san); ip != nil {
			ipAddresses = append(ipAddresses, ip)
		} else {
			dnsNames = append(dnsNames, san)
		}
	}

	// Add hostname to SANs if not already present
	if hostname != "" {
		if ip := net.ParseIP(hostname); ip != nil {
			// Hostname is an IP address
			if !containsIP(ipAddresses, ip) {
				ipAddresses = append(ipAddresses, ip)
			}
		} else {
			// Hostname is a DNS name
			if !containsString(dnsNames, hostname) {
				dnsNames = append(dnsNames, hostname)
			}
		}
	}

	template.DNSNames = dnsNames
	template.IPAddresses = ipAddresses

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, signerCert, &key.PublicKey, signerKey)
	if err != nil {
		h.t.Fatalf("Failed to create server certificate: %v", err)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		h.t.Fatalf("Failed to parse server certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return cert, key, certPEM, keyPEM
}

// WriteCABundle writes multiple CA certificates to a PEM file.
// The bundle can contain root CAs, intermediates, or both.
// Returns the full path to the written file.
func (h *TestCAHarness) WriteCABundle(certs []*x509.Certificate, filename string) string {
	h.t.Helper()

	path := filepath.Join(h.TempDir, filename)

	//nolint:prealloc // Test helper with small number of certs; preallocation not critical
	var bundle []byte
	for _, cert := range certs {
		certPEM := pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: cert.Raw,
		})
		bundle = append(bundle, certPEM...)
	}

	if err := os.WriteFile(path, bundle, 0644); err != nil {
		h.t.Fatalf("Failed to write CA bundle: %v", err)
	}

	return path
}

// WriteServerCertChain writes a server certificate and private key to files.
// Returns paths to the certificate file and key file.
func (h *TestCAHarness) WriteServerCertChain(cert *x509.Certificate, key *rsa.PrivateKey, basename string) (certPath, keyPath string) {
	h.t.Helper()

	// Write certificate
	certPath = filepath.Join(h.TempDir, basename+".crt")
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		h.t.Fatalf("Failed to write server certificate: %v", err)
	}

	// Write key
	keyPath = filepath.Join(h.TempDir, basename+".key")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		h.t.Fatalf("Failed to write server key: %v", err)
	}

	return certPath, keyPath
}

// GenerateExpiredCert generates an expired server certificate for testing.
// The certificate will have NotAfter set to 1 hour ago.
func (h *TestCAHarness) GenerateExpiredCert(hostname string, signerCert *x509.Certificate, signerKey *rsa.PrivateKey) (*x509.Certificate, *rsa.PrivateKey, []byte, []byte) {
	h.t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		h.t.Fatalf("Failed to generate server key: %v", err)
	}

	// Create certificate template with past dates
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		h.t.Fatalf("Failed to generate serial number: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{"CertWatch Test"},
		},
		DNSNames:              []string{hostname},
		NotBefore:             time.Now().Add(-48 * time.Hour), // 2 days ago
		NotAfter:              time.Now().Add(-1 * time.Hour),  // 1 hour ago (EXPIRED)
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}

	// Create certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, signerCert, &key.PublicKey, signerKey)
	if err != nil {
		h.t.Fatalf("Failed to create expired certificate: %v", err)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		h.t.Fatalf("Failed to parse expired certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	return cert, key, certPEM, keyPEM
}

// containsString checks if a slice contains a string.
func containsString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// containsIP checks if a slice contains an IP address.
func containsIP(slice []net.IP, ip net.IP) bool {
	for _, addr := range slice {
		if addr.Equal(ip) {
			return true
		}
	}
	return false
}
