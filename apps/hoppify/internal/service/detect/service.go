package detect

import (
	"context"
	"errors"
	"fmt"
	"image"
	"mime"
	"strings"
	"time"

	"github.com/chistopat/hoppify/internal/imageutil"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
	"github.com/chistopat/hoppify/internal/service/imagesource"

	"github.com/google/uuid"
)

const defaultMaxObjectBytes = 15 * 1024 * 1024

type Repository interface {
	FindCaptureByUUID(ctx context.Context, id uuid.UUID) (capturemodel.Record, error)
}

type ObjectStorage interface {
	GetObject(ctx context.Context, bucket string, objectKey string, maxBytes int64) (capturemodel.Object, error)
}

type Detector interface {
	Detect(ctx context.Context, img image.Image) (detectionmodel.ImageResult, error)
}

type Config struct {
	MaxObjectBytes int64
}

type Service struct {
	repository     Repository
	storage        ObjectStorage
	detector       Detector
	maxObjectBytes int64
}

type imageSource struct {
	body []byte
	uuid string
	url  string
}

func NewService(repository Repository, storage ObjectStorage, detector Detector, cfg Config) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("detect repository is required")
	}
	if storage == nil {
		return nil, fmt.Errorf("detect object storage is required")
	}
	if detector == nil {
		return nil, fmt.Errorf("detect detector is required")
	}

	return &Service{
		repository:     repository,
		storage:        storage,
		detector:       detector,
		maxObjectBytes: normalizeMaxObjectBytes(cfg.MaxObjectBytes),
	}, nil
}

func (svc *Service) Detect(ctx context.Context, request detectionmodel.Request) (detectionmodel.Response, error) {
	started := time.Now()

	source, err := svc.resolveImageSource(ctx, request)
	if err != nil {
		return detectionmodel.Response{}, err
	}

	img, err := decodeImage(source.body)
	if err != nil {
		return detectionmodel.Response{}, newError(InvalidRequest, "image cannot be decoded", err)
	}

	imageResult, err := svc.detector.Detect(ctx, img)
	if errors.Is(err, ErrModelUnavailable) {
		return detectionmodel.Response{}, newError(ModelUnavailable, "detection model is unavailable", err)
	}
	if err != nil {
		return detectionmodel.Response{}, newError(InferenceError, "object detection failed", err)
	}
	if imageResult.Shape == [2]int{} {
		imageResult.Shape = imageShape(img)
	}

	return detectionmodel.Response{
		Images: []detectionmodel.ImageResult{imageResult},
		Metadata: detectionmodel.Metadata{
			UUID:             source.uuid,
			URL:              source.url,
			ImageCount:       1,
			FunctionTimeCall: secondsSince(started),
		},
	}, nil
}

func (svc *Service) resolveImageSource(
	ctx context.Context,
	request detectionmodel.Request,
) (imageSource, error) {
	if err := validateRequestSource(request); err != nil {
		return imageSource{}, err
	}
	if request.UUID != "" {
		return svc.resolveCaptureSource(ctx, request.UUID)
	}
	if request.ImageURL() != "" {
		return svc.resolveS3Source(ctx, request.ImageURL())
	}
	if request.File != nil {
		return svc.resolveFileSource(request.File)
	}

	return imageSource{}, newError(InvalidRequest, "image source is required", nil)
}

func (svc *Service) resolveCaptureSource(ctx context.Context, rawUUID string) (imageSource, error) {
	captureID, err := uuid.Parse(strings.TrimSpace(rawUUID))
	if err != nil {
		return imageSource{}, newError(InvalidRequest, "uuid must be a valid UUID", err)
	}

	record, err := svc.repository.FindCaptureByUUID(ctx, captureID)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return imageSource{}, newError(NotFound, "capture was not found", err)
	}
	if err != nil {
		return imageSource{}, newError(InternalError, "internal server error", err)
	}
	if record.Type != capturemodel.TypeImage || record.ContentType != capturemodel.ContentTypeJPEG {
		return imageSource{}, newError(UnsupportedMediaType, "capture is not a supported image", nil)
	}

	object, err := svc.storage.GetObject(ctx, record.Bucket, record.ObjectKey, svc.maxObjectBytes)
	if err != nil {
		return imageSource{}, newError(StorageError, "object storage read failed", err)
	}

	return imageSource{
		body: object.Body,
		uuid: captureID.String(),
		url:  capturemodel.URI(record.Bucket, record.ObjectKey),
	}, nil
}

