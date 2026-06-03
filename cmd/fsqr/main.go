package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chistopat/fsqr/internal/config"
	"github.com/chistopat/fsqr/internal/db"
	"github.com/chistopat/fsqr/internal/embeddings"
	httpapi "github.com/chistopat/fsqr/internal/http"
	"github.com/chistopat/fsqr/internal/logger"
	"github.com/chistopat/fsqr/internal/observability"
	categoryrepo "github.com/chistopat/fsqr/internal/repository/categories"
	placesrepo "github.com/chistopat/fsqr/internal/repository/places"
	categoryservice "github.com/chistopat/fsqr/internal/service/category"
	placeservice "github.com/chistopat/fsqr/internal/service/place"

	"github.com/gofiber/fiber/v2"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	appLogger, err := logger.New(cfg.Logger)
	if err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer func() {
		_ = appLogger.Sync()
	}()

	ctx := context.Background()
	shutdownTracing, err := observability.InitTracing(
		ctx,
		cfg.Observability.ServiceName,
		cfg.App.Env,
		cfg.Observability.Tracing,
	)
	if err != nil {
		log.Fatalf("init tracing: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := shutdownTracing(shutdownCtx); err != nil {
			appLogger.Warn("shutdown tracing", zap.Error(err))
		}
	}()

	database, err := db.OpenPostgres(ctx, cfg.Database)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer func() {
		_ = database.Close()
	}()

	categoryRepository, err := categoryrepo.New(database, appLogger.Named("categories.repository"))
	if err != nil {
		log.Fatalf("init category repository: %v", err)
	}
	placeRepository, err := placesrepo.New(database, appLogger.Named("places.repository"))
	if err != nil {
		log.Fatalf("init place repository: %v", err)
	}

	embedder, err := embeddings.NewOpenAIClient(embeddings.OpenAIConfig{
		BaseURL: cfg.Embeddings.BaseURL,
		APIKey:  cfg.Embeddings.APIKey,
		Model:   cfg.Embeddings.Model,
		Timeout: cfg.Embeddings.Timeout,
	}, appLogger.Named("embeddings.openai"))
	if err != nil {
		log.Fatalf("init embeddings client: %v", err)
	}

	categoryService := categoryservice.New(categoryRepository, embedder, appLogger.Named("categories.service"))
	placeSearchService := placeservice.NewPlaceSearch(
		categoryService,
		placeRepository,
		appLogger.Named("places.service"),
	)
	placeDetailsService := placeservice.NewPlaceDetails(
		placeRepository,
		appLogger.Named("places.details.service"),
	)
	metricsRegistry := prometheus.NewRegistry()
	app := httpapi.NewRouter(httpapi.Dependencies{
		SearchService:   placeSearchService,
		PlaceService:    placeDetailsService,
		CategoryService: categoryService,
		HealthChecker:   database,
		MetricsRegistry: metricsRegistry,
		Logger:          appLogger.Named("http"),
		WebConfig: &httpapi.WebConfig{
			MapboxAccessToken: cfg.Web.MapboxAccessToken,
			MapboxStyle:       cfg.Web.MapboxStyle,
		},
	})
	metricsApp := httpapi.NewMetricsRouter(metricsRegistry, cfg.Observability.Metrics.Path)
	appLogger.Info(
		"starting http server",
		zap.String("app", cfg.App.Name),
		zap.String("env", cfg.App.Env),
		zap.String("addr", cfg.HTTP.Addr),
	)
	appLogger.Info(
		"starting metrics server",
		zap.String("addr", cfg.Observability.Metrics.Addr),
		zap.String("path", cfg.Observability.Metrics.Path),
	)

	serverErrors := make(chan error, 2)
	go listenFiber(app, cfg.HTTP.Addr, "http", serverErrors)
	go listenFiber(metricsApp, cfg.Observability.Metrics.Addr, "metrics", serverErrors)

	signalCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	select {
	case <-signalCtx.Done():
		appLogger.Info("shutdown signal received")
	case err := <-serverErrors:
		appLogger.Error("server stopped unexpectedly", zap.Error(err))
		os.Exit(1)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := shutdownFiber(shutdownCtx, app, "http"); err != nil {
		appLogger.Error("shutdown http server", zap.Error(err))
		os.Exit(1)
	}
	if err := shutdownFiber(shutdownCtx, metricsApp, "metrics"); err != nil {
		appLogger.Error("shutdown metrics server", zap.Error(err))
		os.Exit(1)
	}

	appLogger.Info("shutdown complete")
}

func listenFiber(app *fiber.App, addr, name string, serverErrors chan<- error) {
	if err := app.Listen(addr); err != nil {
		serverErrors <- errors.Join(errors.New(name+" server"), err)
	}
}

func shutdownFiber(ctx context.Context, app *fiber.App, name string) error {
	if err := app.ShutdownWithContext(ctx); err != nil {
		return errors.Join(errors.New(name+" server"), err)
	}

	return nil
}
