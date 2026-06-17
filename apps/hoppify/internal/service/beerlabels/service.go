package beerlabels

import (
	"context"
	"errors"
	"fmt"
	"strings"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
)

const defaultMaxObjectBytes = 15 * 1024 * 1024

type CaptureRepository interface {
	FindCaptureByUUID(ctx context.Context, id uuid.UUID) (capturemodel.Record, error)
}

type RecognitionRepository interface {
	FindBeerLabelRecognition(
		ctx context.Context,
		captureID uuid.UUID,
		promptVersion string,
	) (beerlabelmodel.Record, error)
	InsertBeerLabelRecognition(ctx context.Context, record *beerlabelmodel.Record) error
}

type ObjectStorage interface {
	GetObject(ctx context.Context, bucket string, objectKey string, maxBytes int64) (capturemodel.Object, error)
}

type Recognizer interface {
	IdentifyBeerLabel(ctx context.Context, image []byte) (beerlabelmodel.Result, error)
	Model() string
	PromptVersion() string
}

type Config struct {
	MaxObjectBytes int64
}

type Service struct {
	captures       CaptureRepository
	recognitions   RecognitionRepository
	storage        ObjectStorage
	recognizer     Recognizer
	maxObjectBytes int64
}

func NewService(
	captures CaptureRepository,
	recognitions RecognitionRepository,
	storage ObjectStorage,
	recognizer Recognizer,
	cfg Config,
) (*Service, error) {
	if captures == nil {
		return nil, fmt.Errorf("beer labels capture repository is required")
	}
	if recognitions == nil {
		return nil, fmt.Errorf("beer labels recognition repository is required")
	}
	if storage == nil {
		return nil, fmt.Errorf("beer labels object storage is required")
	}
	if recognizer == nil {
		return nil, fmt.Errorf("beer labels recognizer is required")
	}

	return &Service{
		captures:       captures,
		recognitions:   recognitions,
		storage:        storage,
		recognizer:     recognizer,
		maxObjectBytes: normalizeMaxObjectBytes(cfg.MaxObjectBytes),
	}, nil
}