func (svc *Service) resolveS3Source(ctx context.Context, rawURL string) (imageSource, error) {
	bucket, objectKey, err := imagesource.ParseS3URI(rawURL)
	if err != nil {
		return imageSource{}, newError(InvalidRequest, "url must be an s3 uri", err)
	}

	object, err := svc.storage.GetObject(ctx, bucket, objectKey, svc.maxObjectBytes)
	if err != nil {
		return imageSource{}, newError(StorageError, "object storage read failed", err)
	}
	if err := validateOptionalImageContentType(object.ContentType); err != nil {
		return imageSource{}, err
	}

	source := imageSource{
		body: object.Body,
		url:  capturemodel.URI(bucket, objectKey),
	}
	if id, ok := imagesource.UUIDFromObjectKey(objectKey); ok {
		source.uuid = id.String()
	}

	return source, nil
}

func (svc *Service) resolveFileSource(file *capturemodel.UploadFile) (imageSource, error) {
	if file.SizeBytes <= 0 {
		file.SizeBytes = int64(len(file.Data))
	}
	if file.SizeBytes > svc.maxObjectBytes || int64(len(file.Data)) > svc.maxObjectBytes {
		return imageSource{}, newError(InvalidRequest, "file exceeds maximum size", nil)
	}
	if err := validateRequiredImageContentType(file.ContentType); err != nil {
		return imageSource{}, err
	}

	return imageSource{body: append([]byte(nil), file.Data...)}, nil
}

func decodeImage(body []byte) (image.Image, error) {
	img, _, err := imageutil.DecodeOriented(body)
	if err != nil {
		return nil, err
	}

	return img, nil
}

func imageShape(img image.Image) [2]int {
	bounds := img.Bounds()

	return [2]int{bounds.Dy(), bounds.Dx()}
}

func secondsSince(started time.Time) float64 {
	return float64(time.Since(started).Microseconds()) / 1_000_000
}

func normalizeMaxObjectBytes(maxObjectBytes int64) int64 {
	if maxObjectBytes <= 0 {
		return defaultMaxObjectBytes
	}

	return maxObjectBytes
}

func validateRequestSource(request detectionmodel.Request) error {
	sourceCount := 0
	if strings.TrimSpace(request.UUID) != "" {
		sourceCount++
	}
	if strings.TrimSpace(request.ImageURL()) != "" {
		sourceCount++
	}
	if request.File != nil {
		sourceCount++
	}
	if sourceCount != 1 {
		return newError(InvalidRequest, "exactly one image source is required", nil)
	}

	return nil
}

func validateRequiredImageContentType(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return newError(UnsupportedMediaType, "file content type is required", nil)
	}

	return validateOptionalImageContentType(raw)
}

func validateOptionalImageContentType(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}

	contentType, _, err := mime.ParseMediaType(raw)
	if err != nil {
		return newError(UnsupportedMediaType, "invalid image content type", err)
	}
	if _, ok := supportedImageContentTypes[strings.ToLower(contentType)]; !ok {
		return newError(UnsupportedMediaType, "unsupported image content type", nil)
	}

	return nil
}

var supportedImageContentTypes = map[string]struct{}{
	capturemodel.ContentTypeJPEG: {},
	"image/png":                  {},
	"image/webp":                 {},
}

type unavailableDetector struct {
	err error
}

func NewUnavailableDetector(err error) Detector {
	return &unavailableDetector{err: err}
}

func (detector *unavailableDetector) Detect(
	_ context.Context,
	_ image.Image,
) (detectionmodel.ImageResult, error) {
	if detector.err == nil {
		return detectionmodel.ImageResult{}, ErrModelUnavailable
	}

	return detectionmodel.ImageResult{}, fmt.Errorf("%w: %w", ErrModelUnavailable, detector.err)
}
