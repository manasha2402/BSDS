package main

import (
	"sync"
	"time"
)

const (
	StateClosed   = "CLOSED"
	StateOpen     = "OPEN"
	StateHalfOpen = "HALF_OPEN"
)

// CircuitBreaker protects calls to an external service
type CircuitBreaker struct {
	mu sync.RWMutex

	state        string        // current state
	failures     int           // consecutive failure count
	threshold    int           // how many failures before opening
	timeout      time.Duration // how long to stay open before retrying
	lastFailedAt time.Time     // when the circuit was opened
}

// NewCircuitBreaker creates a breaker.
// Example: NewCircuitBreaker(5, 10*time.Second)
//
//	→ opens after 5 failures, retries after 10 seconds
func NewCircuitBreaker(threshold int, timeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:     StateClosed,
		threshold: threshold,
		timeout:   timeout,
	}
}

// AllowRequest decides: should we call the downstream service?
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {

	case StateClosed:
		// Normal operation — let the request through
		return true

	case StateOpen:
		// Circuit is tripped. Has enough time passed to retry?
		if time.Since(cb.lastFailedAt) > cb.timeout {
			// Transition to half-open: allow ONE test request
			cb.state = StateHalfOpen
			return true
		}
		// Still within timeout — block the call
		return false

	case StateHalfOpen:
		// We're already testing with one request — block the rest
		return false
	}

	return false
}

// RecordSuccess is called when the downstream service responds successfully.
// Resets the breaker back to normal.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
}

// RecordFailure is called when the downstream service fails.
// After enough failures, the circuit opens.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.lastFailedAt = time.Now()

	if cb.failures >= cb.threshold {
		cb.state = StateOpen
	}
}

// GetState returns the current state for logging and API responses
func (cb *CircuitBreaker) GetState() string {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
