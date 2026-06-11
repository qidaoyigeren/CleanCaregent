package middleware

import (
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/gin-gonic/gin"
)

func AccessLog(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		startedAt := time.Now()
		c.Next()

		requestID, _ := c.Get("request_id")
		spanContext := trace.SpanContextFromContext(c.Request.Context())
		logger.Info("http request",
			zap.String("request_id", stringValue(requestID)),
			zap.String("method", c.Request.Method),
			zap.String("path", c.FullPath()),
			zap.Int("status", c.Writer.Status()),
			zap.Int("response_bytes", c.Writer.Size()),
			zap.Duration("latency", time.Since(startedAt)),
			zap.String("client_ip", c.ClientIP()),
			zap.String("otel_trace_id", spanContext.TraceID().String()),
			zap.String("otel_span_id", spanContext.SpanID().String()),
		)
	}
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}
