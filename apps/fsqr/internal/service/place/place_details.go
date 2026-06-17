package place

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/chistopat/fsqr/internal/models"
	"github.com/chistopat/fsqr/internal/observability"
	placesrepo "github.com/chistopat/fsqr/internal/repository/places"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const MaxPlaceUUIDRunes = 128

type PlaceService interface {
	GetPlace(ctx context.Context, uuid string) (models.PlaceDetails, error)
}

type PlaceDetailsRepository interface {
	GetByUUID(ctx context.Context, uuid string) (models.PlaceDetails, error)
}

type PlaceDetails struct {
	places PlaceDetailsRepository
	logger *zap.Logger
}

type InvalidPlaceInputError struct {
	Err error
}

func NewPlaceDetails(places PlaceDetailsRepository, loggers ...*zap.Logger) *PlaceDetails {
	return &PlaceDetails{
		places: places,
		logger: optionalLogger(loggers),
	}
}

func (service *PlaceDetails) GetPlace(ctx context.Context, rawUUID string) (models.PlaceDetails, error) {
	uuid, err := normalizePlaceUUID(rawUUID)
	if err != nil {
		return models.PlaceDetails{}, invalidPlaceInput(err)
	}

	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/service/place").Start(
		ctx,
		"places.get",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.Int("place.uuid.length", len([]rune(uuid)))),
	)
	defer span.End()

	if service.places == nil {
		err := fmt.Errorf("place repository is not configured")
		observability.RecordSpanError(span, err)
		return models.PlaceDetails{}, err
	}

	started := time.Now()
	place, err := service.places.GetByUUID(ctx, uuid)
	if err != nil {
		if placesrepo.IsPlaceNotFound(err) {
			return models.PlaceDetails{}, ErrPlaceNotFound
		}

		err = fmt.Errorf("get place by uuid: %w", err)
		observability.RecordSpanError(span, err)
		return models.PlaceDetails{}, err
	}

	service.log().Debug(
		"place details response",
		zap.String("uuid", uuid),
		zap.Duration("elapsed", time.Since(started)),
	)

	return place, nil
}

func normalizePlaceUUID(raw string) (string, error) {
	uuid := strings.TrimSpace(raw)
	if uuid == "" {
		return "", fmt.Errorf("place uuid must not be empty")
	}
	if utf8.RuneCountInString(uuid) > MaxPlaceUUIDRunes {
		return "", fmt.Errorf("place uuid must be at most %d characters", MaxPlaceUUIDRunes)
	}

	return uuid, nil
}

func invalidPlaceInput(err error) error {
	return InvalidPlaceInputError{Err: err}
}

func IsInvalidPlaceInput(err error) bool {
	var invalid InvalidPlaceInputError

	return errors.As(err, &invalid)
}

func (err InvalidPlaceInputError) Error() string {
	if err.Err == nil {
		return "invalid place input"
	}

	return err.Err.Error()
}

func (err InvalidPlaceInputError) Unwrap() error {
	return err.Err
}

var ErrPlaceNotFound = errors.New("place was not found")

func IsPlaceNotFound(err error) bool {
	return errors.Is(err, ErrPlaceNotFound)
}

func (service *PlaceDetails) log() *zap.Logger {
	if service.logger != nil {
		return service.logger
	}

	return zap.NewNop()
}
