package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const defaultMetricsPath = "/metrics"

type CaptureStatsProvider interface {
	CaptureStats(ctx context.Context) (capturemodel.Stats, error)
}

type Metrics struct {
	Registry *prometheus.Registry
	HTTP     *HTTPMetrics
}

type HTTPMetrics struct {
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
}

type captureStatsCollector struct {
	provider       CaptureStatsProvider
	log            *zap.Logger
	countDesc      *prometheus.Desc
	sizeBytesDesc  *prometheus.Desc
	collectTimeout time.Duration
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func NewMetrics(provider CaptureStatsProvider, log *zap.Logger) *Metrics {
	registry := prometheus.NewRegistry()
	registerCollector(registry, collectors.NewGoCollector())
	registerCollector(registry, collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	metrics := &Metrics{
		Registry: registry,
		HTTP:     newHTTPMetrics(registry),
	}
	if provider != nil {
		registerCollector(registry, newCaptureStatsCollector(provider, log))
	}

	return metrics
}

func NewMetricsHandler(registry *prometheus.Registry, metricsPath string) http.Handler {
	mux := http.NewServeMux()
	mux.Handle(normalizeMetricsPath(metricsPath), promhttp.HandlerFor(newMetricsRegistry(registry), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	}))

	return mux
}

func newHTTPMetrics(registry prometheus.Registerer) *HTTPMetrics {
	return &HTTPMetrics{
		requests: registerCounterVec(registry, prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "hoppify",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total number of HTTP requests.",
			},
			[]string{"method", "route", "status"},
		)),
		duration: registerHistogramVec(registry, prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "hoppify",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request duration in seconds.",
				Buckets:   []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
			},
			[]string{"method", "route", "status"},
		)),
	}
}

func observeHTTP(metrics *HTTPMetrics, method, route string, status int, duration time.Duration) {
	if metrics == nil {
		return
	}

	statusLabel := strconv.Itoa(status)
	metrics.requests.WithLabelValues(method, route, statusLabel).Inc()
	metrics.duration.WithLabelValues(method, route, statusLabel).Observe(duration.Seconds())
}

func logAndMeasureRequests(next http.Handler, log *zap.Logger, metrics *HTTPMetrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(recorder, r)

		route := routeLabel(r)
		duration := time.Since(started)
		observeHTTP(metrics, r.Method, route, recorder.status, duration)
		loggerOrNop(log).Info(
			"http request completed",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("route", route),
			zap.Int("status", recorder.status),
			zap.Duration("duration", duration),
		)
	})
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func routeLabel(r *http.Request) string {
	if r.Pattern != "" {
		return r.Pattern
	}

	return "unmatched"
}

func newCaptureStatsCollector(provider CaptureStatsProvider, log *zap.Logger) *captureStatsCollector {
	return &captureStatsCollector{
		provider: provider,
		log:      loggerOrNop(log),
		countDesc: prometheus.NewDesc(
			"hoppify_captures_images_total",
			"Current number of image captures stored in Postgres.",
			nil,
			nil,
		),
		sizeBytesDesc: prometheus.NewDesc(
			"hoppify_captures_image_size_bytes_total",
			"Current total JPEG bytes for image captures stored in Postgres.",
			nil,
			nil,
		),
		collectTimeout: 2 * time.Second,
	}
}

func (collector *captureStatsCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- collector.countDesc
	ch <- collector.sizeBytesDesc
}

func (collector *captureStatsCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), collector.collectTimeout)
	defer cancel()

	stats, err := collector.provider.CaptureStats(ctx)
	if err != nil {
		collector.log.Warn("collect capture stats failed", zap.Error(err))
		ch <- prometheus.NewInvalidMetric(collector.countDesc, err)
		ch <- prometheus.NewInvalidMetric(collector.sizeBytesDesc, err)
		return
	}

	ch <- prometheus.MustNewConstMetric(collector.countDesc, prometheus.GaugeValue, float64(stats.ImageCount))
	ch <- prometheus.MustNewConstMetric(
		collector.sizeBytesDesc,
		prometheus.GaugeValue,
		float64(stats.ImageSizeBytesTotal),
	)
}

func newMetricsRegistry(registry *prometheus.Registry) *prometheus.Registry {
	if registry == nil {
		registry = prometheus.NewRegistry()
	}

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

func registerCollector(registry prometheus.Registerer, collector prometheus.Collector) {
	if err := registry.Register(collector); err != nil {
		if _, ok := errors.AsType[prometheus.AlreadyRegisteredError](err); ok {
			return
		}

		panic(fmt.Sprintf("register prometheus collector: %v", err))
	}
}

func registerCounterVec(registry prometheus.Registerer, collector *prometheus.CounterVec) *prometheus.CounterVec {
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

func registerHistogramVec(registry prometheus.Registerer, collector *prometheus.HistogramVec) *prometheus.HistogramVec {
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
