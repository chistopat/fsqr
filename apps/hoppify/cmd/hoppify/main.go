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
	httpapi "github.com/chistopat/hoppify/internal/http"
	"github.com/chistopat/hoppify/internal/logger"
	capturerepo "github.com/chistopat/hoppify/internal/repository/captures"
	captureservice "github.com/chistopat/hoppify/internal/service/captures"
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

	metrics := httpapi.NewMetrics(repository, appLogger.Named("metrics"))
	server := &http.Server{
		Addr: cfg.HTTP.Addr,
		Handler: httpapi.NewHandler(
			httpapi.WithCaptureService(captureService),
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
