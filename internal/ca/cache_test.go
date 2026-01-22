package ca

import (
	"context"
	"crypto/x509"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestCACacheManager_ConcurrentGetOrLoad tests concurrent access to GetOrLoad
// with the same cache key to verify double-checked locking and single load.
func TestCACacheManager_ConcurrentGetOrLoad(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	// Generate test certificate
	testCA := generateTestCAHelper(t, "Concurrent Test CA")

	// Create inline source
	source := NewInlineSource(testCA, loader, logger)

	// Track number of actual loads (should be 1 despite concurrent calls)
	var loadCount atomic.Int32

	// Use a custom loader that tracks loads
	sourceWithCount := &trackingSource{
		source:    source,
		loadCount: &loadCount,
	}

	// Launch 100 concurrent goroutines all trying to load same cache key
	const numGoroutines = 100
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	ctx := context.Background()
	errors := make(chan error, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			pool, err := cacheManager.GetOrLoad(ctx, []CASource{sourceWithCount}, "custom")
			if err != nil {
				errors <- err
				return
			}
			if pool == nil {
				errors <- fmt.Errorf("got nil pool")
			}
		}()
	}

	wg.Wait()
	close(errors)

	// Check for errors
	for err := range errors {
		t.Errorf("Goroutine error: %v", err)
	}

	// Verify source was only loaded once (double-checked locking works)
	loads := loadCount.Load()
	if loads != 1 {
		t.Errorf("Expected source to be loaded exactly once, got %d loads", loads)
	}
}

// TestCACacheManager_ConcurrentInvalidation tests concurrent invalidation
// during active loads to ensure thread safety.
func TestCACacheManager_ConcurrentInvalidation(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	// Generate test certificate
	testCA := generateTestCAHelper(t, "Invalidation Test CA")
	source := NewInlineSource(testCA, loader, logger)
	sourceID := source.ID()

	ctx := context.Background()

	// Pre-load cache
	_, err := cacheManager.GetOrLoad(ctx, []CASource{source}, "custom")
	if err != nil {
		t.Fatalf("Failed to pre-load cache: %v", err)
	}

	// Concurrently invalidate and load
	var wg sync.WaitGroup
	const numOperations = 50

	// Concurrent invalidations
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func() {
			defer wg.Done()
			cacheManager.InvalidateCache(sourceID)
		}()
	}

	// Concurrent loads
	wg.Add(numOperations)
	for i := 0; i < numOperations; i++ {
		go func() {
			defer wg.Done()
			_, _ = cacheManager.GetOrLoad(ctx, []CASource{source}, "custom")
		}()
	}

	wg.Wait()

	// Verify cache manager is still functional
	pool, err := cacheManager.GetOrLoad(ctx, []CASource{source}, "custom")
	if err != nil {
		t.Errorf("Cache manager broken after concurrent invalidation: %v", err)
	}
	if pool == nil {
		t.Error("Got nil pool after concurrent invalidation")
	}
}

