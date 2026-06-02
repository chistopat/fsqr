package httpapi

import (
	"context"
	"errors"
	"fmt"
	stdhttp "net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const defaultMetricsPath = "/metrics"

type httpMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

func newHTTPMetrics(registry prometheus.Registerer) *httpMetrics {
	return &httpMetrics{
		requests: registerCounterVec(registry, prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "fsqr",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests.",
			},
			[]string{"method", "route", "status"},
		)),
		duration: registerHistogramVec(registry, prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "fsqr",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "route", "status"},
		)),
	}
}

func traceRequests() fiber.Handler {
	tracer := otel.Tracer("github.com/chistopat/fsqr/internal/http")

	return func(ctx *fiber.Ctx) error {
		requestContext := extractTraceContext(ctx.UserContext(), ctx)
		requestContext, span := tracer.Start(
			requestContext,
			ctx.Method()+" "+ctx.Path(),
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", ctx.Method()),
				attribute.String("url.path", ctx.Path()),
				attribute.String("client.address", ctx.IP()),
			),
		)
		ctx.SetUserContext(requestContext)

		err := ctx.Next()
		status := responseStatus(ctx, err)
		route := routeLabel(ctx)

		span.SetName(ctx.Method() + " " + route)
		span.SetAttributes(
			attribute.String("http.route", route),
			attribute.Int("http.response.status_code", status),
		)
		if err != nil {
			span.RecordError(err)
		}
		if status >= fiber.StatusInternalServerError {
			span.SetStatus(codes.Error, statusDescription(status, err))
		}
		span.End()

		return err
	}
}

func recordHTTPMetrics(metrics *httpMetrics) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		started := time.Now()
		err := ctx.Next()

		status := responseStatus(ctx, err)
		route := routeLabel(ctx)
		metrics.requests.WithLabelValues(ctx.Method(), route, strconv.Itoa(status)).Inc()
		metrics.duration.WithLabelValues(ctx.Method(), route, strconv.Itoa(status)).
			Observe(time.Since(started).Seconds())

		return err
	}
}

func metricsHandler(registry *prometheus.Registry) fiber.Handler {
	return adaptor.HTTPHandler(promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))
}

func newMetricsRegistry(registry *prometheus.Registry) *prometheus.Registry {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

	registerCollector(registry, collectors.NewGoCollector())
	registerCollector(registry, collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	return registry
}

func normalizeMetricsPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return defaultMetricsPath
	}
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}

	return path
}

func extractTraceContext(ctx context.Context, fiberCtx *fiber.Ctx) context.Context {
	headers := stdhttp.Header{}
	fiberCtx.Request().Header.VisitAll(func(key []byte, value []byte) {
		headers.Add(string(key), string(value))
	})

	return otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(headers))
}

func responseStatus(ctx *fiber.Ctx, err error) int {
	status := ctx.Response().StatusCode()
	if err == nil {
		return status
	}

	var fiberError *fiber.Error
	if errors.As(err, &fiberError) {
		return fiberError.Code
	}
	if status >= fiber.StatusBadRequest {
		return status
	}

	return fiber.StatusInternalServerError
}

func routeLabel(ctx *fiber.Ctx) string {
	route := ctx.Route()
	if route != nil && route.Path != "" {
		return route.Path
	}

	return "unmatched"
}

func statusDescription(status int, err error) string {
	if err != nil {
		return err.Error()
	}
	if text := stdhttp.StatusText(status); text != "" {
		return text
	}

	return strconv.Itoa(status)
}

func registerCollector(registry prometheus.Registerer, collector prometheus.Collector) {
	if err := registry.Register(collector); err != nil {
		if _, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
			return
		}

		panic(fmt.Sprintf("register prometheus collector: %v", err))
	}
}

func registerCounterVec(
	registry prometheus.Registerer,
	collector *prometheus.CounterVec,
) *prometheus.CounterVec {
	if err := registry.Register(collector); err != nil {
		if alreadyRegistered, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
			if existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.CounterVec); ok {
				return existing
			}
		}

		panic(fmt.Sprintf("register prometheus counter: %v", err))
	}

	return collector
}

func registerHistogramVec(
	registry prometheus.Registerer,
	collector *prometheus.HistogramVec,
) *prometheus.HistogramVec {
	if err := registry.Register(collector); err != nil {
		if alreadyRegistered, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
			if existing, ok := alreadyRegistered.ExistingCollector.(*prometheus.HistogramVec); ok {
				return existing
			}
		}

		panic(fmt.Sprintf("register prometheus histogram: %v", err))
	}

	return collector
}
