package observability

import (
	"context"
	"fmt"

	"github.com/chistopat/fsqr/internal/config"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

func InitTracing(
	ctx context.Context,
	serviceName string,
	environment string,
	cfg config.TracingConfig,
) (func(context.Context) error, error) {
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	if !cfg.Enabled {
		return func(context.Context) error {
			return nil
		}, nil
	}

	options := []otlptracegrpc.Option{}
	if cfg.OTLPEndpointURL != "" {
		options = append(options, otlptracegrpc.WithEndpointURL(cfg.OTLPEndpointURL))
	}
	if cfg.OTLPInsecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}

	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("init otlp trace exporter: %w", err)
	}

	traceResource, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			"",
			attribute.String("service.name", serviceName),
			attribute.String("deployment.environment.name", environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("init trace resource: %w", err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(traceResource),
	)
	otel.SetTracerProvider(provider)

	return provider.Shutdown, nil
}

func RecordSpanError(span trace.Span, err error) {
	if err == nil {
		return
	}

	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
