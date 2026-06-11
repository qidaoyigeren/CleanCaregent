package api

import (
	"errors"
	"net/http"
	"strconv"

	"CleanCaregent/internal/middleware"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/service"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type BusinessHandler struct {
	service *service.BusinessService
}

type createAfterSalesRequest struct {
	OrderNo          string   `json:"order_no" binding:"required"`
	OrderItemID      int64    `json:"order_item_id"`
	IssueType        string   `json:"issue_type"`
	Description      string   `json:"description" binding:"required"`
	DiagnosisSummary string   `json:"diagnosis_summary"`
	EvidenceIDs      []string `json:"evidence_ids"`
	IdempotencyKey   string   `json:"idempotency_key"`
}

func NewBusinessHandler(service *service.BusinessService) *BusinessHandler {
	return &BusinessHandler{service: service}
}

func (h *BusinessHandler) ListProducts(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "BUSINESS_UNAVAILABLE", "business service is not configured")
		return
	}
	limit := 20
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	items, err := h.service.ListProducts(c.Request.Context(), c.Query("category"), limit)
	if err != nil {
		writeBusinessError(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

func (h *BusinessHandler) GetProduct(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "BUSINESS_UNAVAILABLE", "business service is not configured")
		return
	}
	item, err := h.service.GetProduct(c.Request.Context(), c.Param("product_code"))
	if err != nil {
		writeBusinessError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *BusinessHandler) GetOrder(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "BUSINESS_UNAVAILABLE", "business service is not configured")
		return
	}
	item, err := h.service.GetOrder(c.Request.Context(), middleware.UserID(c), c.Param("order_no"))
	if err != nil {
		writeBusinessError(c, err)
		return
	}
	response.OK(c, item)
}

func (h *BusinessHandler) CreateAfterSales(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "BUSINESS_UNAVAILABLE", "business service is not configured")
		return
	}
	var request createAfterSalesRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "order_no and description are required")
		return
	}
	item, err := h.service.CreateAfterSales(c.Request.Context(), service.CreateAfterSalesRequest{
		UserID:           middleware.UserID(c),
		OrderNo:          request.OrderNo,
		OrderItemID:      request.OrderItemID,
		IssueType:        request.IssueType,
		Description:      request.Description,
		DiagnosisSummary: request.DiagnosisSummary,
		EvidenceIDs:      request.EvidenceIDs,
		IdempotencyKey:   request.IdempotencyKey,
	})
	if err != nil {
		writeBusinessError(c, err)
		return
	}
	response.Created(c, item)
}

func writeBusinessError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidBusinessRequest):
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", err.Error())
	case errors.Is(err, repository.ErrProductNotFound):
		response.Error(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "product not found")
	case errors.Is(err, repository.ErrOrderNotFound), errors.Is(err, repository.ErrOrderItemNotFound):
		response.Error(c, http.StatusNotFound, "ORDER_NOT_FOUND", "order or order item not found")
	case errors.Is(err, repository.ErrTicketConflict):
		response.Error(c, http.StatusConflict, "TICKET_ALREADY_EXISTS", "after-sales ticket already exists")
	default:
		response.Error(c, http.StatusInternalServerError, "BUSINESS_OPERATION_FAILED", "business operation failed")
	}
}
