package api

import (
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/service"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type KnowledgeHandler struct {
	service   *service.KnowledgeService
	retriever rag.Retriever
	ragConfig struct {
		DenseTopK     int
		KeywordTopK   int
		RerankTopK    int
		MinDenseScore float64
	}
}

type ingestKnowledgeRequest struct {
	DocID         string         `json:"doc_id" binding:"required"`
	Title         string         `json:"title" binding:"required"`
	Content       string         `json:"content" binding:"required"`
	Category      string         `json:"category" binding:"required"`
	Brand         string         `json:"brand"`
	DocType       string         `json:"doc_type" binding:"required"`
	Version       string         `json:"version"`
	EffectiveTime *time.Time     `json:"effective_time"`
	ExpireTime    *time.Time     `json:"expire_time"`
	Source        string         `json:"source"`
	IntentTags    []string       `json:"intent_tags"`
	Metadata      map[string]any `json:"metadata"`
}

type searchKnowledgeRequest struct {
	Query      string   `json:"query" binding:"required"`
	Categories []string `json:"categories"`
	Brands     []string `json:"brands"`
	Models     []string `json:"models"`
	DocTypes   []string `json:"doc_types"`
	TopK       int      `json:"top_k"`
}

func NewKnowledgeHandler(
	service *service.KnowledgeService,
	retriever rag.Retriever,
	denseTopK, keywordTopK, rerankTopK int,
	minDenseScore float64,
) *KnowledgeHandler {
	handler := &KnowledgeHandler{
		service:   service,
		retriever: retriever,
	}
	handler.ragConfig.DenseTopK = denseTopK
	handler.ragConfig.KeywordTopK = keywordTopK
	handler.ragConfig.RerankTopK = rerankTopK
	handler.ragConfig.MinDenseScore = minDenseScore
	return handler
}

func (h *KnowledgeHandler) Ingest(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "knowledge service is not configured")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
	var request ingestKnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid knowledge document")
		return
	}
	result, err := h.service.Ingest(c.Request.Context(), service.IngestDocumentRequest{
		DocID:         request.DocID,
		Title:         request.Title,
		Content:       request.Content,
		Category:      request.Category,
		Brand:         request.Brand,
		DocType:       request.DocType,
		Version:       request.Version,
		EffectiveTime: request.EffectiveTime,
		ExpireTime:    request.ExpireTime,
		Source:        request.Source,
		IntentTags:    request.IntentTags,
		Metadata:      request.Metadata,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidKnowledgeDocument):
			response.Error(c, http.StatusBadRequest, "INVALID_KNOWLEDGE_DOCUMENT", err.Error())
		case errors.Is(err, repository.ErrKnowledgeDocumentConflict):
			response.Error(c, http.StatusConflict, "DOCUMENT_VERSION_EXISTS", "document version already exists")
		case errors.Is(err, service.ErrKnowledgeUnavailable):
			response.Error(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", err.Error())
		default:
			response.Error(c, http.StatusInternalServerError, "KNOWLEDGE_INGEST_FAILED", "knowledge document ingestion failed")
		}
		return
	}
	response.Created(c, result)
}

func (h *KnowledgeHandler) Search(c *gin.Context) {
	if h.retriever == nil {
		response.Error(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "knowledge retriever is not configured")
		return
	}
	var request searchKnowledgeRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "query is required")
		return
	}
	if utf8.RuneCountInString(request.Query) > 500 {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "query is too long")
		return
	}
	topK := request.TopK
	if topK <= 0 {
		topK = h.ragConfig.RerankTopK
	}
	if topK > 20 {
		topK = 20
	}
	results, err := h.retriever.Search(c.Request.Context(), rag.SearchRequest{
		Query: request.Query,
		Mode:  rag.SearchHybrid,
		Filter: rag.MetadataFilter{
			Categories: request.Categories,
			Brands:     request.Brands,
			Models:     request.Models,
			DocTypes:   request.DocTypes,
		},
		DenseTopK:   h.ragConfig.DenseTopK,
		KeywordTopK: h.ragConfig.KeywordTopK,
		RerankTopK:  topK,
		MinScore:    h.ragConfig.MinDenseScore,
		NeedRerank:  true,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "KNOWLEDGE_SEARCH_FAILED", "knowledge search failed")
		return
	}
	response.OK(c, gin.H{"items": results})
}
