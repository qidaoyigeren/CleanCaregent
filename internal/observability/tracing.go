package observability

import (
	"context"
	"strings"

	"CleanCaregent/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

type Shutdown func(context.Context) error

func Init(ctx context.Context, cfg config.TracingConfig) (Shutdown, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	if !cfg.Enabled {
		return func(context.Context) error { return nil }, nil
	}

	options := []sdktrace.TracerProviderOption{
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.SampleRatio))),
		sdktrace.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			attribute.String("deployment.environment", "local"),
		)),
	}
	if strings.TrimSpace(cfg.OTLPEndpoint) != "" {
		exporterOptions := []otlptracehttp.Option{}
		if strings.HasPrefix(cfg.OTLPEndpoint, "http://") || strings.HasPrefix(cfg.OTLPEndpoint, "https://") {
			exporterOptions = append(exporterOptions, otlptracehttp.WithEndpointURL(cfg.OTLPEndpoint))
		} else {
			exporterOptions = append(exporterOptions, otlptracehttp.WithEndpoint(cfg.OTLPEndpoint))
		}
		if cfg.Insecure {
			exporterOptions = append(exporterOptions, otlptracehttp.WithInsecure())
		}
		exporter, err := otlptracehttp.New(ctx, exporterOptions...)
		if err != nil {
			return nil, err
		}
		options = append(options, sdktrace.WithBatcher(exporter))
	}
	provider := sdktrace.NewTracerProvider(options...)
	otel.SetTracerProvider(provider)
	return provider.Shutdown, nil
}
