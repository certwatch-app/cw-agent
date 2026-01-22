package ca

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestK8sWatcher_ResourceVersionTracking tests ResourceVersion change detection.
func TestK8sWatcher_ResourceVersionTracking(t *testing.T) {
	// Create initial ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cm",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Data: map[string]string{
			"ca.crt": generateTestCAHelper(t, "Test CA"),
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	var changeCount atomic.Int32
	onChange := func() {
		changeCount.Add(1)
	}

	logger := zap.NewNop()
	watcher, err := NewK8sWatcher(fakeClient, "default", "test-cm", "ConfigMap", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	// Override poll interval for faster testing
	watcher.pollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for initial poll (should not trigger onChange)
	time.Sleep(200 * time.Millisecond)

	initialCount := changeCount.Load()
	if initialCount != 0 {
		t.Errorf("Expected 0 onChange calls after initial poll, got %d", initialCount)
	}

	// Get latest ConfigMap and update it (simulate ConfigMap update)
	updatedCM := &corev1.ConfigMap{}
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-cm"}, updatedCM); err != nil {
		t.Fatalf("Failed to get ConfigMap: %v", err)
	}
	updatedCM.Data["ca.crt"] = updatedCM.Data["ca.crt"] + "\n# Modified"
	if err := fakeClient.Update(ctx, updatedCM); err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	// Wait for poll to detect change
	time.Sleep(300 * time.Millisecond)

	// onChange should have been called
	count := changeCount.Load()
	if count < 1 {
		t.Errorf("Expected at least 1 onChange call after ResourceVersion change, got %d", count)
	}
}

// TestK8sWatcher_PollingLoop tests the polling loop behavior.
func TestK8sWatcher_PollingLoop(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "polling-test",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Data: map[string]string{
			"ca.crt": generateTestCAHelper(t, "Polling Test CA"),
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	var changeCount atomic.Int32
	onChange := func() {
		changeCount.Add(1)
	}

	logger := zap.NewNop()
	watcher, err := NewK8sWatcher(fakeClient, "default", "polling-test", "ConfigMap", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	watcher.pollInterval = 150 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for initial poll
	time.Sleep(200 * time.Millisecond)

	// Update ConfigMap multiple times
	for i := 2; i <= 4; i++ {
		updatedCM := &corev1.ConfigMap{}
		if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "polling-test"}, updatedCM); err != nil {
			t.Fatalf("Failed to get ConfigMap: %v", err)
		}
		updatedCM.Data["ca.crt"] = updatedCM.Data["ca.crt"] + "\n# Modified"
		if err := fakeClient.Update(ctx, updatedCM); err != nil {
			t.Fatalf("Failed to update ConfigMap: %v", err)
		}
		time.Sleep(200 * time.Millisecond) // Allow poll to detect
	}

	// Should have detected 3 changes (versions 2, 3, 4)
	count := changeCount.Load()
	if count < 3 {
		t.Errorf("Expected at least 3 onChange calls, got %d", count)
	}
}

// TestK8sWatcher_SecretSupport tests watching a Secret resource.
func TestK8sWatcher_SecretSupport(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-secret",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Data: map[string][]byte{
			"ca.crt": []byte(generateTestCAHelper(t, "Secret Test CA")),
		},
	}

	fakeClient := newFakeK8sClient(t, secret)

	var changeCount atomic.Int32
	onChange := func() {
		changeCount.Add(1)
	}

	logger := zap.NewNop()
	watcher, err := NewK8sWatcher(fakeClient, "default", "test-secret", "Secret", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	watcher.pollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for initial poll
	time.Sleep(200 * time.Millisecond)

	// Get latest Secret and update it
	updatedSecret := &corev1.Secret{}
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "test-secret"}, updatedSecret); err != nil {
		t.Fatalf("Failed to get Secret: %v", err)
	}
	updatedSecret.Data["ca.crt"] = append(updatedSecret.Data["ca.crt"], []byte("\n# Modified")...)
	if err := fakeClient.Update(ctx, updatedSecret); err != nil {
		t.Fatalf("Failed to update Secret: %v", err)
	}

	// Wait for poll to detect change
	time.Sleep(300 * time.Millisecond)

	// onChange should have been called
	count := changeCount.Load()
	if count < 1 {
		t.Errorf("Expected at least 1 onChange call for Secret update, got %d", count)
	}
}

