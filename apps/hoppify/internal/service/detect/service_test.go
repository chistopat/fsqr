package detect

import (
	"context"
	"errors"
	"image"
	"os"
	"testing"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"

	"github.com/google/uuid"
)

const testCaptureUUID = "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"

func TestDetectReturnsUltralyticsStyleResponse(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	imageBody := readFixture(t)
	repository := &fakeRepository{record: newCaptureRecord(captureID)}
	storage := &fakeStorage{object: capturemodel.Object{
		Bucket:         "hoppify",
		ObjectKey:      "captures/image/" + testCaptureUUID + ".jpg",
		ContentType:    capturemodel.ContentTypeJPEG,
		ChecksumSHA256: "checksum",
		Body:           imageBody,
	}}
	detector := &fakeDetector{result: detectionmodel.ImageResult{
		Results: []detectionmodel.Detection{{
			Class:      0,
			Name:       "object",
			Confidence: 0.91,
			Box:        detectionmodel.Box{X1: 10, Y1: 20, X2: 30, Y2: 40},
		}},
		Speed: detectionmodel.Speed{Preprocess: 1, Inference: 2, Postprocess: 3},
	}}
	service := newTestService(t, repository, storage, detector)

	response, err := service.Detect(context.Background(), captureID.String())
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	if repository.requestedUUID != captureID {
		t.Fatalf("expected lookup uuid %s, got %s", captureID, repository.requestedUUID)
	}
	if storage.maxBytes != defaultMaxObjectBytes {
		t.Fatalf("expected storage max bytes %d, got %d", defaultMaxObjectBytes, storage.maxBytes)
	}
	if !detector.called {
		t.Fatalf("expected detector call")
	}
	if response.Metadata.UUID != captureID.String() {
		t.Fatalf("expected metadata uuid %q, got %q", captureID.String(), response.Metadata.UUID)
	}
	if response.Metadata.ImageCount != 1 {
		t.Fatalf("expected one image, got %d", response.Metadata.ImageCount)
	}
	if len(response.Images) != 1 {
		t.Fatalf("expected one image result, got %d", len(response.Images))
	}
	if response.Images[0].Shape != [2]int{1024, 768} {
		t.Fatalf("expected shape [1024 768], got %#v", response.Images[0].Shape)
	}
	if len(response.Images[0].Results) != 1 {
		t.Fatalf("expected one detection, got %d", len(response.Images[0].Results))
	}
	if response.Images[0].Results[0].Box.X2 != 30 {
		t.Fatalf("expected detection box to be returned, got %#v", response.Images[0].Results[0].Box)
	}
}

func TestDetectRejectsInvalidUUID(t *testing.T) {
	t.Parallel()

	service := newTestService(t, &fakeRepository{}, &fakeStorage{}, &fakeDetector{})

	_, err := service.Detect(context.Background(), "bad")

	assertDetectError(t, err, InvalidRequest)
}

func TestDetectReturnsNotFoundForMissingCapture(t *testing.T) {
	t.Parallel()

	service := newTestService(t, &fakeRepository{err: capturemodel.ErrNotFound}, &fakeStorage{}, &fakeDetector{})

	_, err := service.Detect(context.Background(), testCaptureUUID)

	assertDetectError(t, err, NotFound)
}

func TestDetectReturnsStorageErrorWhenObjectReadFails(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	service := newTestService(
		t,
		&fakeRepository{record: newCaptureRecord(captureID)},
		&fakeStorage{err: errors.New("s3 down")},
		&fakeDetector{},
	)

	_, err := service.Detect(context.Background(), testCaptureUUID)

	assertDetectError(t, err, StorageError)
}

func TestDetectReturnsInferenceErrorWhenDetectorFails(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	service := newTestService(
		t,
		&fakeRepository{record: newCaptureRecord(captureID)},
		&fakeStorage{object: capturemodel.Object{Body: readFixture(t)}},
		&fakeDetector{err: errors.New("onnx failed")},
	)

	_, err := service.Detect(context.Background(), testCaptureUUID)

	assertDetectError(t, err, InferenceError)
}

func newTestService(
	t *testing.T,
	repository Repository,
	storage ObjectStorage,
	detector Detector,
) *Service {
	t.Helper()

	service, err := NewService(repository, storage, detector, Config{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

func newCaptureRecord(captureID uuid.UUID) capturemodel.Record {
	return capturemodel.Record{
		UUID:           captureID,
		Type:           capturemodel.TypeImage,
		Bucket:         "hoppify",
		ObjectKey:      "captures/image/" + captureID.String() + ".jpg",
		ContentType:    capturemodel.ContentTypeJPEG,
		SizeBytes:      123,
		ChecksumSHA256: "checksum",
		Metadata:       map[string]any{},
	}
}

func readFixture(t *testing.T) []byte {
	t.Helper()

	body, err := os.ReadFile("../../../tests/fixtures/detect-shelf.jpg")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	return body
}

func assertDetectError(t *testing.T, err error, code ErrorCode) {
	t.Helper()

	var detectErr *Error
	if !errors.As(err, &detectErr) {
		t.Fatalf("expected detect error, got %v", err)
	}
	if detectErr.Code != code {
		t.Fatalf("expected error code %q, got %q", code, detectErr.Code)
	}
}

type fakeRepository struct {
	record        capturemodel.Record
	requestedUUID uuid.UUID
	err           error
}

func (repo *fakeRepository) FindCaptureByUUID(_ context.Context, id uuid.UUID) (capturemodel.Record, error) {
	repo.requestedUUID = id
	if repo.err != nil {
		return capturemodel.Record{}, repo.err
	}

	return repo.record, nil
}

type fakeStorage struct {
	object   capturemodel.Object
	maxBytes int64
	err      error
}

func (storage *fakeStorage) GetObject(
	_ context.Context,
	_ string,
	_ string,
	maxBytes int64,
) (capturemodel.Object, error) {
	storage.maxBytes = maxBytes
	if storage.err != nil {
		return capturemodel.Object{}, storage.err
	}

	return storage.object, nil
}

type fakeDetector struct {
	result detectionmodel.ImageResult
	called bool
	err    error
}

func (detector *fakeDetector) Detect(_ context.Context, _ image.Image) (detectionmodel.ImageResult, error) {
	detector.called = true
	if detector.err != nil {
		return detectionmodel.ImageResult{}, detector.err
	}

	return detector.result, nil
}
