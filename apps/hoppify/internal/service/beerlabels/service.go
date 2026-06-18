package beerlabels

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	"github.com/chistopat/hoppify/internal/service/imagesource"

	"github.com/google/uuid"
)

const (
	defaultMaxObjectBytes = 15 * 1024 * 1024
	defaultListLimit      = 30
	maxListLimit          = 100
)

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
	ListBeerLabelRecognitions(ctx context.Context, query capturemodel.ListQuery) (beerlabelmodel.ListResult, error)
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

type imageSource struct {
	body      []byte
	captureID uuid.NullUUID
	cacheable bool
	url       string
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

func (svc *Service) Identify(ctx context.Context, request beerlabelmodel.Request) (beerlabelmodel.Response, error) {
	if err := validateRequestSource(request); err != nil {
		return beerlabelmodel.Response{}, err
	}

	if request.UUID != "" {
		return svc.identifyByUUID(ctx, request.UUID)
	}

	return svc.identifyByS3URL(ctx, request.ImageURL())
}

func (svc *Service) ListRecognitions(
	ctx context.Context,
	query capturemodel.ListQuery,
) (beerlabelmodel.ListResponse, error) {
	query = normalizeListQuery(query)

	result, err := svc.recognitions.ListBeerLabelRecognitions(ctx, query)
	if err != nil {
		return beerlabelmodel.ListResponse{}, newError(InternalError, "internal server error", err)
	}

	return beerlabelmodel.ListResponseFromRecords(result.Records, query, result.HasMore), nil
}

func (svc *Service) identifyByUUID(ctx context.Context, rawUUID string) (beerlabelmodel.Response, error) {
	captureID, err := uuid.Parse(strings.TrimSpace(rawUUID))
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

	source := imageSource{
		body: object.Body,
		captureID: uuid.NullUUID{
			UUID:  captureID,
			Valid: true,
		},
		cacheable: true,
		url:       capturemodel.URI(capture.Bucket, capture.ObjectKey),
	}

	return svc.identifyAndMaybeCache(ctx, source)
}

func (svc *Service) identifyByS3URL(ctx context.Context, rawURL string) (beerlabelmodel.Response, error) {
	bucket, objectKey, err := imagesource.ParseS3URI(rawURL)
	if err != nil {
		return beerlabelmodel.Response{}, newError(InvalidRequest, "url must be an s3 uri", err)
	}

	captureID, cacheable, err := svc.captureIdentityForObjectKey(ctx, objectKey)
	if err != nil {
		return beerlabelmodel.Response{}, err
	}
	sourceURL := capturemodel.URI(bucket, objectKey)
	promptVersion := svc.recognizer.PromptVersion()
	if cacheable {
		cached, err := svc.recognitions.FindBeerLabelRecognition(ctx, captureID.UUID, promptVersion)
		if err == nil {
			response := beerlabelmodel.ResponseFromRecord(&cached, true)
			response.URL = sourceURL

			return response, nil
		}
		if !errors.Is(err, beerlabelmodel.ErrNotFound) {
			return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
		}
	}

	object, err := svc.storage.GetObject(ctx, bucket, objectKey, svc.maxObjectBytes)
	if err != nil {
		return beerlabelmodel.Response{}, newError(StorageError, "object storage read failed", err)
	}
	if err := validateS3JPEGContentType(object.ContentType); err != nil {
		return beerlabelmodel.Response{}, err
	}

	return svc.identifyAndMaybeCache(ctx, imageSource{
		body:      object.Body,
		captureID: captureID,
		cacheable: cacheable,
		url:       sourceURL,
	})
}

func (svc *Service) identifyAndMaybeCache(
	ctx context.Context,
	source imageSource,
) (beerlabelmodel.Response, error) {
	result, err := svc.recognizer.IdentifyBeerLabel(ctx, source.body)
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
		CaptureUUID:   source.captureID.UUID,
		Model:         svc.recognizer.Model(),
		PromptVersion: svc.recognizer.PromptVersion(),
		Result:        result,
		CreatedAt:     time.Now().UTC(),
	}
	if !source.cacheable {
		return responseFromRecord(&record, false, source), nil
	}

	if err := svc.recognitions.InsertBeerLabelRecognition(ctx, &record); err != nil {
		cached, findErr := svc.recognitions.FindBeerLabelRecognition(ctx, source.captureID.UUID, record.PromptVersion)
		if findErr == nil {
			return responseFromRecord(&cached, true, source), nil
		}

		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	saved, err := svc.recognitions.FindBeerLabelRecognition(ctx, source.captureID.UUID, record.PromptVersion)
	if err != nil {
		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	return responseFromRecord(&saved, false, source), nil
}

func (svc *Service) captureIdentityForObjectKey(
	ctx context.Context,
	objectKey string,
) (uuid.NullUUID, bool, error) {
	captureID, ok := imagesource.UUIDFromObjectKey(objectKey)
	if !ok {
		return uuid.NullUUID{}, false, nil
	}

	_, err := svc.captures.FindCaptureByUUID(ctx, captureID)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return uuid.NullUUID{UUID: captureID, Valid: true}, false, nil
	}
	if err != nil {
		return uuid.NullUUID{}, false, newError(InternalError, "internal server error", err)
	}

	return uuid.NullUUID{UUID: captureID, Valid: true}, true, nil
}

func normalizeMaxObjectBytes(maxObjectBytes int64) int64 {
	if maxObjectBytes <= 0 {
		return defaultMaxObjectBytes
	}

	return maxObjectBytes
}

func normalizeListQuery(query capturemodel.ListQuery) capturemodel.ListQuery {
	if query.Limit <= 0 {
		query.Limit = defaultListLimit
	}
	if query.Limit > maxListLimit {
		query.Limit = maxListLimit
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	return query
}

func validateRequestSource(request beerlabelmodel.Request) error {
	sourceCount := 0
	if strings.TrimSpace(request.UUID) != "" {
		sourceCount++
	}
	if strings.TrimSpace(request.ImageURL()) != "" {
		sourceCount++
	}
	if sourceCount != 1 {
		return newError(InvalidRequest, "exactly one image source is required", nil)
	}

	return nil
}

func responseFromRecord(record *beerlabelmodel.Record, cached bool, source imageSource) beerlabelmodel.Response {
	response := beerlabelmodel.ResponseFromRecord(record, cached)
	if !source.captureID.Valid {
		response.UUID = ""
	}
	response.URL = source.url

	return response
}

func validateS3JPEGContentType(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	contentType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return newError(UnsupportedMediaType, "invalid s3 object content type", err)
	}
	if !strings.EqualFold(contentType, capturemodel.ContentTypeJPEG) {
		return newError(UnsupportedMediaType, "s3 object is not a supported image", nil)
	}

	return nil
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
	trimOptionalString(&result.SearchEntryPointHTML)
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
