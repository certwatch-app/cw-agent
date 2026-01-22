package ca

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// CACacheManager manages CA certificate pools with hot-reload support.
// It maintains a cache of loaded CA pools keyed by source IDs and provides
// automatic invalidation when sources change.
type CACacheManager struct {
	mu              sync.RWMutex
	pools           map[string]*cachedPool        // keyed by composite source ID
	watchers        map[string]context.CancelFunc // keyed by source ID
	loader          *Loader
	logger          *zap.Logger
	reloadCallbacks []func(sourceID string)
}

// cachedPool represents a cached CA certificate pool with metadata
type cachedPool struct {
	pool       *x509.CertPool
	certs      []*x509.Certificate
	hash       [32]byte // SHA256 of concatenated DER bytes
	lastReload time.Time
	sourceIDs  []string // Source IDs that contributed to this pool
}

// NewCACacheManager creates a new CA cache manager
func NewCACacheManager(loader *Loader, logger *zap.Logger) *CACacheManager {
	return &CACacheManager{
		pools:           make(map[string]*cachedPool),
		watchers:        make(map[string]context.CancelFunc),
		loader:          loader,
		logger:          logger,
		reloadCallbacks: make([]func(sourceID string), 0),
	}
}

// GetOrLoad retrieves a CA pool from cache or loads it from sources.
// It combines certificates from all sources based on the trust mode.
// trustMode can be: "system", "custom", or "combined"
func (m *CACacheManager) GetOrLoad(ctx context.Context, sources []CASource, trustMode string) (*x509.CertPool, error) {
	if len(sources) == 0 && trustMode != "system" {
		return nil, fmt.Errorf("no CA sources provided and trust mode is not 'system'")
	}

	// Compute cache key from source IDs
	cacheKey := m.computeCacheKey(sources, trustMode)

	// Fast path: check cache with read lock
	m.mu.RLock()
	cached, found := m.pools[cacheKey]
	m.mu.RUnlock()

	if found {
		m.logger.Debug("CA pool cache hit",
			zap.String("cache_key", cacheKey),
			zap.Time("last_reload", cached.lastReload))
		return cached.pool, nil
	}

	// Slow path: load from sources with write lock
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have loaded)
	if cached, found := m.pools[cacheKey]; found {
		return cached.pool, nil
	}

	m.logger.Info("Loading CA pool from sources",
		zap.String("trust_mode", trustMode),
		zap.Int("source_count", len(sources)))

	// Load certificates from all custom sources
	var allCerts []*x509.Certificate
	var sourceIDs []string

	for _, source := range sources {
		certs, err := source.Load(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to load from source %s: %w", source.ID(), err)
		}

		m.logger.Debug("Loaded certificates from source",
			zap.String("source_id", source.ID()),
			zap.String("source_type", source.Type()),
			zap.Int("cert_count", len(certs)))

		allCerts = append(allCerts, certs...)
		sourceIDs = append(sourceIDs, source.ID())
	}

	// Build CA pool based on trust mode
	var pool *x509.CertPool
	var err error

	switch trustMode {
	case "system":
		// Use system CA pool only
		pool, err = x509.SystemCertPool()
		if err != nil {
			return nil, fmt.Errorf("failed to load system CA pool: %w", err)
		}
		m.logger.Debug("Using system CA pool")

	case "custom":
		// Use only custom CAs
		if len(allCerts) == 0 {
			return nil, fmt.Errorf("no custom CA certificates loaded and trust mode is 'custom'")
		}
		pool = x509.NewCertPool()
		for _, cert := range allCerts {
			pool.AddCert(cert)
		}
		m.logger.Debug("Using custom CA pool", zap.Int("cert_count", len(allCerts)))

	case "combined":
		// Combine system and custom CAs
		pool, err = x509.SystemCertPool()
		if err != nil {
			// If system pool unavailable, start with empty pool
			m.logger.Warn("System CA pool unavailable, using custom CAs only", zap.Error(err))
			pool = x509.NewCertPool()
		}
		for _, cert := range allCerts {
			pool.AddCert(cert)
		}
		m.logger.Debug("Using combined CA pool",
			zap.Int("custom_cert_count", len(allCerts)))

	default:
		return nil, fmt.Errorf("invalid trust mode: %s", trustMode)
	}

	// Compute content hash for change detection
	contentHash := m.computeContentHash(allCerts)

	// Cache the pool
	m.pools[cacheKey] = &cachedPool{
		pool:       pool,
		certs:      allCerts,
		hash:       contentHash,
		lastReload: time.Now(),
		sourceIDs:  sourceIDs,
	}

	m.logger.Info("CA pool loaded and cached",
		zap.String("cache_key", cacheKey),
		zap.Int("total_custom_certs", len(allCerts)),
		zap.String("content_hash", fmt.Sprintf("%x", contentHash[:8])))

	return pool, nil
}