// TestCACacheManager_ConcurrentCallbacks tests callback registration and triggering
// with panic recovery.
func TestCACacheManager_ConcurrentCallbacks(t *testing.T) {
	tmpDir := t.TempDir()
	testCA := generateTestCAHelper(t, "Callback Test CA")
	caFile := writeTestFileHelper(t, tmpDir, "test.pem", testCA, 0644)

	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	// Track callback invocations
	var callbackCount atomic.Int32

	// Register multiple callbacks, some that panic
	for i := 0; i < 10; i++ {
		idx := i
		cacheManager.OnReload(func(sourceID string) {
			if idx%3 == 0 {
				// Every third callback panics
				panic(fmt.Sprintf("callback %d panicked", idx))
			}
			callbackCount.Add(1)
		})
	}

	// Create file source and start watching
	source := NewFileSource(caFile, loader, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cacheManager.StartWatching(ctx, source); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Trigger callbacks by modifying the file (which triggers watcher's onChange)
	if err := os.WriteFile(caFile, []byte(testCA+"modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Wait for file watcher debounce (2s) + callbacks to complete
	time.Sleep(3 * time.Second)

	// Verify non-panicking callbacks were called
	// Expected: 10 callbacks, 4 panic (0,3,6,9), 6 succeed
	count := callbackCount.Load()
	if count < 6 {
		t.Errorf("Expected at least 6 successful callbacks, got %d", count)
	}

	// Verify cache manager is still functional after callback panics
	pool, err := cacheManager.GetOrLoad(context.Background(), []CASource{source}, "custom")
	if err != nil {
		t.Errorf("Cache manager broken after callback panics: %v", err)
	}
	if pool == nil {
		t.Error("Got nil pool after callback panics")
	}
}

// TestCACacheManager_DoubleCheckedLocking verifies double-checked locking pattern
// by simulating race condition at lock boundary.
func TestCACacheManager_DoubleCheckedLocking(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	testCA := generateTestCAHelper(t, "DCL Test CA")
	source := NewInlineSource(testCA, loader, logger)

	ctx := context.Background()

	// Launch many concurrent loads
	const numGoroutines = 50
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-start // Synchronize start to increase lock contention
			_, _ = cacheManager.GetOrLoad(ctx, []CASource{source}, "custom")
		}()
	}

	// Release all goroutines at once
	close(start)
	wg.Wait()

	// Verify cache has only one entry
	cacheManager.mu.RLock()
	cacheSize := len(cacheManager.pools)
	cacheManager.mu.RUnlock()

	if cacheSize != 1 {
		t.Errorf("Expected cache to have 1 entry, got %d (double-checked locking failed)", cacheSize)
	}
}

// TestCACacheManager_TrustModes tests all trust mode combinations.
func TestCACacheManager_TrustModes(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	testCA := generateTestCAHelper(t, "Trust Mode Test CA")
	source := NewInlineSource(testCA, loader, logger)

	ctx := context.Background()

	tests := []struct {
		name        string
		sources     []CASource
		trustMode   string
		expectError bool
	}{
		{
			name:        "system trust mode",
			sources:     []CASource{},
			trustMode:   "system",
			expectError: false,
		},
		{
			name:        "custom trust mode with sources",
			sources:     []CASource{source},
			trustMode:   "custom",
			expectError: false,
		},
		{
			name:        "custom trust mode without sources",
			sources:     []CASource{},
			trustMode:   "custom",
			expectError: true,
		},
		{
			name:        "combined trust mode",
			sources:     []CASource{source},
			trustMode:   "combined",
			expectError: false,
		},
		{
			name:        "invalid trust mode",
			sources:     []CASource{source},
			trustMode:   "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := cacheManager.GetOrLoad(ctx, tt.sources, tt.trustMode)

			if tt.expectError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					// Skip if system CAs unavailable
					if tt.trustMode == "system" || tt.trustMode == "combined" {
						t.Skipf("System CAs unavailable: %v", err)
					}
					t.Errorf("Unexpected error: %v", err)
				}
				if pool == nil {
					t.Error("Expected non-nil pool")
				}
			}
		})
	}
}

// TestCACacheManager_CacheKeyComputation tests cache key generation.
func TestCACacheManager_CacheKeyComputation(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	testCA1 := generateTestCAHelper(t, "Test CA 1")
	testCA2 := generateTestCAHelper(t, "Test CA 2")

	source1 := NewInlineSource(testCA1, loader, logger)
	source2 := NewInlineSource(testCA2, loader, logger)

	// Test 1: Same sources in different order should produce same key
	key1 := cacheManager.computeCacheKey([]CASource{source1, source2}, "custom")
	key2 := cacheManager.computeCacheKey([]CASource{source2, source1}, "custom")

	if key1 != key2 {
		t.Errorf("Expected same cache key for sources in different order\nGot:\n  %s\n  %s", key1, key2)
	}

	// Test 2: Different trust modes should produce different keys
	key3 := cacheManager.computeCacheKey([]CASource{source1}, "custom")
	key4 := cacheManager.computeCacheKey([]CASource{source1}, "system")

	if key3 == key4 {
		t.Error("Expected different cache keys for different trust modes")
	}

	// Test 3: Different sources should produce different keys
	key5 := cacheManager.computeCacheKey([]CASource{source1}, "custom")
	key6 := cacheManager.computeCacheKey([]CASource{source2}, "custom")

	if key5 == key6 {
		t.Error("Expected different cache keys for different sources")
	}
}

// TestCACacheManager_ContentHashChange tests content hash detection.
func TestCACacheManager_ContentHashChange(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	// Generate two different certificates
	testCA1 := generateTestCAHelper(t, "Test CA 1")
	testCA2 := generateTestCAHelper(t, "Test CA 2")

	cert1 := parseCertFromPEM(t, testCA1)
	cert2 := parseCertFromPEM(t, testCA2)

	// Compute hashes
	hash1 := cacheManager.computeContentHash([]*x509.Certificate{cert1})
	hash2 := cacheManager.computeContentHash([]*x509.Certificate{cert2})

	if hash1 == hash2 {
		t.Error("Expected different hashes for different certificates")
	}

	// Test 3: Same certs in different order should produce same hash (sorted)
	hash3 := cacheManager.computeContentHash([]*x509.Certificate{cert1, cert2})
	hash4 := cacheManager.computeContentHash([]*x509.Certificate{cert2, cert1})

	if hash3 != hash4 {
		t.Error("Expected same hash for certs in different order (should be sorted)")
	}
}

