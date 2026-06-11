package llm

import (
	"testing"
	"time"
)

func TestCircuitBreakerTransitionsClosedOpenHalfOpenClosed(t *testing.T) {
	now := time.Unix(100, 0)
	breaker := NewCircuitBreaker(2, time.Minute)
	breaker.now = func() time.Time { return now }

	if !breaker.Allow() {
		t.Fatal("closed breaker rejected request")
	}
	breaker.Record(false)
	if breaker.State() != CircuitClosed {
		t.Fatalf("state = %s, want closed", breaker.State())
	}
	if !breaker.Allow() {
		t.Fatal("closed breaker rejected second request")
	}
	breaker.Record(false)
	if breaker.State() != CircuitOpen {
		t.Fatalf("state = %s, want open", breaker.State())
	}
	if breaker.Allow() {
		t.Fatal("open breaker allowed request before timeout")
	}

	now = now.Add(time.Minute)
	if breaker.State() != CircuitHalfOpen {
		t.Fatalf("state = %s, want half_open", breaker.State())
	}
	if !breaker.Allow() {
		t.Fatal("half-open breaker rejected probe")
	}
	if breaker.Allow() {
		t.Fatal("half-open breaker allowed concurrent probe")
	}
	breaker.Record(true)
	if breaker.State() != CircuitClosed {
		t.Fatalf("state = %s, want closed", breaker.State())
	}
}
