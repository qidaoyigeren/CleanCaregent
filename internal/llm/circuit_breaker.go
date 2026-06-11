package llm

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("llm circuit breaker is open")

type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"
	CircuitOpen     CircuitState = "open"
	CircuitHalfOpen CircuitState = "half_open"
)

type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failures         int
	failureThreshold int
	openTimeout      time.Duration
	openedAt         time.Time
	probeInFlight    bool
	now              func() time.Time
}

func NewCircuitBreaker(failureThreshold int, openTimeout time.Duration) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if openTimeout <= 0 {
		openTimeout = 30 * time.Second
	}
	return &CircuitBreaker{
		state:            CircuitClosed,
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
		now:              time.Now,
	}
}

func (b *CircuitBreaker) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if b.now().Sub(b.openedAt) < b.openTimeout {
			return false
		}
		b.state = CircuitHalfOpen
		b.probeInFlight = true
		return true
	case CircuitHalfOpen:
		if b.probeInFlight {
			return false
		}
		b.probeInFlight = true
		return true
	default:
		return false
	}
}

func (b *CircuitBreaker) Record(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if success {
		b.failures = 0
		b.probeInFlight = false
		b.state = CircuitClosed
		return
	}
	b.probeInFlight = false
	if b.state == CircuitHalfOpen {
		b.openLocked()
		return
	}
	b.failures++
	if b.failures >= b.failureThreshold {
		b.openLocked()
	}
}

func (b *CircuitBreaker) State() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == CircuitOpen && b.now().Sub(b.openedAt) >= b.openTimeout {
		return CircuitHalfOpen
	}
	return b.state
}

func (b *CircuitBreaker) openLocked() {
	b.state = CircuitOpen
	b.openedAt = b.now()
	b.probeInFlight = false
}
