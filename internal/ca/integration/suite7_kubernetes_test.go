package integration

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/certwatch-app/cw-agent/internal/ca"
	"github.com/certwatch-app/cw-agent/internal/ca/integration/harness"
	"github.com/certwatch-app/cw-agent/internal/ca/integration/k8s"
	"github.com/certwatch-app/cw-agent/internal/config"
	"github.com/certwatch-app/cw-agent/internal/scanner"
)

// skipIfShort skips Kubernetes integration tests in short mode
func skipIfShort(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping Kubernetes integration test in short mode")
	}
}

// TestIntegration_K8s_ConfigMapCASource_BasicLoad validates basic ConfigMap CA source loading
func TestIntegration_K8s_ConfigMapCASource_BasicLoad(t *testing.T) {
	skipIfShort(t)

	// Setup EnvTest
	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	// Create test CA
	caHarness := harness.NewTestCAHarness(t)

	// Create ConfigMap with CA bundle
	ns := suite.CreateNamespace(t, "test-ns")
	_ = suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.RootPEM)

	// Create ConfigMapSource
	ctx := context.Background()
	logger := zap.NewNop()
	source := ca.NewConfigMapSource(suite.Client, ns.Name, "ca-bundle", "ca.crt", nil, logger)

	// Load CAs
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CAs from ConfigMap: %v", err)
	}

	// Verify
	if len(certs) != 1 {
		t.Errorf("Expected 1 CA, got %d", len(certs))
	}
	if certs[0].Subject.CommonName != "Test Root CA" {
		t.Errorf("Expected CN='Test Root CA', got '%s'", certs[0].Subject.CommonName)
	}

	// Verify ID format
	expectedID := fmt.Sprintf("configmap:%s/ca-bundle#ca.crt", ns.Name)
	if source.ID() != expectedID {
		t.Errorf("Expected ID '%s', got '%s'", expectedID, source.ID())
	}

	t.Logf("✓ ConfigMap CA source loaded successfully")
}

// TestIntegration_K8s_SecretCASource_BasicLoad validates basic Secret CA source loading
func TestIntegration_K8s_SecretCASource_BasicLoad(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)

	// Create Secret with CA bundle
	ns := suite.CreateNamespace(t, "test-ns")
	_ = suite.CreateSecretWithCA(t, ns.Name, "ca-secret", "ca.crt", caHarness.RootPEM)

	// Create SecretSource
	ctx := context.Background()
	logger := zap.NewNop()
	source := ca.NewSecretSource(suite.Client, ns.Name, "ca-secret", "ca.crt", nil, logger)

	// Load CAs
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CAs from Secret: %v", err)
	}

	// Verify
	if len(certs) != 1 {
		t.Errorf("Expected 1 CA, got %d", len(certs))
	}
	if certs[0].Subject.CommonName != "Test Root CA" {
		t.Errorf("Expected CN='Test Root CA', got '%s'", certs[0].Subject.CommonName)
	}

	// Verify ID format
	expectedID := fmt.Sprintf("secret:%s/ca-secret#ca.crt", ns.Name)
	if source.ID() != expectedID {
		t.Errorf("Expected ID '%s', got '%s'", expectedID, source.ID())
	}

	t.Logf("✓ Secret CA source loaded successfully")
}

