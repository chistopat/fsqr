package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chistopat/hoppify/internal/config"
	"github.com/chistopat/hoppify/internal/db"
	onnxdetector "github.com/chistopat/hoppify/internal/detector/onnx"
	httpapi "github.com/chistopat/hoppify/internal/http"
	"github.com/chistopat/hoppify/internal/logger"
	geminirecognizer "github.com/chistopat/hoppify/internal/recognizer/gemini"
	beerlabelrepo "github.com/chistopat/hoppify/internal/repository/beerlabels"
	capturerepo "github.com/chistopat/hoppify/internal/repository/captures"
	beerlabelservice "github.com/chistopat/hoppify/internal/service/beerlabels"
	captureservice "github.com/chistopat/hoppify/internal/service/captures"
	cropservice "github.com/chistopat/hoppify/internal/service/crops"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"
	"github.com/chistopat/hoppify/internal/storage"

	"go.uber.org/zap"
)

func main() {
	if err := run(); err != nil {
		log.Fatalf("run hoppify: %v", err)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	appLogger, err := logger.New(cfg.Logger)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	ctx := context.Background()
	database, err := db.OpenPostgres(ctx, cfg.Database)
	if err != nil {
		return fmt.Errorf("open postgres: %w", err)
	}
	defer func() {
		_ = database.Close()
	}()

	repository, err := capturerepo.New(database, appLogger.Named("captures.repository"))
	if err != nil {
		return fmt.Errorf("init captures repository: %w", err)
	}
	beerLabelRepository, err := beerlabelrepo.New(database, appLogger.Named("beer_labels.repository"))
	if err != nil {
		return fmt.Errorf("init beer labels repository: %w", err)
	}

	objectStorage, err := storage.NewS3Storage(ctx, storage.S3Config{
		Region:          cfg.S3.Region,
		EndpointURL:     cfg.S3.EndpointURL,
		AccessKeyID:     cfg.S3.AccessKeyID,
		SecretAccessKey: cfg.S3.SecretAccessKey,
		SessionToken:    cfg.S3.SessionToken,
		ForcePathStyle:  cfg.S3.ForcePathStyle,
	}, appLogger.Named("storage.s3"))
	if err != nil {
		return fmt.Errorf("init s3 storage: %w", err)
	}

	captureService, err := captureservice.NewService(repository, objectStorage, captureservice.Config{
		Bucket:      cfg.S3.Bucket,
		Limits:      cfg.Upload.Limits(),
		JPEGQuality: cfg.Upload.JPEGQuality,
	})
	if err != nil {
		return fmt.Errorf("init captures service: %w", err)
	}

	objectDetector, closeDetector := newDetectorBackend(cfg.Detector, appLogger.Named("detector.onnx"))
	defer closeDetector()

	detectService, err := detectservice.NewService(repository, objectStorage, objectDetector, detectservice.Config{
		MaxObjectBytes: cfg.Upload.Limits().MaxFileBytes,
	})
	if err != nil {
		return fmt.Errorf("init detect service: %w", err)
	}

	cropService, err := cropservice.NewService(repository, objectStorage, cropservice.Config{
		MaxObjectBytes: cfg.Upload.Limits().MaxFileBytes,
		JPEGQuality:    cfg.Upload.JPEGQuality,
		MaxBoxes:       cfg.Detector.MaxDetections,
	})
	if err != nil {
		return fmt.Errorf("init crops service: %w", err)
	}

	beerLabelService, err := newBeerLabelService(
		&cfg.BeerLabel,
		repository,
		beerLabelRepository,
		objectStorage,
		appLogger,
		cfg.Upload.Limits().MaxFileBytes,
	)
	if err != nil {
		return err
	}

	metrics := httpapi.NewMetrics(repository, appLogger.Named("metrics"))
	server := &http.Server{
		Addr: cfg.HTTP.Addr,
		Handler: httpapi.NewHandler(
			httpapi.WithCaptureService(captureService),
			httpapi.WithDetectService(detectService),
			httpapi.WithCropService(cropService),
			httpapi.WithBeerLabelService(beerLabelService),
			httpapi.WithCaptureLimits(cfg.Upload.Limits()),
			httpapi.WithLogger(appLogger.Named("http")),
			httpapi.WithHTTPMetrics(metrics.HTTP),
		),
		ReadHeaderTimeout: 5 * time.Second,
	}
	metricsServer := &http.Server{
		Addr:              cfg.Observability.Metrics.Addr,
		Handler:           httpapi.NewMetricsHandler(metrics.Registry, cfg.Observability.Metrics.Path),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return serveUntilStopped(appLogger, server, metricsServer, &cfg)
}

func newBeerLabelService(
	cfg *config.BeerLabelConfig,
	captures beerlabelservice.CaptureRepository,
	recognitions beerlabelservice.RecognitionRepository,
	objectStorage beerlabelservice.ObjectStorage,
	appLogger *zap.Logger,
	maxObjectBytes int64,
) (*beerlabelservice.Service, error) {
	beerLabelService, err := beerlabelservice.NewService(
		captures,
		recognitions,
		objectStorage,
		newBeerLabelGeminiRecognizer(cfg, appLogger.Named("beer_labels.gemini")),
		beerlabelservice.Config{MaxObjectBytes: maxObjectBytes},
	)
	if err != nil {
		return nil, fmt.Errorf("init beer labels service: %w", err)
	}

	return beerLabelService, nil
}

func newBeerLabelGeminiRecognizer(
	cfg *config.BeerLabelConfig,
	recognizerLog *zap.Logger,
) beerlabelservice.Recognizer {
	recognizer, err := geminirecognizer.NewClient(geminirecognizer.Config{
		APIKey:  cfg.GeminiAPIKey,
		Model:   cfg.GeminiModel,
		Timeout: cfg.GeminiTimeout,
	})
	if err != nil {
		recognizerLog.Warn("gemini beer label recognizer unavailable", zap.Error(err))
		return beerlabelservice.NewUnavailableRecognizer(cfg.GeminiModel, geminirecognizer.PromptVersionV3, err)
	}

	return recognizer
}

func newDetectorBackend(
	cfg config.DetectorConfig,
	detectorLog *zap.Logger,
) (detectorBackend detectservice.Detector, closeDetector func()) {
	detector, err := onnxdetector.NewDetector(onnxdetector.Config{
		ModelPath:           cfg.ModelPath,
		RuntimeLibraryPath:  cfg.RuntimeLibraryPath,
		ImageSize:           cfg.ImageSize,
		ConfidenceThreshold: cfg.ConfidenceThreshold,
		IOUThreshold:        cfg.IOUThreshold,
		MaxDetections:       cfg.MaxDetections,
	})
	if err != nil {
		detectorLog.Error("onnx detector unavailable", zap.Error(err))
		return detectservice.NewUnavailableDetector(err), func() {}
	}

	return detector, func() {
		if err := detector.Close(); err != nil {
			detectorLog.Error("close onnx detector failed", zap.Error(err))
		}
	}
}

func serveUntilStopped(appLog *zap.Logger, server, metricsServer *http.Server, cfg *config.Config) error {
	appLog.Info(
		"starting http server",
		zap.String("app", cfg.App.Name),
		zap.String("env", cfg.App.Env),
		zap.String("addr", cfg.HTTP.Addr),
	)
	appLog.Info(
		"starting metrics server",
		zap.String("addr", cfg.Observability.Metrics.Addr),
		zap.String("path", cfg.Observability.Metrics.Path),
	)

	serverErrors := make(chan error, 2)
	go listenHTTP(server, "http", serverErrors)
	go listenHTTP(metricsServer, "metrics", serverErrors)

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		appLog.Info("shutdown signal received")
	case err := <-serverErrors:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := shutdownHTTP(shutdownCtx, server, "http"); err != nil {
		return err
	}
	if err := shutdownHTTP(shutdownCtx, metricsServer, "metrics"); err != nil {
		return err
	}

	appLog.Info("shutdown complete")

	return nil
}

func listenHTTP(server *http.Server, name string, serverErrors chan<- error) {
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		serverErrors <- errors.Join(errors.New(name+" server"), err)
	}
}

func shutdownHTTP(ctx context.Context, server *http.Server, name string) error {
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(errors.New(name+" server"), err)
	}

	return nil
}
