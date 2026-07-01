package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"CleanCaregent/internal/agent"
	"CleanCaregent/internal/repository/inmemory"
)

func TestConversationServiceDeduplicatesInFlightClientMessageID(t *testing.T) {
	repo := inmemory.NewConversationRepository()
	runner := &blockingRunner{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	service := NewConversationService(repo, runner, time.Second)
	service.requestPoll = 5 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conversation, err := service.Create(ctx, "user-1", "support")
	if err != nil {
		t.Fatal(err)
	}

	outcomes := make(chan askOutcome, 2)

	go func() {
		result, err := service.Ask(ctx, "user-1", conversation.ID, "create repair ticket", "cm-1", nil)
		outcomes <- askOutcome{result: result, err: err}
	}()

	select {
	case <-runner.started:
	case <-ctx.Done():
		t.Fatal("runner did not start")
	}

	go func() {
		result, err := service.Ask(ctx, "user-1", conversation.ID, "create repair ticket", "cm-1", nil)
		outcomes <- askOutcome{result: result, err: err}
	}()

	time.Sleep(20 * time.Millisecond)
	close(runner.release)

	first := receiveAskOutcome(t, ctx, outcomes)
	second := receiveAskOutcome(t, ctx, outcomes)
	for _, outcome := range []askOutcome{first, second} {
		if outcome.err != nil {
			t.Fatalf("Ask() error = %v", outcome.err)
		}
		if outcome.result.Result.Answer != "single execution" {
			t.Fatalf("answer = %q", outcome.result.Result.Answer)
		}
	}
	if calls := runner.calls.Load(); calls != 1 {
		t.Fatalf("runner calls = %d, want 1", calls)
	}
	if !isIdempotentReplayForTest(first.result.Result.Mode) && !isIdempotentReplayForTest(second.result.Result.Mode) {
		t.Fatalf("one duplicate request should replay, got modes %q and %q",
			first.result.Result.Mode,
			second.result.Result.Mode,
		)
	}
}

func isIdempotentReplayForTest(mode string) bool {
	return mode == "idempotent_wait" || mode == "idempotent_replay"
}

func receiveAskOutcome(
	t *testing.T,
	ctx context.Context,
	outcomes <-chan askOutcome,
) askOutcome {
	t.Helper()
	select {
	case outcome := <-outcomes:
		return outcome
	case <-ctx.Done():
		t.Fatal("timed out waiting for Ask result")
		return askOutcome{}
	}
}

type askOutcome struct {
	result AskResult
	err    error
}

type blockingRunner struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (r *blockingRunner) Run(
	ctx context.Context,
	_ agent.Request,
	_ agent.EventSink,
) (agent.Result, error) {
	r.calls.Add(1)
	r.once.Do(func() {
		close(r.started)
	})
	select {
	case <-ctx.Done():
		return agent.Result{}, ctx.Err()
	case <-r.release:
		return agent.Result{Answer: "single execution", Mode: "test"}, nil
	}
}
