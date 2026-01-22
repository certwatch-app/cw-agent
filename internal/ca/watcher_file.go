package ca

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"go.uber.org/zap"
)

// FileWatcher watches a file for changes using fsnotify with debouncing
type FileWatcher struct {
	watcher  *fsnotify.Watcher
	path     string
	debounce time.Duration
	onChange func()
	logger   *zap.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewFileWatcher creates a new file watcher with debouncing
func NewFileWatcher(path string, debounce time.Duration, onChange func(), logger *zap.Logger) (*FileWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &FileWatcher{
		watcher:  watcher,
		path:     path,
		debounce: debounce,
		onChange: onChange,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}, nil
}

// Start starts watching the file for changes
func (w *FileWatcher) Start(ctx context.Context) error {
	// Add path to watcher
	if err := w.watcher.Add(w.path); err != nil {
		return fmt.Errorf("failed to watch file %s: %w", w.path, err)
	}

	w.logger.Info("Started watching file for changes",
		zap.String("path", w.path),
		zap.Duration("debounce", w.debounce))

	go w.watchLoop(ctx)
	return nil
}

// watchLoop is the main watch loop with debouncing
func (w *FileWatcher) watchLoop(ctx context.Context) {
	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	for {
		select {
		case <-ctx.Done():
			w.logger.Debug("File watcher stopped (context canceled)",
				zap.String("path", w.path))
			return

		case <-w.stopCh:
			w.logger.Debug("File watcher stopped",
				zap.String("path", w.path))
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				w.logger.Debug("File watcher events channel closed")
				return
			}

			// Filter relevant events (Write, Create, Remove, Rename)
			// These events indicate the file has been modified
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}

			w.logger.Debug("File change detected",
				zap.String("path", event.Name),
				zap.String("op", event.Op.String()))

			// Reset debounce timer
			// This handles atomic writes (temp file → rename) which trigger multiple events
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(w.debounce)
			debounceCh = debounceTimer.C

		case <-debounceCh:
			// Debounce period expired, file is stable
			w.logger.Info("File change confirmed after debounce",
				zap.String("path", w.path),
				zap.Duration("debounce", w.debounce))

			w.onChange()
			debounceCh = nil

		case err, ok := <-w.watcher.Errors:
			if !ok {
				w.logger.Debug("File watcher errors channel closed")
				return
			}
			w.logger.Error("File watcher error",
				zap.String("path", w.path),
				zap.Error(err))
		}
	}
}

// Stop stops the file watcher (idempotent - safe to call multiple times)
func (w *FileWatcher) Stop() error {
	var err error
	w.stopOnce.Do(func() {
		close(w.stopCh)
		err = w.watcher.Close()
	})
	return err
}
