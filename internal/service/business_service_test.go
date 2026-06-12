package service

import (
	"context"
	"errors"
	"testing"
)

func TestCreateAfterSalesRequiresConfirmationAndIdempotencyKey(t *testing.T) {
	service := NewBusinessService(nil)
	base := CreateAfterSalesRequest{
		UserID:      "user-1",
		OrderNo:     "CC20260603001",
		Description: "无法充电",
	}
	if _, err := service.CreateAfterSales(context.Background(), base); !errors.Is(err, ErrInvalidBusinessRequest) {
		t.Fatalf("missing confirmation/key error = %v", err)
	}
	base.Confirmed = true
	if _, err := service.CreateAfterSales(context.Background(), base); !errors.Is(err, ErrInvalidBusinessRequest) {
		t.Fatalf("missing key error = %v", err)
	}
}
