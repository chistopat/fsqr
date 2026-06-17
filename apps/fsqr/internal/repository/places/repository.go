package places

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chistopat/fsqr/internal/models"
	locationmodel "github.com/chistopat/fsqr/internal/models/location"
	placemodel "github.com/chistopat/fsqr/internal/models/place"
	"github.com/chistopat/fsqr/internal/observability"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	DefaultSearchLimit = 128
	MinSearchLimit     = 1
	MaxSearchLimit     = 128
	MaxCategoryIDs     = 3
)

var ErrPlaceNotFound = errors.New("place was not found")

//go:embed search_bbox.sql
var searchBBoxSQL string

//go:embed search_bbox_antimeridian.sql
var searchBBoxAntimeridianSQL string

//go:embed search_no_bbox_any.sql
var searchNoBBoxAnySQL string

//go:embed search_no_bbox_category_first.sql
var searchNoBBoxCategoryFirstSQL string

//go:embed get_by_uuid.sql
var getByUUIDSQL string

type Repository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

type SearchInput struct {
	CategoryIDs []int64
	Location    locationmodel.Location
	Limit       int
	BBox        *locationmodel.BBox
}

func New(db *sqlx.DB, loggers ...*zap.Logger) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("places postgres db is required")
	}

	return &Repository{
		db:     db,
		logger: optionalLogger(loggers),
	}, nil
}

func (repo *Repository) Search(ctx context.Context, input SearchInput) ([]placemodel.Place, error) {
	normalized, err := normalizeSearchInput(input)
	if err != nil {
		return nil, err
	}

	query := selectSearchQuery(normalized)
	started := time.Now()
	compactStatement := compactSQL(query.statement)
	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/repository/places").Start(
		ctx,
		"places.repository.search."+query.operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
			attribute.String("db.query.text", compactStatement),
			attribute.String("place.search.operation", query.operation),
			attribute.Int("place.search.category_count", len(normalized.categoryIDs)),
			attribute.Int("place.search.limit", normalized.limit),
			attribute.Float64("geo.center.latitude", normalized.location.Lat()),
			attribute.Float64("geo.center.longitude", normalized.location.Lon()),
			attribute.Bool("geo.bbox.present", normalized.bbox != nil),
		),
	)
	defer span.End()

	var rows []row
	if err := repo.db.SelectContext(ctx, &rows, query.statement, query.args...); err != nil {
		err = fmt.Errorf("select place matches: %w", err)
		observability.RecordSpanError(span, err)
		repo.log().Debug(
			"place query failed",
			zap.String("operation", query.operation),
			zap.String("sql", compactStatement),
			zap.Int("category_count", len(normalized.categoryIDs)),
			zap.Int("limit", normalized.limit),
			zap.Float64("lat", normalized.location.Lat()),
			zap.Float64("lon", normalized.location.Lon()),
			zap.Bool("bbox", normalized.bbox != nil),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return nil, err
	}

	places := make([]placemodel.Place, 0, len(rows))
	for _, row := range rows {
		places = append(places, mapPlace(row))
	}

	repo.log().Debug(
		"place query",
		zap.String("operation", query.operation),
		zap.String("sql", compactStatement),
		zap.Int("category_count", len(normalized.categoryIDs)),
		zap.Int("limit", normalized.limit),
		zap.Float64("lat", normalized.location.Lat()),
		zap.Float64("lon", normalized.location.Lon()),
		zap.Bool("bbox", normalized.bbox != nil),
		zap.Int("rows", len(rows)),
		zap.Duration("elapsed", time.Since(started)),
	)

	span.SetAttributes(attribute.Int("db.response.returned_rows", len(rows)))

	return places, nil
}

func (repo *Repository) GetByUUID(ctx context.Context, uuid string) (models.PlaceDetails, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return models.PlaceDetails{}, fmt.Errorf("place uuid is required")
	}

	started := time.Now()
	compactStatement := compactSQL(getByUUIDSQL)
	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/repository/places").Start(
		ctx,
		"places.repository.get_by_uuid",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
			attribute.String("db.query.text", compactStatement),
		),
	)
	defer span.End()

	var row detailRow
	if err := repo.db.GetContext(ctx, &row, getByUUIDSQL, uuid); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return models.PlaceDetails{}, ErrPlaceNotFound
		}

		err = fmt.Errorf("select place by uuid: %w", err)
		observability.RecordSpanError(span, err)
		repo.log().Debug(
			"place get by uuid failed",
			zap.String("sql", compactStatement),
			zap.String("uuid", uuid),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return models.PlaceDetails{}, err
	}

	place := mapPlaceDetails(&row)
	repo.log().Debug(
		"place get by uuid",
		zap.String("sql", compactStatement),
		zap.String("uuid", uuid),
		zap.Duration("elapsed", time.Since(started)),
	)
	span.SetAttributes(attribute.Int("db.response.returned_rows", 1))

	return place, nil
}

type normalizedSearchInput struct {
	categoryIDs []int64
	location    locationmodel.Location
	limit       int
	bbox        *locationmodel.BBox
}

type searchQuery struct {
	operation string
	statement string
	args      []any
}

func normalizeSearchInput(input SearchInput) (normalizedSearchInput, error) {
	categoryIDs, err := normalizeCategoryIDs(input.CategoryIDs)
	if err != nil {
		return normalizedSearchInput{}, err
	}
	if !input.Location.Valid() {
		return normalizedSearchInput{}, fmt.Errorf("place search location is required")
	}

	limit := input.Limit
	if limit == 0 {
		limit = DefaultSearchLimit
	}
	if limit < MinSearchLimit || limit > MaxSearchLimit {
		return normalizedSearchInput{}, fmt.Errorf(
			"place search limit must be between %d and %d",
			MinSearchLimit,
			MaxSearchLimit,
		)
	}

	bbox := input.BBox
	if bbox != nil {
		if err := validateBBox(*bbox); err != nil {
			return normalizedSearchInput{}, err
		}
	}

	return normalizedSearchInput{
		categoryIDs: categoryIDs,
		location:    input.Location,
		limit:       limit,
		bbox:        bbox,
	}, nil
}