// TestCACacheManager_CallbackPanicRecovery verifies panic recovery in callbacks.
func TestCACacheManager_CallbackPanicRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	testCA := generateTestCAHelper(t, "Panic Recovery Test CA")
	caFile := writeTestFileHelper(t, tmpDir, "test.pem", testCA, 0644)

	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	// Register callback that always panics
	cacheManager.OnReload(func(sourceID string) {
		panic("intentional panic")
	})

	// Track if second callback is called
	var called atomic.Bool
	cacheManager.OnReload(func(sourceID string) {
		called.Store(true)
	})

	// Create file source and start watching
	source := NewFileSource(caFile, loader, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := cacheManager.StartWatching(ctx, source); err != nil {
		t.Fatalf("Failed to start watching: %v", err)
	}

	// Give watcher time to start
	time.Sleep(100 * time.Millisecond)

	// Trigger callbacks by modifying file
	if err := os.WriteFile(caFile, []byte(testCA+"modified"), 0644); err != nil {
		t.Fatalf("Failed to modify file: %v", err)
	}

	// Wait for file watcher debounce (2s) + callbacks
	time.Sleep(3 * time.Second)

	// Verify second callback was called despite first panicking
	if !called.Load() {
		t.Error("Second callback was not called after first callback panicked")
	}

	// Verify cache manager still works
	pool, err := cacheManager.GetOrLoad(context.Background(), []CASource{source}, "custom")
	if err != nil {
		t.Errorf("Cache manager broken after callback panic: %v", err)
	}
	if pool == nil {
		t.Error("Got nil pool after callback panic")
	}
}

// TestCACacheManager_Shutdown tests shutdown cleanup.
func TestCACacheManager_Shutdown(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)

	// Load some data
	testCA := generateTestCAHelper(t, "Shutdown Test CA")
	source := NewInlineSource(testCA, loader, logger)
	ctx := context.Background()

	_, err := cacheManager.GetOrLoad(ctx, []CASource{source}, "custom")
	if err != nil {
		t.Fatalf("Failed to load initial data: %v", err)
	}

	// Register callbacks
	cacheManager.OnReload(func(sourceID string) {
		// No-op
	})

	// Verify data exists before shutdown
	cacheManager.mu.RLock()
	poolCount := len(cacheManager.pools)
	callbackCount := len(cacheManager.reloadCallbacks)
	cacheManager.mu.RUnlock()

	if poolCount == 0 {
		t.Error("Expected pools before shutdown")
	}
	if callbackCount == 0 {
		t.Error("Expected callbacks before shutdown")
	}

	// Shutdown
	cacheManager.Shutdown()

	// Verify data cleared
	cacheManager.mu.RLock()
	poolCountAfter := len(cacheManager.pools)
	callbackCountAfter := len(cacheManager.reloadCallbacks)
	watcherCountAfter := len(cacheManager.watchers)
	cacheManager.mu.RUnlock()

	if poolCountAfter != 0 {
		t.Errorf("Expected 0 pools after shutdown, got %d", poolCountAfter)
	}
	if callbackCountAfter != 0 {
		t.Errorf("Expected 0 callbacks after shutdown, got %d", callbackCountAfter)
	}
	if watcherCountAfter != 0 {
		t.Errorf("Expected 0 watchers after shutdown, got %d", watcherCountAfter)
	}
}

// TestCACacheManager_EmptySources tests error handling with empty sources.
func TestCACacheManager_EmptySources(t *testing.T) {
	logger := zap.NewNop()
	loader := NewLoader(logger)
	cacheManager := NewCACacheManager(loader, logger)
	defer cacheManager.Shutdown()

	ctx := context.Background()

	// Test with no sources and non-system trust mode
	_, err := cacheManager.GetOrLoad(ctx, []CASource{}, "custom")
	if err == nil {
		t.Error("Expected error for empty sources with custom trust mode")
	}

	expectedMsg := "no CA sources provided"
	if err != nil && err.Error()[:len(expectedMsg)] != expectedMsg {
		t.Errorf("Expected error about no CA sources, got: %v", err)
	}
}

// trackingSource wraps a CASource and tracks how many times Load is called.
type trackingSource struct {
	source    CASource
	loadCount *atomic.Int32
}

func (s *trackingSource) Load(ctx context.Context) ([]*x509.Certificate, error) {
	s.loadCount.Add(1)
	return s.source.Load(ctx)
}

func (s *trackingSource) ID() string {
	return s.source.ID()
}

func (s *trackingSource) Type() string {
	return s.source.Type()
}