// TestIntegration_K8s_ConfigMapHotReload_ResourceVersion validates ConfigMap hot-reload with ResourceVersion tracking
func TestIntegration_K8s_ConfigMapHotReload_ResourceVersion(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)

	// Create ConfigMap with Intermediate CA only (missing Root)
	ns := suite.CreateNamespace(t, "test-ns")
	cm := suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.IntermediatePEM)
	initialVersion := cm.ResourceVersion

	// Start server with cert signed by Root CA
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert("127.0.0.1", nil, caHarness.RootCert, caHarness.RootKey)
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Create scanner with ConfigMap source
	ctx := context.Background()
	logger := zap.NewNop()
	scn := scanner.NewWithK8sClient(5*time.Second, 1, suite.Client, logger)

	// Start K8s watcher
	changeDetected := make(chan struct{}, 1)
	sourceID := fmt.Sprintf("configmap:%s/ca-bundle#ca.crt", ns.Name)
	watcher, err := ca.NewK8sWatcher(suite.Client, ns.Name, "ca-bundle", "ConfigMap", func() {
		// Invalidate cache when ConfigMap changes
		scn.InvalidateCache(sourceID)
		changeDetected <- struct{}{}
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create K8s watcher: %v", err)
	}
	watcher.Start(ctx)
	defer watcher.Stop()

	// First scan - should FAIL (Root CA missing)
	result1 := scn.ScanWithCA(ctx, server.Hostname, server.Port, &config.CAConfig{
		TrustMode:      "custom",
		ValidationMode: "basic",
		CASources: []config.CASourceConfig{
			{
				Type:      "configmap",
				Namespace: ns.Name,
				Name:      "ca-bundle",
				Key:       "ca.crt",
			},
		},
	})
	if result1.Chain.Valid {
		t.Error("Expected validation to FAIL with incomplete CA bundle")
	}
	t.Logf("✓ Initial validation failed as expected: %s", result1.Chain.ValidationError)

	// Update ConfigMap (add Root CA)
	t.Log("Updating ConfigMap with complete CA bundle...")
	bundlePEM := append(caHarness.RootPEM, caHarness.IntermediatePEM...)
	cm = suite.UpdateConfigMapCA(t, cm, "ca.crt", bundlePEM)
	newVersion := cm.ResourceVersion

	// Verify ResourceVersion changed
	if newVersion == initialVersion {
		t.Error("ResourceVersion did not change after update")
	}
	t.Logf("ResourceVersion: %s → %s", initialVersion, newVersion)

	// Wait for watcher to detect change (polling interval + buffer)
	select {
	case <-changeDetected:
		t.Log("✓ Watcher detected ConfigMap change")
	case <-time.After(35 * time.Second): // 30s poll interval + 5s buffer
		t.Error("Watcher did not detect ConfigMap change within timeout")
	}

	// Second scan - should SUCCEED (Root CA now present)
	result2 := scn.ScanWithCA(ctx, server.Hostname, server.Port, &config.CAConfig{
		TrustMode:      "custom",
		ValidationMode: "basic",
		CASources: []config.CASourceConfig{
			{
				Type:      "configmap",
				Namespace: ns.Name,
				Name:      "ca-bundle",
				Key:       "ca.crt",
			},
		},
	})
	if !result2.Chain.Valid {
		t.Errorf("Expected validation to SUCCEED after hot-reload, got: %s", result2.Chain.ValidationError)
	}
	if result2.Chain.TrustedRoot != "Test Root CA" {
		t.Errorf("Expected trusted root 'Test Root CA', got '%s'", result2.Chain.TrustedRoot)
	}

	t.Log("✓ Hot-reload succeeded - validation now passes")
	t.Logf("  Trusted Root: %s", result2.Chain.TrustedRoot)
}

// TestIntegration_K8s_SecretHotReload_ResourceVersion validates Secret hot-reload with ResourceVersion tracking
func TestIntegration_K8s_SecretHotReload_ResourceVersion(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)

	// Create Secret with Intermediate CA only (missing Root)
	ns := suite.CreateNamespace(t, "test-ns")
	secret := suite.CreateSecretWithCA(t, ns.Name, "ca-secret", "ca.crt", caHarness.IntermediatePEM)
	initialVersion := secret.ResourceVersion

	// Start server with cert signed by Root CA
	serverCert, serverKey, _, _ := caHarness.GenerateServerCert("127.0.0.1", nil, caHarness.RootCert, caHarness.RootKey)
	server := harness.NewTestHTTPSServer(t, serverCert, serverKey)
	server.Start(t)
	defer server.Stop()

	// Create scanner with Secret source
	ctx := context.Background()
	logger := zap.NewNop()
	scn := scanner.NewWithK8sClient(5*time.Second, 1, suite.Client, logger)

	// Start K8s watcher
	changeDetected := make(chan struct{}, 1)
	sourceID := fmt.Sprintf("secret:%s/ca-secret#ca.crt", ns.Name)
	watcher, err := ca.NewK8sWatcher(suite.Client, ns.Name, "ca-secret", "Secret", func() {
		// Invalidate cache when Secret changes
		scn.InvalidateCache(sourceID)
		changeDetected <- struct{}{}
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create K8s watcher: %v", err)
	}
	watcher.Start(ctx)
	defer watcher.Stop()

	// First scan - should FAIL (Root CA missing)
	result1 := scn.ScanWithCA(ctx, server.Hostname, server.Port, &config.CAConfig{
		TrustMode:      "custom",
		ValidationMode: "basic",
		CASources: []config.CASourceConfig{
			{
				Type:      "secret",
				Namespace: ns.Name,
				Name:      "ca-secret",
				Key:       "ca.crt",
			},
		},
	})
	if result1.Chain.Valid {
		t.Error("Expected validation to FAIL with incomplete CA bundle")
	}
	t.Logf("✓ Initial validation failed as expected: %s", result1.Chain.ValidationError)

	// Update Secret (add Root CA)
	t.Log("Updating Secret with complete CA bundle...")
	bundlePEM := append(caHarness.RootPEM, caHarness.IntermediatePEM...)
	secret = suite.UpdateSecretCA(t, secret, "ca.crt", bundlePEM)
	newVersion := secret.ResourceVersion

	// Verify ResourceVersion changed
	if newVersion == initialVersion {
		t.Error("ResourceVersion did not change after update")
	}
	t.Logf("ResourceVersion: %s → %s", initialVersion, newVersion)

	// Wait for watcher to detect change
	select {
	case <-changeDetected:
		t.Log("✓ Watcher detected Secret change")
	case <-time.After(35 * time.Second):
		t.Error("Watcher did not detect Secret change within timeout")
	}

	// Second scan - should SUCCEED
	result2 := scn.ScanWithCA(ctx, server.Hostname, server.Port, &config.CAConfig{
		TrustMode:      "custom",
		ValidationMode: "basic",
		CASources: []config.CASourceConfig{
			{
				Type:      "secret",
				Namespace: ns.Name,
				Name:      "ca-secret",
				Key:       "ca.crt",
			},
		},
	})
	if !result2.Chain.Valid {
		t.Errorf("Expected validation to SUCCEED after hot-reload, got: %s", result2.Chain.ValidationError)
	}
	if result2.Chain.TrustedRoot != "Test Root CA" {
		t.Errorf("Expected trusted root 'Test Root CA', got '%s'", result2.Chain.TrustedRoot)
	}

	t.Log("✓ Hot-reload succeeded - validation now passes")
	t.Logf("  Trusted Root: %s", result2.Chain.TrustedRoot)
}

