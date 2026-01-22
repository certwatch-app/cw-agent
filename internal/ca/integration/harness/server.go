// Package harness provides test infrastructure for CA validation integration tests.
package harness

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// TestHTTPSServer wraps an HTTPS server for integration testing.
// It provides a simple HTTPS server with configurable certificates for testing
// CA validation scenarios.
type TestHTTPSServer struct {
	URL      string // e.g., https://127.0.0.1:54321
	Hostname string
	Port     int
	Server   *http.Server
	Listener net.Listener

	cert     *x509.Certificate
	key      *rsa.PrivateKey
	certMu   sync.RWMutex
	t        *testing.T
	stopOnce sync.Once
}

// NewTestHTTPSServer creates a new test HTTPS server with the provided certificate.
// It listens on a random available port (127.0.0.1:0).
func NewTestHTTPSServer(t *testing.T, cert *x509.Certificate, key *rsa.PrivateKey) *TestHTTPSServer {
	t.Helper()

	// Create listener on random available port
	lc := net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Extract port from listener address
	addr := listener.Addr().(*net.TCPAddr)
	port := addr.Port
	hostname := "127.0.0.1"

	// Encode certificate and key to PEM for TLS config
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Create TLS certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("Failed to create TLS certificate: %v", err)
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		MinVersion:   tls.VersionTLS12,
	}

	// Create HTTP handler (simple 200 OK response)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Test HTTPS Server\n")) //nolint:errcheck // Test HTTP handler; response headers sent, error indicates broken client connection
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK\n")) //nolint:errcheck // Test HTTP handler; response headers sent, error indicates broken client connection
	})

	// Create HTTP server
	server := &http.Server{
		Handler:      mux,
		TLSConfig:    tlsConfig,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}

	return &TestHTTPSServer{
		URL:      fmt.Sprintf("https://%s:%d", hostname, port),
		Hostname: hostname,
		Port:     port,
		Server:   server,
		Listener: listener,
		cert:     cert,
		key:      key,
		t:        t,
	}
}

// Start starts the HTTPS server in a goroutine.
// It returns immediately. Use WaitReady() to wait for the server to be ready.
func (s *TestHTTPSServer) Start(t *testing.T) {
	t.Helper()

	go func() {
		if err := s.Server.ServeTLS(s.Listener, "", ""); err != nil && err != http.ErrServerClosed {
			s.t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	s.WaitReady(t, 5*time.Second)
}

// WaitReady waits for the server to be ready to accept connections.
// It polls the /health endpoint until it responds or the timeout expires.
func (s *TestHTTPSServer) WaitReady(t *testing.T, timeout time.Duration) {
	t.Helper()

	// Create HTTP client that accepts any certificate (we're testing TLS, not HTTP)
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, //nolint:gosec // Test client only
			},
		},
		Timeout: 1 * time.Second,
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("%s/health", s.URL), nil)
		if err != nil {
			cancel()
			time.Sleep(50 * time.Millisecond)
			continue
		}

		resp, err := client.Do(req)
		cancel()
		if err == nil {
			resp.Body.Close() //nolint:errcheck // Test server health check; error indicates broken connection, non-actionable
			if resp.StatusCode == http.StatusOK {
				return // Server is ready
			}
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Server did not become ready within %v", timeout)
}

// Stop gracefully stops the HTTPS server.
func (s *TestHTTPSServer) Stop() {
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := s.Server.Shutdown(ctx); err != nil {
			s.t.Logf("Server shutdown error: %v", err)
		}

		s.Listener.Close() //nolint:errcheck // Test server cleanup; error non-actionable during shutdown
	})
}

// UpdateCert updates the server's certificate and key dynamically.
// This is useful for testing certificate rotation scenarios.
// Note: The server must be restarted for changes to take effect.
func (s *TestHTTPSServer) UpdateCert(cert *x509.Certificate, key *rsa.PrivateKey) error {
	s.certMu.Lock()
	defer s.certMu.Unlock()

	// Encode certificate and key to PEM
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})

	// Create new TLS certificate
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return fmt.Errorf("failed to create TLS certificate: %w", err)
	}

	// Update TLS config
	s.Server.TLSConfig.Certificates = []tls.Certificate{tlsCert}
	s.cert = cert
	s.key = key

	return nil
}

// GetCert returns the current certificate (thread-safe).
func (s *TestHTTPSServer) GetCert() *x509.Certificate {
	s.certMu.RLock()
	defer s.certMu.RUnlock()
	return s.cert
}

// Address returns the server address (hostname:port).
func (s *TestHTTPSServer) Address() string {
	return fmt.Sprintf("%s:%d", s.Hostname, s.Port)
}
