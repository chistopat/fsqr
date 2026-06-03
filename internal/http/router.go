package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/chistopat/fsqr/internal/models"
	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	locationmodel "github.com/chistopat/fsqr/internal/models/location"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	placeservice "github.com/chistopat/fsqr/internal/service/place"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

type Dependencies struct {
	SearchService   placeservice.PlaceSearchService
	PlaceService    placeservice.PlaceService
	CategoryService CategoryService
	HealthChecker   HealthChecker
	MetricsRegistry *prometheus.Registry
	Logger          *zap.Logger
}

type CategoryService interface {
	SearchCategories(ctx context.Context, query querymodel.Query) ([]categorymodel.Category, error)
}

type HealthChecker interface {
	PingContext(ctx context.Context) error
}

func NewRouter(deps Dependencies) *fiber.App {
	log := loggerOrNop(deps.Logger)
	metricsRegistry := newMetricsRegistry(deps.MetricsRegistry)
	app := fiber.New(fiber.Config{
		AppName:      "fsqr API",
		ErrorHandler: errorHandler(log),
	})

	app.Use(recover.New())
	app.Use(traceRequests())
	app.Use(recordHTTPMetrics(newHTTPMetrics(metricsRegistry)))
	app.Get("/live", live)
	app.Get("/health", health(deps.HealthChecker, log))
	app.Get("/swagger.json", serveSwaggerJSON)
	app.Get("/swagger", serveSwaggerViewer)
	app.Get("/swagger/", serveSwaggerViewer)
	registerFrontend(app)

	api := app.Group("/api/v1")
	api.Get("/search", searchPlaces(deps.SearchService, log))
	api.Get("/categories", searchCategories(deps.CategoryService, log))
	api.Get("/places/:uuid", getPlace(deps.PlaceService, log))

	return app
}

func NewMetricsRouter(registry *prometheus.Registry, metricsPath string) *fiber.App {
	app := fiber.New(fiber.Config{
		AppName: "fsqr metrics",
	})

	app.Get(normalizeMetricsPath(metricsPath), metricsHandler(newMetricsRegistry(registry)))

	return app
}

func live(ctx *fiber.Ctx) error {
	return ctx.JSON(models.HealthResponse{Status: "ok"})
}

func health(checker HealthChecker, log *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if checker == nil {
			return writeInternalError(ctx, log, errors.New("health checker is not wired"))
		}

		checkCtx, cancel := context.WithTimeout(ctx.UserContext(), 2*time.Second)
		defer cancel()

		if err := checker.PingContext(checkCtx); err != nil {
			logInternalError(ctx, log, err)

			return ctx.Status(fiber.StatusServiceUnavailable).JSON(models.HealthResponse{Status: "unhealthy"})
		}

		return ctx.JSON(models.HealthResponse{Status: "ok"})
	}
}

func searchCategories(categoryService CategoryService, log *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if categoryService == nil {
			return writeInternalError(ctx, log, errors.New("category service is not wired"))
		}

		limit, err := queryLimit(ctx)
		if err != nil {
			return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, "limit must be an integer")
		}

		query, err := querymodel.New(ctx.Query("query"), limit)
		if err != nil {
			return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
		}

		categories, err := categoryService.SearchCategories(ctx.UserContext(), query)
		if err != nil {
			return writeInternalError(ctx, log, err)
		}

		return ctx.JSON(categories)
	}
}

func searchPlaces(searchService placeservice.PlaceSearchService, log *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if searchService == nil {
			return writeInternalError(ctx, log, errors.New("search service is not wired"))
		}

		limit, err := searchLimit(ctx)
		if err != nil {
			return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
		}

		location, err := parseLocation(ctx.Query("location"))
		if err != nil {
			return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
		}

		distanceMeters, err := searchDistance(ctx)
		if err != nil {
			return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
		}

		result, err := searchService.SearchPlaces(ctx.UserContext(), placeservice.SearchPlacesInput{
			Query:          ctx.Query("query"),
			Location:       location,
			Limit:          limit,
			DistanceMeters: distanceMeters,
		})
		if err != nil {
			if placeservice.IsInvalidSearchInput(err) {
				return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
			}

			return writeInternalError(ctx, log, err)
		}

		return ctx.JSON(result)
	}
}