// TestIntegration_K8s_ConfigMap_MultipleCAs validates multiple CAs in single ConfigMap
func TestIntegration_K8s_ConfigMap_MultipleCAs(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)

	// Bundle with Root + Intermediate + Evil CA
	bundlePEM := append(caHarness.RootPEM, caHarness.IntermediatePEM...)
	bundlePEM = append(bundlePEM, caHarness.EvilRootPEM...)

	// Create ConfigMap
	ns := suite.CreateNamespace(t, "test-ns")
	_ = suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", bundlePEM)

	// Load CAs
	ctx := context.Background()
	logger := zap.NewNop()
	source := ca.NewConfigMapSource(suite.Client, ns.Name, "ca-bundle", "ca.crt", nil, logger)
	certs, err := source.Load(ctx)
	if err != nil {
		t.Fatalf("Failed to load CAs: %v", err)
	}

	// Verify all 3 CAs loaded
	if len(certs) != 3 {
		t.Errorf("Expected 3 CAs, got %d", len(certs))
	}

	// Verify CA names
	expectedCNs := map[string]bool{
		"Test Root CA":         false,
		"Test Intermediate CA": false,
		"Evil Root CA":         false,
	}
	for _, cert := range certs {
		cn := cert.Subject.CommonName
		if _, exists := expectedCNs[cn]; exists {
			expectedCNs[cn] = true
		}
	}

	for cn, found := range expectedCNs {
		if !found {
			t.Errorf("CA '%s' not found in loaded certificates", cn)
		}
	}

	t.Logf("✓ Multiple CAs loaded from ConfigMap")
}

// TestIntegration_K8s_ConfigMap_ConcurrentUpdates validates concurrent ConfigMap updates
func TestIntegration_K8s_ConfigMap_ConcurrentUpdates(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)
	ns := suite.CreateNamespace(t, "test-ns")
	cm := suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.RootPEM)

	// Multiple watchers on same ConfigMap
	ctx := context.Background()
	logger := zap.NewNop()
	var changeCount1 int32
	var changeCount2 int32

	watcher1, err := ca.NewK8sWatcher(suite.Client, ns.Name, "ca-bundle", "ConfigMap", func() {
		atomic.AddInt32(&changeCount1, 1)
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create K8s watcher 1: %v", err)
	}
	watcher1.Start(ctx)
	defer watcher1.Stop()

	watcher2, err := ca.NewK8sWatcher(suite.Client, ns.Name, "ca-bundle", "ConfigMap", func() {
		atomic.AddInt32(&changeCount2, 1)
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create K8s watcher 2: %v", err)
	}
	watcher2.Start(ctx)
	defer watcher2.Stop()

	// Perform 3 rapid updates
	for i := 0; i < 3; i++ {
		updatePEM := append(caHarness.RootPEM, []byte(fmt.Sprintf("# Update %d\n", i))...)
		cm = suite.UpdateConfigMapCA(t, cm, "ca.crt", updatePEM)
		time.Sleep(1 * time.Second) // Faster than polling interval
	}

	// Wait for watchers to catch up
	time.Sleep(35 * time.Second) // Poll interval + buffer

	// Verify both watchers detected changes
	// Note: With 30s polling interval, rapid updates (1s apart) will be coalesced
	// into a single change detection. This is expected behavior.
	count1 := atomic.LoadInt32(&changeCount1)
	count2 := atomic.LoadInt32(&changeCount2)

	if count1 < 1 {
		t.Errorf("Watcher 1: Expected at least 1 change, got %d", count1)
	}
	if count2 < 1 {
		t.Errorf("Watcher 2: Expected at least 1 change, got %d", count2)
	}

	t.Logf("✓ Concurrent watchers: watcher1=%d changes, watcher2=%d changes", count1, count2)
	t.Logf("  Note: Rapid updates coalesced due to 30s polling interval (expected behavior)")
}

// TestIntegration_K8s_ConfigMap_NotFound validates ConfigMap not found error
func TestIntegration_K8s_ConfigMap_NotFound(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	ns := suite.CreateNamespace(t, "test-ns")

	// Try to load from non-existent ConfigMap
	ctx := context.Background()
	logger := zap.NewNop()
	source := ca.NewConfigMapSource(suite.Client, ns.Name, "nonexistent", "ca.crt", nil, logger)
	_, err := source.Load(ctx)

	// Verify error
	if err == nil {
		t.Error("Expected error for non-existent ConfigMap")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Expected 'not found' error, got: %v", err)
	}

	t.Logf("✓ ConfigMap not found error handled correctly")
}

// TestIntegration_K8s_ConfigMap_KeyNotFound validates ConfigMap key not found error
func TestIntegration_K8s_ConfigMap_KeyNotFound(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)
	ns := suite.CreateNamespace(t, "test-ns")
	_ = suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.RootPEM)

	// Try to load from wrong key
	ctx := context.Background()
	logger := zap.NewNop()
	source := ca.NewConfigMapSource(suite.Client, ns.Name, "ca-bundle", "wrong-key", nil, logger)
	_, err := source.Load(ctx)

	// Verify error
	if err == nil {
		t.Error("Expected error for missing key")
	}

	t.Logf("✓ ConfigMap key not found error handled correctly: %v", err)
}

