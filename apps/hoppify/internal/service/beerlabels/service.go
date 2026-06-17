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
	FindBeerLabelRecognition(ctx context.Context, captureID uuid.UUID) (beerlabelmodel.Record, error)
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

	cached, err := svc.recognitions.FindBeerLabelRecognition(ctx, captureID)
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
		cached, findErr := svc.recognitions.FindBeerLabelRecognition(ctx, captureID)
		if findErr == nil {
			return beerlabelmodel.ResponseFromRecord(&cached, true), nil
		}

		return beerlabelmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	saved, err := svc.recognitions.FindBeerLabelRecognition(ctx, captureID)
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
	if result.Evidence == nil {
		result.Evidence = []string{}
	}
	for index := range result.Evidence {
		result.Evidence[index] = strings.TrimSpace(result.Evidence[index])
	}
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