func (svc *Service) Identify(ctx context.Context, rawUUID string) (beerlabelmodel.Response, error) {
	captureID, err := uuid.Parse(rawUUID)
	if err != nil {
		return beerlabelmodel.Response{}, newError(InvalidRequest, "uuid must be a valid UUID", err)
	}
	promptVersion := svc.recognizer.PromptVersion()

	cached, err := svc.recognitions.FindBeerLabelRecognition(ctx, captureID, promptVersion)
	if err == nil {
		return beerlabelmodel.ResponseFromRecord(&cached, true), nil
	}
	if !errors.Is(err, beerlabelmodel.ErrNotFound) {
		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	capture, err := svc.captures.FindCaptureByUUID(ctx, captureID)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return beerlabelmodel.Response{}, newError(NotFound, "capture was not found", err)
	}
	if err != nil {
		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}
	if capture.ContentType != capturemodel.ContentTypeJPEG {
		return beerlabelmodel.Response{}, newError(UnsupportedMediaType, "capture is not a supported image", nil)
	}

	object, err := svc.storage.GetObject(ctx, capture.Bucket, capture.ObjectKey, svc.maxObjectBytes)
	if err != nil {
		return beerlabelmodel.Response{}, newError(StorageError, "object storage read failed", err)
	}

	result, err := svc.recognizer.IdentifyBeerLabel(ctx, object.Body)
	if errors.Is(err, ErrModelUnavailable) {
		return beerlabelmodel.Response{}, newError(ModelUnavailable, "beer label model is unavailable", err)
	}
	if err != nil {
		return beerlabelmodel.Response{}, newError(InferenceError, "beer label recognition failed", err)
	}
	normalizeResult(&result)
	if err := validateResult(&result); err != nil {
		return beerlabelmodel.Response{}, newError(InferenceError, "beer label recognition returned invalid result", err)
	}

	record := beerlabelmodel.Record{
		CaptureUUID:   captureID,
		Model:         svc.recognizer.Model(),
		PromptVersion: svc.recognizer.PromptVersion(),
		Result:        result,
	}
	if err := svc.recognitions.InsertBeerLabelRecognition(ctx, &record); err != nil {
		cached, findErr := svc.recognitions.FindBeerLabelRecognition(ctx, captureID, promptVersion)
		if findErr == nil {
			return beerlabelmodel.ResponseFromRecord(&cached, true), nil
		}

		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	saved, err := svc.recognitions.FindBeerLabelRecognition(ctx, captureID, promptVersion)
	if err != nil {
		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	return beerlabelmodel.ResponseFromRecord(&saved, false), nil
}

func normalizeMaxObjectBytes(maxObjectBytes int64) int64 {
	if maxObjectBytes <= 0 {
		return defaultMaxObjectBytes
	}

	return maxObjectBytes
}

func normalizeResult(result *beerlabelmodel.Result) {
	result.Status = strings.TrimSpace(result.Status)
	result.Container = strings.TrimSpace(result.Container)
	trimOptionalString(&result.BeerName)
	trimOptionalString(&result.Brewery)
	trimOptionalString(&result.Style)
	trimOptionalString(&result.Country)
	trimOptionalString(&result.Notes)
	if result.Evidence == nil {
		result.Evidence = []string{}
	}
	for index := range result.Evidence {
		result.Evidence[index] = strings.TrimSpace(result.Evidence[index])
	}
	normalizeWebSearchResult(result.WebSearch)
	normalizeUntappdRecommendation(result.Untappd)
}

func validateResult(result *beerlabelmodel.Result) error {
	if !validStatus(result.Status) {
		return fmt.Errorf("invalid status %q", result.Status)
	}
	if !validContainer(result.Container) {
		return fmt.Errorf("invalid container %q", result.Container)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	if result.Untappd != nil {
		if !validUntappdStatus(result.Untappd.Status) {
			return fmt.Errorf("invalid untappd status %q", result.Untappd.Status)
		}
		if result.Untappd.Confidence < 0 || result.Untappd.Confidence > 1 {
			return fmt.Errorf("untappd confidence must be between 0 and 1")
		}
	}

	return nil
}

func validStatus(status string) bool {
	switch status {
	case beerlabelmodel.StatusIdentified,
		beerlabelmodel.StatusUncertain,
		beerlabelmodel.StatusUnreadable,
		beerlabelmodel.StatusNotBeer:
		return true
	default:
		return false
	}
}

func validContainer(container string) bool {
	switch container {
	case beerlabelmodel.ContainerBottle,
		beerlabelmodel.ContainerCan,
		beerlabelmodel.ContainerGlass,
		beerlabelmodel.ContainerOther,
		beerlabelmodel.ContainerUnknown:
		return true
	default:
		return false
	}
}

func validUntappdStatus(status string) bool {
	switch status {
	case beerlabelmodel.UntappdDirectMatch,
		beerlabelmodel.UntappdSearchRecommended,
		beerlabelmodel.UntappdAmbiguous,
		beerlabelmodel.UntappdNotFound,
		beerlabelmodel.UntappdNotApplicable:
		return true
	default:
		return false
	}
}

func trimOptionalString(value **string) {
	if value == nil || *value == nil {
		return
	}
	trimmed := strings.TrimSpace(**value)
	if trimmed == "" {
		*value = nil
		return
	}
	*value = &trimmed
}

func normalizeWebSearchResult(result *beerlabelmodel.WebSearchResult) {
	if result == nil {
		return
	}
	if result.Queries == nil {
		result.Queries = []string{}
	}
	queries := result.Queries[:0]
	for _, query := range result.Queries {
		query = strings.TrimSpace(query)
		if query != "" {
			queries = append(queries, query)
		}
	}
	result.Queries = queries
	if result.Sources == nil {
		result.Sources = []beerlabelmodel.WebSource{}
	}
	sources := result.Sources[:0]
	for _, source := range result.Sources {
		source.URL = strings.TrimSpace(source.URL)
		trimOptionalString(&source.Title)
		if source.URL != "" {
			sources = append(sources, source)
		}
	}
	result.Sources = sources
	if len(result.Queries) > 0 || len(result.Sources) > 0 {
		result.Used = true
	}
}

func normalizeUntappdRecommendation(recommendation *beerlabelmodel.UntappdRecommendation) {
	if recommendation == nil {
		return
	}
	recommendation.Status = strings.TrimSpace(recommendation.Status)
	trimOptionalString(&recommendation.URL)
	trimOptionalString(&recommendation.SearchURL)
	trimOptionalString(&recommendation.Name)
	trimOptionalString(&recommendation.Brewery)
	trimOptionalString(&recommendation.Reason)
}