// TestIntegration_K8s_Secret_BinaryDataHandling validates Secret vs ConfigMap data handling
func TestIntegration_K8s_Secret_BinaryDataHandling(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)
	ns := suite.CreateNamespace(t, "test-ns")

	// Create Secret with binary data (Secret.Data is []byte)
	_ = suite.CreateSecretWithCA(t, ns.Name, "ca-secret", "ca.crt", caHarness.RootPEM)

	// Create ConfigMap with string data (ConfigMap.Data is string)
	_ = suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.RootPEM)

	// Load from both sources
	ctx := context.Background()
	logger := zap.NewNop()
	secretSource := ca.NewSecretSource(suite.Client, ns.Name, "ca-secret", "ca.crt", nil, logger)
	cmSource := ca.NewConfigMapSource(suite.Client, ns.Name, "ca-bundle", "ca.crt", nil, logger)

	secretCerts, err := secretSource.Load(ctx)
	if err != nil {
		t.Fatalf("Secret load failed: %v", err)
	}

	cmCerts, err := cmSource.Load(ctx)
	if err != nil {
		t.Fatalf("ConfigMap load failed: %v", err)
	}

	// Verify both loaded the same CA
	if len(secretCerts) != 1 || len(cmCerts) != 1 {
		t.Errorf("Expected 1 CA from each source, got Secret=%d, ConfigMap=%d", len(secretCerts), len(cmCerts))
	}
	if !secretCerts[0].Equal(cmCerts[0]) {
		t.Error("Secret and ConfigMap CAs are not equal")
	}

	t.Logf("✓ Secret (binary) and ConfigMap (string) data handled correctly")
}

// TestIntegration_K8s_Watcher_ContextCancellation validates watcher context cancellation
func TestIntegration_K8s_Watcher_ContextCancellation(t *testing.T) {
	skipIfShort(t)

	suite := k8s.NewEnvTestSuite(t)
	suite.Start(t)
	defer suite.Stop(t)

	caHarness := harness.NewTestCAHarness(t)
	ns := suite.CreateNamespace(t, "test-ns")
	cm := suite.CreateConfigMapWithCA(t, ns.Name, "ca-bundle", "ca.crt", caHarness.RootPEM)

	// Start watcher with cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	changeDetected := false
	logger := zap.NewNop()

	watcher, err := ca.NewK8sWatcher(suite.Client, ns.Name, "ca-bundle", "ConfigMap", func() {
		changeDetected = true
	}, logger)
	if err != nil {
		t.Fatalf("Failed to create K8s watcher: %v", err)
	}
	watcher.Start(ctx)

	// Cancel context immediately
	cancel()

	// Wait briefly
	time.Sleep(2 * time.Second)

	// Update ConfigMap (should not be detected)
	suite.UpdateConfigMapCA(t, cm, "ca.crt", caHarness.IntermediatePEM)

	// Wait for potential change detection
	time.Sleep(35 * time.Second)

	// Verify watcher did not detect change
	if changeDetected {
		t.Error("Watcher detected change after context cancellation")
	}

	t.Logf("✓ Watcher stopped after context cancellation")
}
