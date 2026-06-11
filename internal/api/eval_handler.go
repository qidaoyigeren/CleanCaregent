package api

import (
	"errors"
	"net/http"
	"strconv"

	"CleanCaregent/internal/eval"
	evalmysql "CleanCaregent/internal/eval/mysql"
	"CleanCaregent/internal/middleware"
	"CleanCaregent/pkg/response"

	"github.com/gin-gonic/gin"
)

type EvalHandler struct {
	runner *eval.Runner
	store  eval.Store
}

type runEvalRequest struct {
	DatasetVersion string `json:"dataset_version"`
	SystemVersion  string `json:"system_version"`
	MaxCases       int    `json:"max_cases"`
}

func NewEvalHandler(runner *eval.Runner, store eval.Store) *EvalHandler {
	return &EvalHandler{runner: runner, store: store}
}

func (h *EvalHandler) Run(c *gin.Context) {
	if h.runner == nil {
		response.Error(c, http.StatusServiceUnavailable, "EVAL_UNAVAILABLE", "eval runner is not configured")
		return
	}
	var request runEvalRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&request); err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "invalid eval request")
			return
		}
	}
	if request.MaxCases < 0 || request.MaxCases > 100 {
		response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "max_cases must be between 0 and 100")
		return
	}
	run, err := h.runner.Start(c.Request.Context(), eval.RunRequest{
		UserID:         middleware.UserID(c),
		DatasetVersion: request.DatasetVersion,
		SystemVersion:  request.SystemVersion,
		MaxCases:       request.MaxCases,
	})
	if err != nil {
		response.Error(c, http.StatusInternalServerError, "EVAL_RUN_FAILED", err.Error())
		return
	}
	response.Accepted(c, run)
}

func (h *EvalHandler) Get(c *gin.Context) {
	if h.store == nil {
		response.Error(c, http.StatusServiceUnavailable, "EVAL_UNAVAILABLE", "eval store is not configured")
		return
	}
	includeFailures := false
	if raw := c.Query("include_failures"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			response.Error(c, http.StatusBadRequest, "INVALID_ARGUMENT", "include_failures must be boolean")
			return
		}
		includeFailures = parsed
	}
	run, err := h.store.GetRun(c.Request.Context(), c.Param("run_no"), includeFailures)
	if err != nil {
		if errors.Is(err, evalmysql.ErrRunNotFound) {
			response.Error(c, http.StatusNotFound, "EVAL_RUN_NOT_FOUND", "eval run not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, "EVAL_QUERY_FAILED", "eval run query failed")
		return
	}
	response.OK(c, run)
}
