package detect

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"

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

func (svc *Service) Detect(ctx context.Context, rawUUID string) (detectionmodel.Response, error) {
	started := time.Now()

	captureID, err := uuid.Parse(rawUUID)
	if err != nil {
		return detectionmodel.Response{}, newError(InvalidRequest, "uuid must be a valid UUID", err)
	}

	record, err := svc.repository.FindCaptureByUUID(ctx, captureID)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return detectionmodel.Response{}, newError(NotFound, "capture was not found", err)
	}
	if err != nil {
		return detectionmodel.Response{}, newError(InternalError, "internal server error", err)
	}
	if record.Type != capturemodel.TypeImage || record.ContentType != capturemodel.ContentTypeJPEG {
		return detectionmodel.Response{}, newError(UnsupportedMediaType, "capture is not a supported image", nil)
	}

	object, err := svc.storage.GetObject(ctx, record.Bucket, record.ObjectKey, svc.maxObjectBytes)
	if err != nil {
		return detectionmodel.Response{}, newError(StorageError, "object storage read failed", err)
	}

	img, err := decodeImage(object.Body)
	if err != nil {
		return detectionmodel.Response{}, newError(InvalidRequest, "capture image cannot be decoded", err)
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
			UUID:             captureID.String(),
			ImageCount:       1,
			FunctionTimeCall: secondsSince(started),
		},
	}, nil
}

func decodeImage(body []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
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
