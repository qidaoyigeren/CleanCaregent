package api

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"CleanCaregent/internal/ingest"
	"CleanCaregent/internal/rag"
	"CleanCaregent/internal/repository"
	"CleanCaregent/internal/service"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type KnowledgeHandler struct {
	service   *service.KnowledgeService
	retriever rag.Retriever
	publisher ingest.Publisher
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
	ContentFormat string         `json:"content_format"`
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
	options ...KnowledgeHandlerOption,
) *KnowledgeHandler {
	handler := &KnowledgeHandler{
		service:   service,
		retriever: retriever,
	}
	handler.ragConfig.DenseTopK = denseTopK
	handler.ragConfig.KeywordTopK = keywordTopK
	handler.ragConfig.RerankTopK = rerankTopK
	handler.ragConfig.MinDenseScore = minDenseScore
	for _, option := range options {
		option(handler)
	}
	return handler
}

type KnowledgeHandlerOption func(*KnowledgeHandler)

func WithKnowledgeIngestPublisher(publisher ingest.Publisher) KnowledgeHandlerOption {
	return func(handler *KnowledgeHandler) {
		handler.publisher = publisher
	}
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
	content, err := ingest.NormalizeContent(request.ContentFormat, request.Content)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_CONTENT_FORMAT", err.Error())
		return
	}
	request.Content = content
	if request.Metadata == nil {
		request.Metadata = map[string]any{}
	}
	if request.ContentFormat == "" {
		request.ContentFormat = "markdown"
	}
	request.Metadata["content_format"] = request.ContentFormat
	ingestRequest := service.IngestDocumentRequest{
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
	}
	async := false
	if raw := c.Query("async"); raw != "" {
		async, err = strconv.ParseBool(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "async must be boolean")
			return
		}
	}
	if async {
		if h.publisher == nil {
			response.Error(c, http.StatusServiceUnavailable, "INGEST_QUEUE_UNAVAILABLE", "knowledge ingest queue is not configured")
			return
		}
		queued, queueErr := h.publisher.Enqueue(c.Request.Context(), ingestRequest)
		if queueErr != nil {
			response.Error(c, http.StatusInternalServerError, "KNOWLEDGE_ENQUEUE_FAILED", "knowledge document enqueue failed")
			return
		}
		response.Accepted(c, queued)
		return
	}
	result, err := h.service.Ingest(c.Request.Context(), ingestRequest)
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

// Upload parses a multipart document and sends its extracted text through the normal ingest pipeline.
func (h *KnowledgeHandler) Upload(c *gin.Context) {
	if h.service == nil {
		response.Error(c, http.StatusServiceUnavailable, "KNOWLEDGE_UNAVAILABLE", "知识库服务未配置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_FILE", "请上传 file 字段")
		return
	}
	raw, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_FILE", "读取上传文件失败")
		return
	}
	if closeErr != nil {
		response.Error(c, http.StatusBadRequest, "INVALID_FILE", "关闭上传文件失败")
		return
	}
	format := strings.TrimSpace(c.PostForm("format"))
	if format == "" {
		format = ingest.FormatFromFilename(header.Filename)
	}
	content, err := ingest.ParseDocument(bytes.NewReader(raw), format)
	if err != nil {
		response.Error(c, http.StatusBadRequest, "DOCUMENT_PARSE_FAILED", err.Error())
		return
	}
	request := service.IngestDocumentRequest{
		DocID:    strings.TrimSpace(c.PostForm("doc_id")),
		Title:    strings.TrimSpace(c.PostForm("title")),
		Content:  content,
		Category: strings.TrimSpace(c.PostForm("category")),
		Brand:    strings.TrimSpace(c.PostForm("brand")),
		DocType:  strings.TrimSpace(c.PostForm("doc_type")),
		Version:  strings.TrimSpace(c.PostForm("version")),
		Source:   "upload://" + filepath.Base(header.Filename),
		Metadata: map[string]any{
			"content_format": format,
			"original_name":  filepath.Base(header.Filename),
		},
	}
	result, err := h.service.Ingest(c.Request.Context(), request)
	if err != nil {
		if errors.Is(err, service.ErrInvalidKnowledgeDocument) {
			response.Error(c, http.StatusBadRequest, "INVALID_KNOWLEDGE_DOCUMENT", err.Error())
			return
		}
		response.Error(c, http.StatusInternalServerError, "KNOWLEDGE_INGEST_FAILED", "文档入库失败")
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
