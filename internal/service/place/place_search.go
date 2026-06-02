package place

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/chistopat/fsqr/internal/models"
	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	locationmodel "github.com/chistopat/fsqr/internal/models/location"
	placemodel "github.com/chistopat/fsqr/internal/models/place"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	"github.com/chistopat/fsqr/internal/observability"
	placesrepo "github.com/chistopat/fsqr/internal/repository/places"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	DefaultSearchLimit          = placesrepo.DefaultSearchLimit
	MinSearchLimit              = placesrepo.MinSearchLimit
	MaxSearchLimit              = placesrepo.MaxSearchLimit
	SearchCategoryLimit         = placesrepo.MaxCategoryIDs
	DefaultSearchDistanceMeters = 5000
	MinSearchDistanceMeters     = 1
)

type PlaceSearchService interface {
	SearchPlaces(ctx context.Context, input SearchPlacesInput) (models.SearchResponse, error)
}

type SearchPlacesInput struct {
	Query          string
	Location       locationmodel.Location
	Limit          int
	DistanceMeters int
}

type PlaceSearchCategoryService interface {
	SearchCategories(ctx context.Context, query querymodel.Query) ([]categorymodel.Category, error)
}

type PlaceRepository interface {
	Search(ctx context.Context, input placesrepo.SearchInput) ([]placemodel.Place, error)
}

type PlaceSearch struct {
	categories PlaceSearchCategoryService
	places     PlaceRepository
	logger     *zap.Logger
}

type InvalidSearchInputError struct {
	Err error
}

func NewPlaceSearch(
	categories PlaceSearchCategoryService,
	places PlaceRepository,
	loggers ...*zap.Logger,
) *PlaceSearch {
	return &PlaceSearch{
		categories: categories,
		places:     places,
		logger:     optionalLogger(loggers),
	}
}

func (service *PlaceSearch) SearchPlaces(
	ctx context.Context,
	input SearchPlacesInput,
) (models.SearchResponse, error) {
	started := time.Now()
	limit, err := normalizeSearchLimit(input.Limit)
	if err != nil {
		return models.SearchResponse{}, invalidSearchInput(err)
	}
	if !input.Location.Valid() {
		return models.SearchResponse{}, invalidSearchInput(fmt.Errorf("location is required"))
	}
	distanceMeters, err := normalizeSearchDistance(input.DistanceMeters)
	if err != nil {
		return models.SearchResponse{}, invalidSearchInput(err)
	}

	query, err := querymodel.New(input.Query, SearchCategoryLimit)
	if err != nil {
		return models.SearchResponse{}, invalidSearchInput(err)
	}

	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/service/place").Start(
		ctx,
		"search.places",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("search.query", query.String()),
			attribute.Int("search.limit", limit),
			attribute.Int("search.distance_meters", distanceMeters),
			attribute.Float64("geo.center.latitude", input.Location.Lat()),
			attribute.Float64("geo.center.longitude", input.Location.Lon()),
		),
	)
	defer span.End()

	if service.categories == nil {
		err := fmt.Errorf("category service is not configured")
		observability.RecordSpanError(span, err)
		return models.SearchResponse{}, err
	}
	if service.places == nil {
		err := fmt.Errorf("place repository is not configured")
		observability.RecordSpanError(span, err)
		return models.SearchResponse{}, err
	}

	categories, err := service.categories.SearchCategories(ctx, query)
	if err != nil {
		err = fmt.Errorf("search categories: %w", err)
		observability.RecordSpanError(span, err)
		return models.SearchResponse{}, err
	}

	categoryIDs := mapCategoryIDs(categories)
	if len(categoryIDs) == 0 {
		response := models.SearchResponse{
			TookMS: elapsedMillis(started),
			Places: []models.SearchPlace{},
		}
		service.log().Debug(
			"place search response",
			zap.String("query", query.String()),
			zap.Int("limit", limit),
			zap.Int("distance_meters", distanceMeters),
			zap.Int("categories", len(categoryIDs)),
			zap.Int("places", 0),
			zap.Duration("elapsed", time.Since(started)),
		)
		span.SetAttributes(
			attribute.Int("search.categories", 0),
			attribute.Int("search.returned_places", 0),
		)

		return response, nil
	}

	bbox, err := locationmodel.NewBBoxAround(input.Location, float64(distanceMeters))
	if err != nil {
		err = invalidSearchInput(err)
		observability.RecordSpanError(span, err)
		return models.SearchResponse{}, err
	}

	places, err := service.places.Search(ctx, placesrepo.SearchInput{
		CategoryIDs: categoryIDs,
		Location:    input.Location,
		Limit:       limit,
		BBox:        &bbox,
	})
	if err != nil {
		err = fmt.Errorf("search places: %w", err)
		observability.RecordSpanError(span, err)
		return models.SearchResponse{}, err
	}

	response := models.SearchResponse{
		TookMS: elapsedMillis(started),
		Places: mapSearchPlaces(places),
	}
	service.log().Debug(
		"place search response",
		zap.String("query", query.String()),
		zap.Int("limit", limit),
		zap.Int("distance_meters", distanceMeters),
		zap.Int("categories", len(categoryIDs)),
		zap.Int("places", len(response.Places)),
		zap.Duration("elapsed", time.Since(started)),
	)
	span.SetAttributes(
		attribute.Int("search.categories", len(categoryIDs)),
		attribute.Int("search.returned_places", len(response.Places)),
	)

	return response, nil
}

func normalizeSearchLimit(limit int) (int, error) {
	if limit == 0 {
		return DefaultSearchLimit, nil
	}
	if limit < MinSearchLimit || limit > MaxSearchLimit {
		return 0, fmt.Errorf("limit must be between %d and %d", MinSearchLimit, MaxSearchLimit)
	}

	return limit, nil
}

func normalizeSearchDistance(distanceMeters int) (int, error) {
	if distanceMeters == 0 {
		return DefaultSearchDistanceMeters, nil
	}
	if distanceMeters < MinSearchDistanceMeters {
		return 0, fmt.Errorf("distance_meters must be positive")
	}

	return distanceMeters, nil
}

func invalidSearchInput(err error) error {
	return InvalidSearchInputError{Err: err}
}

func IsInvalidSearchInput(err error) bool {
	var invalid InvalidSearchInputError

	return errors.As(err, &invalid)
}

func (err InvalidSearchInputError) Error() string {
	if err.Err == nil {
		return "invalid search input"
	}

	return err.Err.Error()
}

func (err InvalidSearchInputError) Unwrap() error {
	return err.Err
}

func mapCategoryIDs(categories []categorymodel.Category) []int64 {
	categoryIDs := make([]int64, 0, len(categories))
	for _, category := range categories {
		categoryIDs = append(categoryIDs, category.ID)
	}

	return categoryIDs
}

func mapSearchPlaces(places []placemodel.Place) []models.SearchPlace {
	result := make([]models.SearchPlace, 0, len(places))
	for _, place := range places {
		result = append(result, models.SearchPlace{
			UUID: place.UUID,
			Name: place.Name,
			Lat:  place.Lat,
			Lon:  place.Lon,
		})
	}

	return result
}

func elapsedMillis(started time.Time) int32 {
	millis := time.Since(started).Milliseconds()
	if millis < 0 {
		return 0
	}
	if millis > math.MaxInt32 {
		return math.MaxInt32
	}

	return int32(millis)
}

func (service *PlaceSearch) log() *zap.Logger {
	if service.logger != nil {
		return service.logger
	}

	return zap.NewNop()
}

func optionalLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}

	return zap.NewNop()
}