func normalizeCategoryIDs(categoryIDs []int64) ([]int64, error) {
	seen := make(map[int64]struct{}, len(categoryIDs))
	normalized := make([]int64, 0, len(categoryIDs))
	for _, categoryID := range categoryIDs {
		if categoryID <= 0 {
			return nil, fmt.Errorf("place search category ids must be positive")
		}
		if _, ok := seen[categoryID]; ok {
			continue
		}

		seen[categoryID] = struct{}{}
		normalized = append(normalized, categoryID)
	}

	if len(normalized) == 0 {
		return nil, fmt.Errorf("place search category ids are required")
	}
	if len(normalized) > MaxCategoryIDs {
		return nil, fmt.Errorf("place search category ids must contain at most %d values", MaxCategoryIDs)
	}

	return normalized, nil
}

func validateBBox(bbox locationmodel.BBox) error {
	if !bbox.Valid() {
		return fmt.Errorf("place search bbox is invalid")
	}

	return nil
}

func selectSearchQuery(input normalizedSearchInput) searchQuery {
	args := []any{input.categoryIDs, input.location.Lon(), input.location.Lat()}
	if input.bbox != nil {
		if input.bbox.CrossesAntimeridian() {
			positiveMinLon, negativeMaxLon := antimeridianLongitudeRanges(input.location, *input.bbox)
			args = append(
				args,
				input.bbox.Min().Lat(),
				input.bbox.Max().Lat(),
				positiveMinLon,
				negativeMaxLon,
				input.limit,
			)

			return searchQuery{
				operation: "bbox_antimeridian",
				statement: searchBBoxAntimeridianSQL,
				args:      args,
			}
		}

		args = append(
			args,
			input.bbox.Min().Lon(),
			input.bbox.Min().Lat(),
			input.bbox.Max().Lon(),
			input.bbox.Max().Lat(),
			input.limit,
		)

		return searchQuery{
			operation: "bbox",
			statement: searchBBoxSQL,
			args:      args,
		}
	}

	args = append(args, input.limit)
	if len(input.categoryIDs) == 1 {
		return searchQuery{
			operation: "no_bbox_category_first",
			statement: searchNoBBoxCategoryFirstSQL,
			args:      args,
		}
	}

	return searchQuery{
		operation: "no_bbox_any",
		statement: searchNoBBoxAnySQL,
		args:      args,
	}
}

func antimeridianLongitudeRanges(
	location locationmodel.Location,
	bbox locationmodel.BBox,
) (positiveMinLon, negativeMaxLon float64) {
	centerLon := location.Lon()
	lonDelta := bbox.LonDeltaDegrees()
	if centerLon >= 0 {
		return centerLon - lonDelta, centerLon + lonDelta - 360
	}

	return centerLon - lonDelta + 360, centerLon + lonDelta
}

func mapPlace(row row) placemodel.Place {
	return placemodel.Place{
		UUID:       row.FSQPlaceID,
		Name:       row.Name.String,
		CategoryID: row.CategoryID,
		Lat:        row.Lat,
		Lon:        row.Lon,
		Distance:   row.Distance,
	}
}

func mapPlaceDetails(row *detailRow) models.PlaceDetails {
	place := models.PlaceDetails{
		UUID: row.UUID,
		Name: row.Name.String,
		Lat:  row.Lat,
		Lon:  row.Lon,
		Category: &models.PlaceCategory{
			FSQCategoryID: row.CategoryFSQCategoryID,
			Name:          row.CategoryName,
			Path:          row.CategoryPath,
		},
	}

	if address := mapPlaceAddress(row); address != nil {
		place.Address = address
	}
	if contacts := mapPlaceContacts(row); contacts != nil {
		place.Contacts = contacts
	}

	return place
}

func mapPlaceAddress(row *detailRow) *models.PlaceAddress {
	address := models.PlaceAddress{
		Line:     nullableString(row.Address),
		Locality: nullableString(row.Locality),
		Region:   nullableString(row.Region),
		Country:  nullableString(row.Country),
	}
	if address.Line == nil &&
		address.Locality == nil &&
		address.Region == nil &&
		address.Country == nil {
		return nil
	}

	return &address
}

func mapPlaceContacts(row *detailRow) *models.PlaceContacts {
	contacts := models.PlaceContacts{
		Tel:        nullableString(row.Tel),
		Website:    nullableString(row.Website),
		Email:      nullableString(row.Email),
		FacebookID: nullableInt64(row.FacebookID),
		Instagram:  nullableString(row.Instagram),
		Twitter:    nullableString(row.Twitter),
	}
	if contacts.Tel == nil &&
		contacts.Website == nil &&
		contacts.Email == nil &&
		contacts.FacebookID == nil &&
		contacts.Instagram == nil &&
		contacts.Twitter == nil {
		return nil
	}

	return &contacts
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}

	return &value.String
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}

	return &value.Int64
}

func IsPlaceNotFound(err error) bool {
	return errors.Is(err, ErrPlaceNotFound)
}

func (repo *Repository) log() *zap.Logger {
	if repo.logger != nil {
		return repo.logger
	}

	return zap.NewNop()
}

func optionalLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}

	return zap.NewNop()
}

func compactSQL(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}