func getPlace(placeService placeservice.PlaceService, log *zap.Logger) fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		if placeService == nil {
			return writeInternalError(ctx, log, errors.New("place service is not wired"))
		}

		result, err := placeService.GetPlace(ctx.UserContext(), ctx.Params("uuid"))
		if err != nil {
			if placeservice.IsInvalidPlaceInput(err) {
				return writeError(ctx, fiber.StatusBadRequest, models.InvalidRequest, err.Error())
			}
			if placeservice.IsPlaceNotFound(err) {
				return writeError(ctx, fiber.StatusNotFound, models.NotFound, "place was not found")
			}

			return writeInternalError(ctx, log, err)
		}

		return ctx.JSON(result)
	}
}

func queryLimit(ctx *fiber.Ctx) (int, error) {
	rawLimit := ctx.Query("limit")
	if rawLimit == "" {
		return querymodel.DefaultLimit, nil
	}

	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}

	return limit, nil
}

func searchLimit(ctx *fiber.Ctx) (int, error) {
	rawLimit := ctx.Query("limit")
	if rawLimit == "" {
		return placeservice.DefaultSearchLimit, nil
	}

	limit, err := strconv.Atoi(rawLimit)
	if err != nil {
		return 0, fmt.Errorf("limit must be an integer")
	}
	if limit < placeservice.MinSearchLimit || limit > placeservice.MaxSearchLimit {
		return 0, fmt.Errorf(
			"limit must be between %d and %d",
			placeservice.MinSearchLimit,
			placeservice.MaxSearchLimit,
		)
	}

	return limit, nil
}

func searchDistance(ctx *fiber.Ctx) (int, error) {
	rawDistance := ctx.Query("distance_meters")
	if rawDistance == "" {
		return placeservice.DefaultSearchDistanceMeters, nil
	}

	distanceMeters, err := strconv.Atoi(rawDistance)
	if err != nil {
		return 0, fmt.Errorf("distance_meters must be an integer")
	}
	if distanceMeters < placeservice.MinSearchDistanceMeters {
		return 0, fmt.Errorf("distance_meters must be positive")
	}

	return distanceMeters, nil
}

func parseLocation(raw string) (locationmodel.Location, error) {
	lat, lon, err := parseCoordinatePair(raw, "location")
	if err != nil {
		return locationmodel.Location{}, err
	}

	location, err := locationmodel.New(lat, lon)
	if err != nil {
		return locationmodel.Location{}, fmt.Errorf("location is invalid: %w", err)
	}

	return location, nil
}

func parseCoordinatePair(raw, name string) (lat, lon float64, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, 0, invalidLocationFormat(name)
	}

	parts := strings.Split(raw, ",")
	if len(parts) != 2 {
		return 0, 0, invalidLocationFormat(name)
	}

	lat, err = parseFloat(parts[0])
	if err != nil {
		return 0, 0, invalidLocationFormat(name)
	}
	lon, err = parseFloat(parts[1])
	if err != nil {
		return 0, 0, invalidLocationFormat(name)
	}

	return lat, lon, nil
}

func parseFloat(raw string) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0, fmt.Errorf("parse coordinate: %w", err)
	}

	return value, nil
}

func invalidLocationFormat(name string) error {
	return &formatError{name: name, format: "lat,lon"}
}

type formatError struct {
	name   string
	format string
}

func (err *formatError) Error() string {
	return err.name + " must be in \"" + err.format + "\" format"
}

func writeError(ctx *fiber.Ctx, status int, code models.ErrorCode, message string) error {
	return ctx.Status(status).JSON(models.ErrorResponse{
		Error: models.APIError{
			Code:    code,
			Message: message,
		},
	})
}

func writeInternalError(ctx *fiber.Ctx, log *zap.Logger, err error) error {
	logInternalError(ctx, log, err)

	return writeError(ctx, fiber.StatusInternalServerError, models.InternalError, "internal server error")
}

func errorHandler(log *zap.Logger) fiber.ErrorHandler {
	return func(ctx *fiber.Ctx, err error) error {
		var fiberError *fiber.Error
		if errors.As(err, &fiberError) && fiberError.Code < fiber.StatusInternalServerError {
			return writeError(ctx, fiberError.Code, models.InvalidRequest, fiberError.Message)
		}

		return writeInternalError(ctx, log, err)
	}
}

func logInternalError(ctx *fiber.Ctx, log *zap.Logger, err error) {
	if log == nil {
		return
	}

	log.Error(
		"http request failed",
		zap.Error(err),
		zap.String("method", ctx.Method()),
		zap.String("path", ctx.Path()),
		zap.String("route", routeLabel(ctx)),
	)
}

func loggerOrNop(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}

	return log
}