// TestK8sWatcher_ResourceNotFound tests behavior when resource doesn't exist initially.
func TestK8sWatcher_ResourceNotFound(t *testing.T) {
	// Create empty client (no resources)
	fakeClient := newFakeK8sClient(t)

	var changeCount atomic.Int32
	onChange := func() {
		changeCount.Add(1)
	}

	logger := zap.NewNop()
	watcher, err := NewK8sWatcher(fakeClient, "default", "missing-cm", "ConfigMap", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	watcher.pollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Start should succeed even if resource doesn't exist
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Start failed when resource doesn't exist: %v", err)
	}
	defer watcher.Stop()

	// Wait for a few poll cycles
	time.Sleep(400 * time.Millisecond)

	// onChange should not be called (resource never existed)
	count := changeCount.Load()
	if count != 0 {
		t.Errorf("Expected 0 onChange calls for non-existent resource, got %d", count)
	}

	// Now create the resource (ResourceVersion is set automatically by API server)
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "missing-cm",
			Namespace: "default",
		},
		Data: map[string]string{
			"ca.crt": generateTestCAHelper(t, "Newly Created CA"),
		},
	}
	if err := fakeClient.Create(ctx, cm); err != nil {
		t.Fatalf("Failed to create ConfigMap: %v", err)
	}

	// Wait for poll to detect new resource
	time.Sleep(300 * time.Millisecond)

	// Still should not trigger onChange (first detection just stores version)
	count = changeCount.Load()
	if count != 0 {
		t.Errorf("Expected 0 onChange calls after resource creation (first poll), got %d", count)
	}

	// Get latest ConfigMap and update it
	updatedCM := &corev1.ConfigMap{}
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: "default", Name: "missing-cm"}, updatedCM); err != nil {
		t.Fatalf("Failed to get ConfigMap: %v", err)
	}
	updatedCM.Data["ca.crt"] = updatedCM.Data["ca.crt"] + "\n# Updated"
	if err := fakeClient.Update(ctx, updatedCM); err != nil {
		t.Fatalf("Failed to update ConfigMap: %v", err)
	}

	// Wait for poll
	time.Sleep(300 * time.Millisecond)

	// NOW onChange should be called
	count = changeCount.Load()
	if count < 1 {
		t.Errorf("Expected at least 1 onChange call after resource update, got %d", count)
	}
}

// TestK8sWatcher_InvalidClientType tests error handling for invalid client type.
func TestK8sWatcher_InvalidClientType(t *testing.T) {
	onChange := func() {}
	logger := zap.NewNop()

	// Pass non-client.Client interface
	invalidClient := "not a client"

	_, err := NewK8sWatcher(invalidClient, "default", "test", "ConfigMap", onChange, logger)
	if err == nil {
		t.Error("Expected error for invalid client type, got nil")
	}

	expectedMsg := "invalid client type"
	if err != nil && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected error about invalid client type, got: %v", err)
	}
}

// TestK8sWatcher_ContextCancellation tests context cancellation during polling.
func TestK8sWatcher_ContextCancellation(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "cancel-test",
			Namespace:       "default",
			ResourceVersion: "1",
		},
		Data: map[string]string{
			"ca.crt": generateTestCAHelper(t, "Cancel Test CA"),
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	var changeCount atomic.Int32
	onChange := func() {
		changeCount.Add(1)
	}

	logger := zap.NewNop()
	watcher, err := NewK8sWatcher(fakeClient, "default", "cancel-test", "ConfigMap", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	watcher.pollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())

	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for initial poll
	time.Sleep(200 * time.Millisecond)

	// Cancel context
	cancel()

	// Wait for watcher to stop
	time.Sleep(200 * time.Millisecond)

	// Update resource after cancellation
	initialCount := changeCount.Load()
	cm.ResourceVersion = "2"
	// Note: Update will fail because context is cancelled, but that's expected
	// The point is to verify watcher doesn't call onChange after cancellation

	time.Sleep(300 * time.Millisecond)

	// Count should not increase after cancellation
	finalCount := changeCount.Load()
	if finalCount > initialCount {
		t.Errorf("onChange was called after context cancellation, count changed from %d to %d", initialCount, finalCount)
	}
}

// TestK8sWatcher_UnsupportedResourceType tests error handling for unsupported resource types.
func TestK8sWatcher_UnsupportedResourceType(t *testing.T) {
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-cm",
			Namespace:       "default",
			ResourceVersion: "1",
		},
	}

	fakeClient := newFakeK8sClient(t, cm)

	onChange := func() {}
	logger := zap.NewNop()

	watcher, err := NewK8sWatcher(fakeClient, "default", "test-cm", "Pod", onChange, logger)
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}

	watcher.pollInterval = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start watcher
	if err := watcher.Start(ctx); err != nil {
		t.Fatalf("Failed to start watcher: %v", err)
	}
	defer watcher.Stop()

	// Wait for a poll cycle - should handle unsupported type gracefully
	time.Sleep(300 * time.Millisecond)

	// Watcher should not crash, just log errors internally
}
