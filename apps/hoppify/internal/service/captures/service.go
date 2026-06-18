package captures

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"mime"
	"strings"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
	_ "golang.org/x/image/webp"
)

const (
	captureNamespace  = "dc8d926c-3559-4b27-96a6-54a802af5d0a"
	captureUUIDPrefix = "hoppify-capture-v1|"
	defaultListLimit  = 30
	maxListLimit      = 100
)

type Repository interface {
	InsertCaptures(ctx context.Context, records []capturemodel.Record) error
	ListCaptures(ctx context.Context, query capturemodel.ListQuery) (capturemodel.ListResult, error)
	FindCaptureByUUID(ctx context.Context, id uuid.UUID) (capturemodel.Record, error)
}

type ObjectStorage interface {
	PutObject(ctx context.Context, object capturemodel.Object) error
	GetObject(ctx context.Context, bucket string, objectKey string, maxBytes int64) (capturemodel.Object, error)
	DeleteObject(ctx context.Context, bucket, objectKey string) error
}

type UUIDGenerator func() (uuid.UUID, error)

type Config struct {
	Bucket      string
	Limits      capturemodel.Limits
	JPEGQuality int
	NewUUID     UUIDGenerator
}

type Service struct {
	repository  Repository
	storage     ObjectStorage
	bucket      string
	limits      capturemodel.Limits
	jpegQuality int
	newUUID     UUIDGenerator
}

type preparedCapture struct {
	record capturemodel.Record
	body   []byte
}

type storedImage struct {
	body      []byte
	preserved bool
	quality   int
}

func NewService(repository Repository, storage ObjectStorage, cfg Config) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("captures repository is required")
	}
	if storage == nil {
		return nil, fmt.Errorf("captures object storage is required")
	}
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("captures bucket is required")
	}

	return &Service{
		repository:  repository,
		storage:     storage,
		bucket:      cfg.Bucket,
		limits:      normalizeLimits(cfg.Limits),
		jpegQuality: normalizeJPEGQuality(cfg.JPEGQuality),
		newUUID:     cfg.NewUUID,
	}, nil
}

func (svc *Service) CreateCaptures(
	ctx context.Context,
	files []capturemodel.UploadFile,
) ([]capturemodel.Response, error) {
	if err := svc.validateBatchSize(files); err != nil {
		return nil, err
	}

	prepared, err := svc.prepareBatch(files)
	if err != nil {
		return nil, err
	}

	uploaded, err := svc.uploadBatch(ctx, prepared)
	if err != nil {
		return nil, err
	}

	records := recordsFromPrepared(prepared)
	if err := svc.repository.InsertCaptures(ctx, records); err != nil {
		svc.cleanupUploaded(uploaded)
		return nil, newError(InternalError, "internal server error", err)
	}

	return responsesFromRecords(records), nil
}

func (svc *Service) ListCaptures(
	ctx context.Context,
	query capturemodel.ListQuery,
) (capturemodel.ListResponse, error) {
	query = normalizeListQuery(query)
	query.Type = capturemodel.TypeImage

	result, err := svc.repository.ListCaptures(ctx, query)
	if err != nil {
		return capturemodel.ListResponse{}, newError(InternalError, "internal server error", err)
	}

	return capturemodel.ListResponseFromRecords(result.Records, query, result.HasMore), nil
}

func (svc *Service) CaptureImage(ctx context.Context, id uuid.UUID) (capturemodel.Object, error) {
	record, err := svc.repository.FindCaptureByUUID(ctx, id)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return capturemodel.Object{}, newError(NotFound, "capture was not found", err)
	}
	if err != nil {
		return capturemodel.Object{}, newError(InternalError, "internal server error", err)
	}
	if record.ContentType != capturemodel.ContentTypeJPEG {
		return capturemodel.Object{}, newError(UnsupportedMediaType, "capture is not a supported image", nil)
	}

	object, err := svc.storage.GetObject(ctx, record.Bucket, record.ObjectKey, svc.limits.MaxFileBytes)
	if err != nil {
		return capturemodel.Object{}, newError(StorageError, "object storage read failed", err)
	}
	if object.ContentType == "" {
		object.ContentType = record.ContentType
	}

	return object, nil
}

func (svc *Service) validateBatchSize(files []capturemodel.UploadFile) error {
	if len(files) == 0 {
		return newError(InvalidRequest, "files field is required", nil)
	}
	if len(files) > svc.limits.MaxFiles {
		return newError(InvalidRequest, fmt.Sprintf("files field accepts at most %d files", svc.limits.MaxFiles), nil)
	}

	return nil
}

func (svc *Service) prepareBatch(files []capturemodel.UploadFile) ([]preparedCapture, error) {
	prepared := make([]preparedCapture, 0, len(files))
	for _, file := range files {
		capture, err := svc.prepareCapture(file)
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, capture)
	}

	return prepared, nil
}