// StartWatching starts watching a CA source for changes and triggers reload on change.
// The watcher runs in a background goroutine until the context is cancelled or StopWatching is called.
func (m *CACacheManager) StartWatching(ctx context.Context, source CASource) error {
	sourceID := source.ID()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already watching
	if _, exists := m.watchers[sourceID]; exists {
		m.logger.Debug("Already watching source", zap.String("source_id", sourceID))
		return nil
	}

	// Create watcher based on source type
	var watcher interface {
		Start(context.Context) error
		Stop() error
	}
	var err error

	onChange := func() {
		m.logger.Info("CA source changed, invalidating cache", zap.String("source_id", sourceID))
		m.InvalidateCache(sourceID)
		m.triggerReloadCallbacks(sourceID)
	}

	switch source.Type() {
	case "file", "pkcs12":
		// File-based sources use fsnotify watcher
		fileSource, ok := source.(interface{ GetPath() string })
		if !ok {
			return fmt.Errorf("file source does not implement GetPath()")
		}
		watcher, err = NewFileWatcher(fileSource.GetPath(), 2*time.Second, onChange, m.logger)

	case "configmap":
		// ConfigMap sources use K8s watcher
		cmSource, ok := source.(interface {
			GetClient() interface{}
			GetNamespace() string
			GetName() string
		})
		if !ok {
			return fmt.Errorf("configmap source does not implement required methods")
		}
		watcher, err = NewK8sWatcher(cmSource.GetClient(), cmSource.GetNamespace(), cmSource.GetName(), "ConfigMap", onChange, m.logger)

	case "secret":
		// Secret sources use K8s watcher
		secretSource, ok := source.(interface {
			GetClient() interface{}
			GetNamespace() string
			GetName() string
		})
		if !ok {
			return fmt.Errorf("secret source does not implement required methods")
		}
		watcher, err = NewK8sWatcher(secretSource.GetClient(), secretSource.GetNamespace(), secretSource.GetName(), "Secret", onChange, m.logger)

	case "inline":
		// Inline sources don't support watching (static data)
		m.logger.Debug("Inline source does not support watching", zap.String("source_id", sourceID))
		return nil

	default:
		return fmt.Errorf("unsupported source type for watching: %s", source.Type())
	}

	if err != nil {
		return fmt.Errorf("failed to create watcher for source %s: %w", sourceID, err)
	}

	// Start watcher in background
	watcherCtx, cancel := context.WithCancel(ctx)
	m.watchers[sourceID] = cancel

	go func() {
		if err := watcher.Start(watcherCtx); err != nil {
			m.logger.Error("Watcher error",
				zap.String("source_id", sourceID),
				zap.Error(err))
		}
	}()

	m.logger.Info("Started watching CA source",
		zap.String("source_id", sourceID),
		zap.String("source_type", source.Type()))

	return nil
}

