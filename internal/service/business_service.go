package service

import (
	"context"
	"errors"
	"strings"

	"CleanCaregent/internal/model"
	"CleanCaregent/internal/platform/id"
	"CleanCaregent/internal/repository"
)

var ErrInvalidBusinessRequest = errors.New("invalid business request")

type BusinessService struct {
	repository repository.BusinessRepository
}

type CreateAfterSalesRequest struct {
	UserID           string
	OrderNo          string
	OrderItemID      int64
	IssueType        string
	Description      string
	DiagnosisSummary string
	EvidenceIDs      []string
	IdempotencyKey   string
}

func NewBusinessService(repository repository.BusinessRepository) *BusinessService {
	return &BusinessService{repository: repository}
}

func (s *BusinessService) ListProducts(ctx context.Context, category string, limit int) ([]model.Product, error) {
	return s.repository.ListProducts(ctx, strings.TrimSpace(category), limit)
}

func (s *BusinessService) GetProduct(ctx context.Context, productCode string) (model.Product, error) {
	productCode = strings.TrimSpace(productCode)
	if productCode == "" {
		return model.Product{}, ErrInvalidBusinessRequest
	}
	return s.repository.GetProduct(ctx, productCode)
}

func (s *BusinessService) GetOrder(ctx context.Context, userID, orderNo string) (model.OrderDetail, error) {
	orderNo = strings.ToUpper(strings.TrimSpace(orderNo))
	if userID == "" || orderNo == "" {
		return model.OrderDetail{}, ErrInvalidBusinessRequest
	}
	return s.repository.GetOrder(ctx, userID, orderNo)
}

func (s *BusinessService) CreateAfterSales(
	ctx context.Context,
	request CreateAfterSalesRequest,
) (model.AfterSalesTicket, error) {
	request.OrderNo = strings.ToUpper(strings.TrimSpace(request.OrderNo))
	request.Description = strings.TrimSpace(request.Description)
	if request.UserID == "" || request.OrderNo == "" || request.Description == "" {
		return model.AfterSalesTicket{}, ErrInvalidBusinessRequest
	}
	if request.IssueType == "" {
		request.IssueType = "repair"
	}
	if request.IdempotencyKey == "" {
		request.IdempotencyKey = id.New("idem")
	}
	return s.repository.CreateAfterSalesTicket(ctx, repository.CreateTicketRequest{
		UserID:           request.UserID,
		OrderNo:          request.OrderNo,
		OrderItemID:      request.OrderItemID,
		IssueType:        request.IssueType,
		Description:      request.Description,
		DiagnosisSummary: request.DiagnosisSummary,
		EvidenceIDs:      request.EvidenceIDs,
		IdempotencyKey:   request.IdempotencyKey,
	})
}
