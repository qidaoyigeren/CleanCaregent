package api

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

type SSEWriter struct {
	context *gin.Context
}

func NewSSEWriter(c *gin.Context) *SSEWriter {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	return &SSEWriter{context: c}
}

func (w *SSEWriter) Send(event string, data any) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal sse payload: %w", err)
	}
	if _, err := fmt.Fprintf(w.context.Writer, "event: %s\ndata: %s\n\n", event, payload); err != nil {
		return fmt.Errorf("write sse event: %w", err)
	}
	w.context.Writer.Flush()
	return nil
}