func (svc *Service) prepareCapture(file capturemodel.UploadFile) (preparedCapture, error) {
	if file.SizeBytes <= 0 {
		file.SizeBytes = int64(len(file.Data))
	}
	if file.SizeBytes > svc.limits.MaxFileBytes || int64(len(file.Data)) > svc.limits.MaxFileBytes {
		return preparedCapture{}, newError(PayloadTooLarge, "file exceeds maximum size", nil)
	}

	declaredContentType, err := validateDeclaredContentType(file.ContentType)
	if err != nil {
		return preparedCapture{}, err
	}

	img, format, err := image.Decode(bytes.NewReader(file.Data))
	if err != nil {
		return preparedCapture{}, newError(InvalidRequest, "file cannot be decoded as an image", err)
	}

	actualContentType, ok := contentTypeByFormat[format]
	if !ok {
		return preparedCapture{}, newError(UnsupportedMediaType, "unsupported image format", nil)
	}
	if actualContentType != declaredContentType {
		return preparedCapture{}, newError(UnsupportedMediaType, "declared media type does not match image content", nil)
	}

	stored, err := svc.prepareStoredImage(format, img, file.Data)
	if err != nil {
		return preparedCapture{}, newError(InvalidRequest, "file cannot be converted to jpeg", err)
	}

	checksum := sha256.Sum256(stored.body)
	checksumHex := hex.EncodeToString(checksum[:])

	id, err := svc.captureUUID(checksumHex)
	if err != nil {
		return preparedCapture{}, newError(InternalError, "internal server error", err)
	}

	return svc.newPreparedCapture(file, id, format, img.Bounds(), stored, checksumHex), nil
}

func (svc *Service) prepareStoredImage(format string, img image.Image, original []byte) (storedImage, error) {
	if format == "jpeg" {
		body := append([]byte(nil), original...)

		return storedImage{body: body, preserved: true}, nil
	}

	body, err := encodeJPEG(img, svc.jpegQuality)
	if err != nil {
		return storedImage{}, err
	}

	return storedImage{body: body, quality: svc.jpegQuality}, nil
}

func (svc *Service) captureUUID(checksumHex string) (uuid.UUID, error) {
	if svc.newUUID != nil {
		return svc.newUUID()
	}

	namespace := uuid.MustParse(captureNamespace)

	return uuid.NewSHA1(namespace, []byte(captureUUIDPrefix+checksumHex)), nil
}

func (svc *Service) newPreparedCapture(
	file capturemodel.UploadFile,
	id uuid.UUID,
	format string,
	bounds image.Rectangle,
	stored storedImage,
	checksumHex string,
) preparedCapture {
	objectKey := fmt.Sprintf("captures/image/%s.jpg", id.String())

	record := capturemodel.Record{
		UUID:           id,
		Type:           capturemodel.TypeImage,
		Bucket:         svc.bucket,
		ObjectKey:      objectKey,
		ContentType:    capturemodel.ContentTypeJPEG,
		SizeBytes:      int64(len(stored.body)),
		ChecksumSHA256: checksumHex,
		Metadata: buildMetadata(file, format, dimensions{
			width:  bounds.Dx(),
			height: bounds.Dy(),
		}, len(stored.body), stored.quality, stored.preserved),
	}

	return preparedCapture{record: record, body: stored.body}
}

func (svc *Service) uploadBatch(ctx context.Context, prepared []preparedCapture) ([]preparedCapture, error) {
	uploaded := make([]preparedCapture, 0, len(prepared))
	for index := range prepared {
		capture := prepared[index]
		object := capture.record.Object()
		object.Body = capture.body

		if err := svc.storage.PutObject(ctx, object); err != nil {
			failedUploads := append([]preparedCapture{}, uploaded...)
			failedUploads = append(failedUploads, capture)
			svc.cleanupUploaded(failedUploads)
			return nil, newError(StorageError, "object storage upload failed", err)
		}

		uploaded = append(uploaded, capture)
	}

	return uploaded, nil
}

func (svc *Service) cleanupUploaded(captures []preparedCapture) {
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for index := range captures {
		capture := captures[index]
		_ = svc.storage.DeleteObject(cleanupCtx, capture.record.Bucket, capture.record.ObjectKey)
	}
}

func recordsFromPrepared(prepared []preparedCapture) []capturemodel.Record {
	records := make([]capturemodel.Record, 0, len(prepared))
	for index := range prepared {
		records = append(records, prepared[index].record)
	}

	return records
}

func responsesFromRecords(records []capturemodel.Record) []capturemodel.Response {
	responses := make([]capturemodel.Response, 0, len(records))
	for index := range records {
		responses = append(responses, records[index].Response())
	}

	return responses
}

func validateDeclaredContentType(raw string) (string, error) {
	if raw == "" {
		return "", newError(UnsupportedMediaType, "file content type is required", nil)
	}

	contentType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return "", newError(UnsupportedMediaType, "invalid file content type", err)
	}

	contentType = strings.ToLower(contentType)
	if _, ok := supportedContentTypes[contentType]; !ok {
		return "", newError(UnsupportedMediaType, "unsupported file content type", nil)
	}

	return contentType, nil
}

func encodeJPEG(img image.Image, quality int) ([]byte, error) {
	bounds := img.Bounds()
	canvas := image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(canvas, canvas.Bounds(), img, bounds.Min, draw.Over)

	var buffer bytes.Buffer
	if err := jpeg.Encode(&buffer, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode jpeg: %w", err)
	}

	return buffer.Bytes(), nil
}

func normalizeLimits(limits capturemodel.Limits) capturemodel.Limits {
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 10
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = 15 * 1024 * 1024
	}
	if limits.MaxRequestBytes <= 0 {
		limits.MaxRequestBytes = 150 * 1024 * 1024
	}

	return limits
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

func normalizeJPEGQuality(quality int) int {
	if quality <= 0 {
		return 95
	}
	if quality > 100 {
		return 100
	}

	return quality
}

var supportedContentTypes = map[string]struct{}{
	"image/jpeg": {},
	"image/png":  {},
	"image/webp": {},
}

var contentTypeByFormat = map[string]string{
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
}