// StopWatching stops watching a specific CA source
func (m *CACacheManager) StopWatching(sourceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cancel, exists := m.watchers[sourceID]
	if !exists {
		return fmt.Errorf("no watcher found for source: %s", sourceID)
	}

	cancel()
	delete(m.watchers, sourceID)

	m.logger.Info("Stopped watching CA source", zap.String("source_id", sourceID))
	return nil
}

// OnReload registers a callback function to be called when any CA source is reloaded
func (m *CACacheManager) OnReload(callback func(sourceID string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reloadCallbacks = append(m.reloadCallbacks, callback)
}

// InvalidateCache removes cached entries that depend on the given source ID
func (m *CACacheManager) InvalidateCache(sourceID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Find and remove all cached pools that use this source
	keysToDelete := make([]string, 0)
	for key, cached := range m.pools {
		for _, sid := range cached.sourceIDs {
			if sid == sourceID {
				keysToDelete = append(keysToDelete, key)
				break
			}
		}
	}

	for _, key := range keysToDelete {
		delete(m.pools, key)
		m.logger.Debug("Invalidated cache entry",
			zap.String("cache_key", key),
			zap.String("source_id", sourceID))
	}

	if len(keysToDelete) > 0 {
		m.logger.Info("Cache invalidated",
			zap.String("source_id", sourceID),
			zap.Int("entries_removed", len(keysToDelete)))
	}
}

// Shutdown stops all watchers and clears the cache
func (m *CACacheManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Stop all watchers
	for sourceID, cancel := range m.watchers {
		cancel()
		m.logger.Debug("Stopped watcher during shutdown", zap.String("source_id", sourceID))
	}

	// Clear all data
	m.watchers = make(map[string]context.CancelFunc)
	m.pools = make(map[string]*cachedPool)
	m.reloadCallbacks = make([]func(sourceID string), 0)

	m.logger.Info("CA cache manager shutdown complete")
}

// computeCacheKey generates a deterministic cache key from sources and trust mode
func (m *CACacheManager) computeCacheKey(sources []CASource, trustMode string) string {
	// Sort source IDs for deterministic key generation
	sourceIDs := make([]string, len(sources))
	for i, source := range sources {
		sourceIDs[i] = source.ID()
	}
	sort.Strings(sourceIDs)

	// Combine trust mode and source IDs
	return fmt.Sprintf("%s:%s", trustMode, strings.Join(sourceIDs, ","))
}

// computeContentHash computes SHA256 hash of all certificate DER bytes concatenated
func (m *CACacheManager) computeContentHash(certs []*x509.Certificate) [32]byte {
	hasher := sha256.New()

	// Sort certificates by subject for deterministic hash
	sortedCerts := make([]*x509.Certificate, len(certs))
	copy(sortedCerts, certs)
	sort.Slice(sortedCerts, func(i, j int) bool {
		return sortedCerts[i].Subject.String() < sortedCerts[j].Subject.String()
	})

	// Hash concatenated DER bytes
	for _, cert := range sortedCerts {
		hasher.Write(cert.Raw)
	}

	var hash [32]byte
	copy(hash[:], hasher.Sum(nil))
	return hash
}

// triggerReloadCallbacks invokes all registered reload callbacks
func (m *CACacheManager) triggerReloadCallbacks(sourceID string) {
	// Note: We call callbacks without holding the lock to avoid deadlocks
	// Make a copy of callbacks first
	m.mu.RLock()
	callbacks := make([]func(sourceID string), len(m.reloadCallbacks))
	copy(callbacks, m.reloadCallbacks)
	m.mu.RUnlock()

	for _, callback := range callbacks {
		// Call in goroutine to avoid blocking
		go func(cb func(string)) {
			defer func() {
				if r := recover(); r != nil {
					m.logger.Error("Reload callback panicked",
						zap.String("source_id", sourceID),
						zap.Any("panic", r))
				}
			}()
			cb(sourceID)
		}(callback)
	}
}
