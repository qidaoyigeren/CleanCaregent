package llm

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("模型服务熔断器处于开启状态")

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
	recoveryHistory  []time.Duration
	successes        uint64
	totalFailures    uint64
}

type CircuitSnapshot struct {
	State           CircuitState    `json:"state"`
	Failures        int             `json:"failures"`
	Successes       uint64          `json:"successes"`
	TotalFailures   uint64          `json:"total_failures"`
	OpenTimeout     time.Duration   `json:"open_timeout"`
	OpenedAt        time.Time       `json:"opened_at,omitempty"`
	RecoveryHistory []time.Duration `json:"recovery_history,omitempty"`
	ProbeInFlight   bool            `json:"probe_in_flight"`
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
		if b.state == CircuitHalfOpen && !b.openedAt.IsZero() {
			recovery := b.now().Sub(b.openedAt)
			if recovery > 0 {
				b.recoveryHistory = append(b.recoveryHistory, recovery)
				if len(b.recoveryHistory) > 20 {
					b.recoveryHistory = append(
						[]time.Duration(nil),
						b.recoveryHistory[len(b.recoveryHistory)-20:]...,
					)
				}
				b.openTimeout = adaptiveOpenTimeout(recovery)
			}
		}
		b.successes++
		b.failures = 0
		b.probeInFlight = false
		b.state = CircuitClosed
		return
	}
	b.probeInFlight = false
	b.totalFailures++
	if b.state == CircuitHalfOpen {
		b.openLocked()
		return
	}
	b.failures++
	if b.failures >= b.failureThreshold {
		b.openLocked()
	}
}

// Snapshot returns the current breaker state and counters.
func (b *CircuitBreaker) Snapshot() CircuitSnapshot {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.state
	if state == CircuitOpen && b.now().Sub(b.openedAt) >= b.openTimeout {
		state = CircuitHalfOpen
	}
	return CircuitSnapshot{
		State:           state,
		Failures:        b.failures,
		Successes:       b.successes,
		TotalFailures:   b.totalFailures,
		OpenTimeout:     b.openTimeout,
		OpenedAt:        b.openedAt,
		RecoveryHistory: append([]time.Duration(nil), b.recoveryHistory...),
		ProbeInFlight:   b.probeInFlight,
	}
}

// Reset closes the breaker and clears transient failure state.
func (b *CircuitBreaker) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = CircuitClosed
	b.failures = 0
	b.openedAt = time.Time{}
	b.probeInFlight = false
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

func adaptiveOpenTimeout(recovery time.Duration) time.Duration {
	value := recovery * 2
	if value < 30*time.Second {
		return 30 * time.Second
	}
	if value > 5*time.Minute {
		return 5 * time.Minute
	}
	return value
}
