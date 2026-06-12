package builtin

import (
	"context"
	"errors"
	"testing"

	"CleanCaregent/internal/tool"
)

func TestCreateAfterSalesTicketRequiresIdempotencyKey(t *testing.T) {
	value := NewCreateAfterSalesTicket(nil)
	_, err := value.Execute(context.Background(), tool.Call{
		TraceID: "trace-1",
		UserID:  "user-1",
		Arguments: map[string]any{
			"confirmed":   true,
			"order_no":    "CC20260603001",
			"description": "无法充电",
		},
	})
	if !errors.Is(err, ErrInvalidArguments) {
		t.Fatalf("Execute() error = %v", err)
	}
}
