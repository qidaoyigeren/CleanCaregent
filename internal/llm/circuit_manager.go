package llm

import (
	"sort"
	"sync"
)

type CircuitStatus struct {
	Name string `json:"name"`
	CircuitSnapshot
}

type CircuitManager struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
}

var DefaultCircuitManager = NewCircuitManager()

// NewCircuitManager creates an empty breaker registry.
func NewCircuitManager() *CircuitManager {
	return &CircuitManager{breakers: make(map[string]*CircuitBreaker)}
}

// Register adds or replaces a named circuit breaker.
func (m *CircuitManager) Register(name string, breaker *CircuitBreaker) {
	if m == nil || name == "" || breaker == nil {
		return
	}
	m.mu.Lock()
	m.breakers[name] = breaker
	m.mu.Unlock()
}

// Status returns all registered breaker snapshots ordered by name.
func (m *CircuitManager) Status() []CircuitStatus {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	result := make([]CircuitStatus, 0, len(m.breakers))
	for name, breaker := range m.breakers {
		result = append(result, CircuitStatus{Name: name, CircuitSnapshot: breaker.Snapshot()})
	}
	m.mu.RUnlock()
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// ResetAll closes every registered breaker.
func (m *CircuitManager) ResetAll() int {
	if m == nil {
		return 0
	}
	m.mu.RLock()
	breakers := make([]*CircuitBreaker, 0, len(m.breakers))
	for _, breaker := range m.breakers {
		breakers = append(breakers, breaker)
	}
	m.mu.RUnlock()
	for _, breaker := range breakers {
		breaker.Reset()
	}
	return len(breakers)
}
