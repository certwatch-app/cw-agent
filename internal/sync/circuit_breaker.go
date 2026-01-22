package sync

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/certwatch-app/cw-agent/internal/metrics"
	"go.uber.org/zap"
)

// CircuitState represents the state of the circuit breaker
type CircuitState string

const (
	// StateClosed means requests pass through normally
	StateClosed CircuitState = "closed"
	// StateOpen means requests are rejected immediately
	StateOpen CircuitState = "open"
	// StateHalfOpen means we're testing if the service has recovered
	StateHalfOpen CircuitState = "half-open"
)

// ErrCircuitOpen is returned when the circuit breaker is open
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitBreaker implements the circuit breaker pattern to prevent cascading failures
type CircuitBreaker struct {
	maxFailures     int32
	timeout         time.Duration
	state           atomic.Value // CircuitState
	failureCount    atomic.Int32
	lastFailureTime atomic.Value // time.Time
	successCount    atomic.Int32
	mu              sync.RWMutex
	logger          *zap.Logger
}

// NewCircuitBreaker creates a new circuit breaker
// maxFailures: number of consecutive failures before opening the circuit
// timeout: duration to wait before transitioning from open to half-open
func NewCircuitBreaker(maxFailures int, timeout time.Duration, logger *zap.Logger) *CircuitBreaker {
	cb := &CircuitBreaker{
		//nolint:gosec // G115: Integer overflow impossible; maxFailures is small constant (5-10) from config
		maxFailures: int32(maxFailures),
		timeout:     timeout,
		logger:      logger,
	}
	cb.state.Store(StateClosed)
	cb.lastFailureTime.Store(time.Time{})

	// Initialize metrics
	metrics.SetCircuitBreakerState(string(StateClosed))

	return cb
}

// Call executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func() error) error {
	state := cb.getState()

	switch state {
	case StateOpen:
		// Check if timeout has expired
		lastFailure := cb.getLastFailureTime()
		if time.Since(lastFailure) > cb.timeout {
			cb.logger.Info("circuit breaker transitioning to half-open", zap.Duration("since_last_failure", time.Since(lastFailure)))
			cb.setState(StateHalfOpen)
			return cb.attemptCall(ctx, fn)
		}
		cb.logger.Debug("circuit breaker is open, rejecting request")
		return ErrCircuitOpen

	case StateHalfOpen:
		return cb.attemptCall(ctx, fn)

	default: // StateClosed
		return cb.attemptCall(ctx, fn)
	}
}

// attemptCall tries to execute the function and records the result
func (cb *CircuitBreaker) attemptCall(ctx context.Context, fn func() error) error {
	err := fn()
	if err != nil {
		cb.recordFailure()
		return err
	}
	cb.recordSuccess()
	return nil
}

// recordFailure increments the failure count and opens the circuit if threshold is reached
func (cb *CircuitBreaker) recordFailure() {
	//nolint:gosec // G115: Integer overflow impossible; circuit opens at maxFailures (~5-10), counters reset on state change
	failureCount := cb.failureCount.Add(1)
	cb.lastFailureTime.Store(time.Now())
	cb.successCount.Store(0)

	state := cb.getState()
	if state == StateHalfOpen {
		// In half-open state, any failure immediately opens the circuit
		cb.logger.Warn("circuit breaker opening after failure in half-open state")
		cb.setState(StateOpen)
		return
	}

	if state == StateClosed && failureCount >= cb.maxFailures {
		cb.logger.Warn("circuit breaker opening after max failures",
			zap.Int32("failureCount", failureCount),
			zap.Int32("maxFailures", cb.maxFailures),
		)
		cb.setState(StateOpen)
	}
}

// recordSuccess resets the failure count and closes the circuit if in half-open state
func (cb *CircuitBreaker) recordSuccess() {
	cb.failureCount.Store(0)
	successCount := cb.successCount.Add(1)

	state := cb.getState()
	if state == StateHalfOpen && successCount >= 1 {
		// After one successful call in half-open state, close the circuit
		cb.logger.Info("circuit breaker closing after successful test")
		cb.setState(StateClosed)
	}
}

// getState returns the current state of the circuit breaker
func (cb *CircuitBreaker) getState() CircuitState {
	return cb.state.Load().(CircuitState)
}

// setState updates the state of the circuit breaker
func (cb *CircuitBreaker) setState(newState CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	oldState := cb.state.Load().(CircuitState)
	if oldState != newState {
		cb.state.Store(newState)

		// Update Prometheus metric
		metrics.SetCircuitBreakerState(string(newState))

		cb.logger.Info("circuit breaker state changed",
			zap.String("old_state", string(oldState)),
			zap.String("new_state", string(newState)),
		)

		// Reset counters on state change
		switch newState {
		case StateClosed:
			cb.failureCount.Store(0)
			cb.successCount.Store(0)
		case StateHalfOpen:
			cb.successCount.Store(0)
		}
	}
}

// getLastFailureTime returns the time of the last failure
func (cb *CircuitBreaker) getLastFailureTime() time.Time {
	t := cb.lastFailureTime.Load()
	if t == nil {
		return time.Time{}
	}
	return t.(time.Time)
}

// GetState returns the current state (for monitoring/metrics)
func (cb *CircuitBreaker) GetState() CircuitState {
	return cb.getState()
}

// GetFailureCount returns the current failure count (for monitoring/metrics)
func (cb *CircuitBreaker) GetFailureCount() int32 {
	return cb.failureCount.Load()
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state.Store(StateClosed)
	cb.failureCount.Store(0)
	cb.successCount.Store(0)
	cb.lastFailureTime.Store(time.Time{})
	cb.logger.Info("circuit breaker manually reset")
}
