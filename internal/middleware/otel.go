package middleware

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
)

func OpenTelemetry(serviceName string) gin.HandlerFunc {
	tracer := otel.Tracer(serviceName + "/http")
	return func(c *gin.Context) {
		ctx := otel.GetTextMapPropagator().Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := tracer.Start(ctx, c.Request.Method+" "+c.Request.URL.Path)
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		span.SetAttributes(
			attribute.String("http.request.method", c.Request.Method),
			attribute.String("http.route", c.FullPath()),
			attribute.Int("http.response.status_code", c.Writer.Status()),
			attribute.String("request.id", stringValueFromContext(c, "request_id")),
		)
		if c.Writer.Status() >= 500 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", c.Writer.Status()))
		}
	}
}

func stringValueFromContext(c *gin.Context, key string) string {
	value, _ := c.Get(key)
	text, _ := value.(string)
	return text
}
