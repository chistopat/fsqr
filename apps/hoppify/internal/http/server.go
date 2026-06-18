package httpapi

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"go.uber.org/zap"
)

//go:embed web/index.html web/assets/*
var webFiles embed.FS

type liveResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type HandlerOption func(*handlerConfig)

type handlerConfig struct {
	captureService       CaptureCreator
	captureLister        CaptureLister
	captureImageProvider CaptureImageProvider
	detectService        DetectorService
	cropService          CropCreator
	beerLabelService     BeerLabelIdentifier
	limits               capturemodel.Limits
	log                  *zap.Logger
	metrics              *HTTPMetrics
}

func WithCaptureService(service CaptureCreator) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.captureService = service
		if lister, ok := service.(CaptureLister); ok {
			cfg.captureLister = lister
		}
		if provider, ok := service.(CaptureImageProvider); ok {
			cfg.captureImageProvider = provider
		}
	}
}

func WithDetectService(service DetectorService) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.detectService = service
	}
}

func WithCropService(service CropCreator) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.cropService = service
	}
}

func WithBeerLabelService(service BeerLabelIdentifier) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.beerLabelService = service
	}
}

func WithCaptureLimits(limits capturemodel.Limits) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.limits = limits
	}
}

func WithLogger(log *zap.Logger) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.log = log
	}
}

func WithHTTPMetrics(metrics *HTTPMetrics) HandlerOption {
	return func(cfg *handlerConfig) {
		cfg.metrics = metrics
	}
}

func NewHandler(options ...HandlerOption) http.Handler {
	cfg := handlerConfig{limits: normalizeHTTPLimits(capturemodel.Limits{})}
	for _, option := range options {
		option(&cfg)
	}

	mux := http.NewServeMux()
	webRoot := mustSub(webFiles, "web")

	mux.Handle("GET /assets/", http.FileServer(http.FS(webRoot)))
	mux.HandleFunc("GET /", serveIndex(webRoot))
	mux.HandleFunc("GET /live", serveLive)
	mux.HandleFunc("GET /swagger.json", serveSwaggerJSON)
	mux.HandleFunc("GET /swagger", serveSwaggerViewer)
	mux.HandleFunc("GET /swagger/", serveSwaggerViewer)
	mux.HandleFunc("GET /api/v1/captures", listCaptures(cfg.captureLister, cfg.log))
	mux.HandleFunc("GET /api/v1/captures/{uuid}/image", serveCaptureImage(cfg.captureImageProvider, cfg.log))
	mux.HandleFunc("POST /api/v1/captures", createCaptures(cfg.captureService, cfg.limits, cfg.log))
	mux.HandleFunc("POST /api/v1/detect", detectObjects(cfg.detectService, cfg.limits, cfg.log))
	mux.HandleFunc("POST /api/v1/crops", createCrops(cfg.cropService, cfg.log))
	mux.HandleFunc("POST /api/v1/beer-labels/identify", identifyBeerLabel(cfg.beerLabelService, cfg.log))

	return logAndMeasureRequests(mux, cfg.log, cfg.metrics)
}

func serveIndex(webRoot fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeFileFS(w, r, webRoot, "index.html")
	}
}

func serveLive(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(liveResponse{Status: "ok", Service: "hoppify"}); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}

func mustSub(files fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(files, dir)
	if err != nil {
		panic(err)
	}

	return sub
}
