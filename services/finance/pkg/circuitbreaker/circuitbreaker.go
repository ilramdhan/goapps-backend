// Package circuitbreaker provides a simple circuit breaker implementation.
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// State represents the circuit breaker state.
type State int

// State constants for circuit breaker.
const (
	StateClosed   State = iota // Normal operation
	StateOpen                  // Failing, reject requests
	StateHalfOpen              // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Common errors
var (
	ErrCircuitOpen     = errors.New("circuit breaker is open")
	ErrTooManyFailures = errors.New("too many failures")
)

// Settings configures the circuit breaker.
type Settings struct {
	// Name of the circuit breaker (for logging/metrics)
	Name string

	// MaxFailures is the number of failures before opening the circuit.
	MaxFailures int

	// Timeout is how long the circuit stays open before moving to half-open.
	Timeout time.Duration

	// MaxHalfOpenRequests is how many requests to allow in half-open state.
	MaxHalfOpenRequests int

	// OnStateChange is called when state changes.
	OnStateChange func(name string, from, to State)
}

// DefaultSettings returns sensible defaults.
func DefaultSettings(name string) Settings {
	return Settings{
		Name:                name,
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	settings         Settings
	mu               sync.RWMutex
	state            State
	failures         int
	successes        int
	lastFailureTime  time.Time
	halfOpenRequests int
}

// New creates a new circuit breaker.
func New(settings Settings) *CircuitBreaker {
	return &CircuitBreaker{
		settings: settings,
		state:    StateClosed,
	}
}

// Execute runs the given function with circuit breaker protection.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(ctx context.Context) error) error {
	if err := cb.beforeRequest(); err != nil {
		return err
	}

	err := fn(ctx)
	cb.afterRequest(err)

	return err
}

// State returns the current state.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Failures returns the current failure count.
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}

func (cb *CircuitBreaker) beforeRequest() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if timeout has passed
		if time.Since(cb.lastFailureTime) > cb.settings.Timeout {
			cb.toHalfOpen()
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		if cb.halfOpenRequests >= cb.settings.MaxHalfOpenRequests {
			return ErrCircuitOpen
		}
		cb.halfOpenRequests++
		return nil
	}

	return nil
}

func (cb *CircuitBreaker) afterRequest(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.settings.MaxHalfOpenRequests {
			cb.toClosed()
		}
	case StateOpen:
		// No action needed in open state for success
	}
}

func (cb *CircuitBreaker) onFailure() {
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.settings.MaxFailures {
			cb.toOpen()
		}
	case StateHalfOpen:
		cb.toOpen()
	case StateOpen:
		// Already open, no state change needed
	}
}

func (cb *CircuitBreaker) toClosed() {
	from := cb.state
	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.notifyStateChange(from, StateClosed)
}

func (cb *CircuitBreaker) toOpen() {
	from := cb.state
	cb.state = StateOpen
	cb.notifyStateChange(from, StateOpen)
}

func (cb *CircuitBreaker) toHalfOpen() {
	from := cb.state
	cb.state = StateHalfOpen
	cb.halfOpenRequests = 0
	cb.successes = 0
	cb.notifyStateChange(from, StateHalfOpen)
}

func (cb *CircuitBreaker) notifyStateChange(from, to State) {
	if cb.settings.OnStateChange != nil && from != to {
		go cb.settings.OnStateChange(cb.settings.Name, from, to)
	}
}
