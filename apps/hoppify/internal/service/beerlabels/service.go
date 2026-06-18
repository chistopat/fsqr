package beerlabels

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"strings"
	"sync"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	"github.com/chistopat/hoppify/internal/service/imagesource"

	"github.com/google/uuid"
)

const (
	defaultMaxObjectBytes          = 15 * 1024 * 1024
	defaultListLimit               = 30
	maxListLimit                   = 100
	defaultRecognitionConcurrency  = 4
	defaultRecognitionRetries      = 2
	defaultRecognitionRetryDelay   = 250 * time.Millisecond
	defaultRecognitionMaxBatchSize = 300
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
	MaxObjectBytes          int64
	RecognitionConcurrency  int
	RecognitionRetries      int
	RecognitionRetryDelay   time.Duration
	RecognitionMaxBatchSize int
}

type Service struct {
	captures                CaptureRepository
	recognitions            RecognitionRepository
	storage                 ObjectStorage
	recognizer              Recognizer
	maxObjectBytes          int64
	recognitionConcurrency  int
	recognitionRetries      int
	recognitionRetryDelay   time.Duration
	recognitionMaxBatchSize int
	recognizerSemaphore     chan struct{}
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

	concurrency := normalizeRecognitionConcurrency(cfg.RecognitionConcurrency)

	return &Service{
		captures:                captures,
		recognitions:            recognitions,
		storage:                 storage,
		recognizer:              recognizer,
		maxObjectBytes:          normalizeMaxObjectBytes(cfg.MaxObjectBytes),
		recognitionConcurrency:  concurrency,
		recognitionRetries:      normalizeRecognitionRetries(cfg.RecognitionRetries),
		recognitionRetryDelay:   normalizeRecognitionRetryDelay(cfg.RecognitionRetryDelay),
		recognitionMaxBatchSize: normalizeRecognitionMaxBatchSize(cfg.RecognitionMaxBatchSize),
		recognizerSemaphore:     make(chan struct{}, concurrency),
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

func (svc *Service) IdentifyBatch(
	ctx context.Context,
	request beerlabelmodel.BatchRequest,
) (beerlabelmodel.BatchResponse, error) {
	items := make([]beerlabelmodel.BatchItem, 0, len(request.UUIDs))
	if err := svc.IdentifyBatchStream(ctx, request, func(item beerlabelmodel.BatchItem) error {
		items = append(items, item)

		return nil
	}); err != nil {
		return beerlabelmodel.BatchResponse{}, err
	}

	return beerlabelmodel.BatchResponse{Recognitions: items}, nil
}

func (svc *Service) IdentifyBatchStream(
	ctx context.Context,
	request beerlabelmodel.BatchRequest,
	emit func(beerlabelmodel.BatchItem) error,
) error {
	if err := svc.validateBatchRequest(request); err != nil {
		return err
	}

	batchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	jobs := make(chan batchJob)
	results := make(chan batchResult)
	var workers sync.WaitGroup
	workerCount := min(svc.recognitionConcurrency, len(request.UUIDs))
	for range workerCount {
		workers.Add(1)
		go svc.identifyBatchWorker(batchCtx, jobs, results, &workers)
	}
	go sendBatchJobs(batchCtx, jobs, request.UUIDs)
	go closeBatchResults(results, &workers)

	return emitBatchResults(batchCtx, cancel, results, len(request.UUIDs), emit)
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
	result, err := svc.identifyWithRetry(ctx, source.body)
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

type batchJob struct {
	index int
	uuid  string
}

type batchResult struct {
	index int
	item  beerlabelmodel.BatchItem
}

func (svc *Service) identifyBatchWorker(
	ctx context.Context,
	jobs <-chan batchJob,
	results chan<- batchResult,
	workers *sync.WaitGroup,
) {
	defer workers.Done()
	for job := range jobs {
		response, err := svc.Identify(ctx, beerlabelmodel.Request{UUID: job.uuid})
		item := beerlabelmodel.BatchItem{UUID: job.uuid}
		if err != nil {
			item.Error = batchErrorFromError(err)
		} else {
			item.Recognition = &response
			if response.UUID != "" {
				item.UUID = response.UUID
			}
		}

		select {
		case results <- batchResult{index: job.index, item: item}:
		case <-ctx.Done():
			return
		}
	}
}

func sendBatchJobs(ctx context.Context, jobs chan<- batchJob, uuids []string) {
	defer close(jobs)
	for index, rawUUID := range uuids {
		select {
		case jobs <- batchJob{index: index, uuid: strings.TrimSpace(rawUUID)}:
		case <-ctx.Done():
			return
		}
	}
}

func closeBatchResults(results chan<- batchResult, workers *sync.WaitGroup) {
	workers.Wait()
	close(results)
}

func emitBatchResults(
	ctx context.Context,
	cancel context.CancelFunc,
	results <-chan batchResult,
	size int,
	emit func(beerlabelmodel.BatchItem) error,
) error {
	completed := 0
	var emitErr error
	for result := range results {
		if emitErr == nil {
			if err := emit(result.item); err != nil {
				emitErr = err
				cancel()
			}
		}
		completed++
	}
	if emitErr != nil {
		return newError(InternalError, "stream batch recognition result", emitErr)
	}
	if completed != size {
		if err := ctx.Err(); err != nil {
			return newError(InternalError, "batch recognition was canceled", err)
		}

		return newError(InternalError, "batch recognition was interrupted", nil)
	}

	return nil
}

func (svc *Service) identifyWithRetry(
	ctx context.Context,
	image []byte,
) (beerlabelmodel.Result, error) {
	var lastErr error
	for attempt := 0; attempt <= svc.recognitionRetries; attempt++ {
		if attempt > 0 {
			if err := sleepContext(ctx, svc.recognitionRetryDelay); err != nil {
				return beerlabelmodel.Result{}, err
			}
		}

		result, err := svc.identifyOnce(ctx, image)
		if err == nil {
			return result, nil
		}
		if !isRetryableRecognizerError(err) {
			return beerlabelmodel.Result{}, err
		}
		lastErr = err
	}

	return beerlabelmodel.Result{}, lastErr
}

func (svc *Service) identifyOnce(ctx context.Context, image []byte) (beerlabelmodel.Result, error) {
	select {
	case svc.recognizerSemaphore <- struct{}{}:
		defer func() { <-svc.recognizerSemaphore }()
	case <-ctx.Done():
		return beerlabelmodel.Result{}, fmt.Errorf("wait for recognizer slot: %w", ctx.Err())
	}

	result, err := svc.recognizer.IdentifyBeerLabel(ctx, image)
	if err != nil {
		return beerlabelmodel.Result{}, fmt.Errorf("identify beer label with recognizer: %w", err)
	}

	return result, nil
}

func isRetryableRecognizerError(err error) bool {
	if errors.Is(err, ErrModelUnavailable) ||
		errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) {
		return false
	}

	return true
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait before recognition retry: %w", ctx.Err())
	}
}

func (svc *Service) validateBatchRequest(request beerlabelmodel.BatchRequest) error {
	if len(request.UUIDs) == 0 {
		return newError(InvalidRequest, "uuids field is required", nil)
	}
	if len(request.UUIDs) > svc.recognitionMaxBatchSize {
		message := fmt.Sprintf("uuids field accepts at most %d values", svc.recognitionMaxBatchSize)
		return newError(InvalidRequest, message, nil)
	}
	for index, rawUUID := range request.UUIDs {
		if _, err := uuid.Parse(strings.TrimSpace(rawUUID)); err != nil {
			message := fmt.Sprintf("uuids[%d] must be a valid UUID", index)
			return newError(InvalidRequest, message, err)
		}
	}

	return nil
}

func batchErrorFromError(err error) *beerlabelmodel.BatchError {
	var labelErr *Error
	if errors.As(err, &labelErr) {
		return &beerlabelmodel.BatchError{
			Code:    string(labelErr.Code),
			Message: labelErr.Message,
		}
	}

	return &beerlabelmodel.BatchError{Code: string(InternalError), Message: "internal server error"}
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

func normalizeRecognitionConcurrency(value int) int {
	if value <= 0 {
		return defaultRecognitionConcurrency
	}

	return value
}

func normalizeRecognitionRetries(value int) int {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return defaultRecognitionRetries
	}

	return value
}

func normalizeRecognitionRetryDelay(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	if value == 0 {
		return defaultRecognitionRetryDelay
	}

	return value
}

func normalizeRecognitionMaxBatchSize(value int) int {
	if value <= 0 {
		return defaultRecognitionMaxBatchSize
	}

	return value
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
