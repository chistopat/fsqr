package beerlabels

import (
	"context"
	"errors"
	"testing"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
)

const testCaptureUUID = "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"

func TestServiceReturnsCachedRecognitionWithoutCallingModel(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	recognitions := &fakeRecognitionRepository{record: newRecognitionRecord(captureID)}
	storage := &fakeStorage{}
	recognizer := &fakeRecognizer{}
	service := newTestService(t, &fakeCaptureRepository{}, recognitions, storage, recognizer)

	response, err := service.Identify(context.Background(), testCaptureUUID)
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if !response.Cached {
		t.Fatalf("expected cached response")
	}
	if recognizer.calls != 0 {
		t.Fatalf("expected recognizer not to be called, got %d calls", recognizer.calls)
	}
	if storage.gets != 0 {
		t.Fatalf("expected storage not to be read, got %d reads", storage.gets)
	}
}

func TestServiceStoresRecognitionAndReusesIt(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	captures := &fakeCaptureRepository{record: capturemodel.Record{
		UUID:           captureID,
		Type:           capturemodel.TypeImageCrop,
		Bucket:         "hoppify",
		ObjectKey:      "captures/crops/parent/" + testCaptureUUID + ".jpg",
		ContentType:    capturemodel.ContentTypeJPEG,
		SizeBytes:      123,
		ChecksumSHA256: "checksum",
		Metadata:       map[string]any{},
	}}
	recognitions := &fakeRecognitionRepository{}
	storage := &fakeStorage{object: capturemodel.Object{Body: []byte("jpeg")}}
	recognizer := &fakeRecognizer{result: beerlabelmodel.Result{
		Status:     beerlabelmodel.StatusIdentified,
		Container:  beerlabelmodel.ContainerBottle,
		Confidence: 0.82,
		Evidence:   []string{"label text appears readable"},
	}}
	service := newTestService(t, captures, recognitions, storage, recognizer)

	first, err := service.Identify(context.Background(), testCaptureUUID)
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}
	second, err := service.Identify(context.Background(), testCaptureUUID)
	if err != nil {
		t.Fatalf("identify beer label again: %v", err)
	}

	if first.Cached {
		t.Fatalf("expected first response to come from model")
	}
	if !second.Cached {
		t.Fatalf("expected second response to come from cache")
	}
	if recognizer.calls != 1 {
		t.Fatalf("expected one recognizer call across retries, got %d", recognizer.calls)
	}
	if recognitions.inserted != 1 {
		t.Fatalf("expected one inserted recognition, got %d", recognitions.inserted)
	}
	if first.Result.Status != beerlabelmodel.StatusIdentified || second.Result.Status != first.Result.Status {
		t.Fatalf("unexpected recognition results: first=%#v second=%#v", first.Result, second.Result)
	}
}

func TestServiceMapsUnavailableRecognizer(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	service := newTestService(
		t,
		&fakeCaptureRepository{record: newCaptureRecord(captureID)},
		&fakeRecognitionRepository{},
		&fakeStorage{object: capturemodel.Object{Body: []byte("jpeg")}},
		&fakeRecognizer{err: ErrModelUnavailable},
	)

	_, err := service.Identify(context.Background(), testCaptureUUID)

	assertBeerLabelError(t, err, ModelUnavailable)
}

func newTestService(
	t *testing.T,
	captures CaptureRepository,
	recognitions RecognitionRepository,
	storage ObjectStorage,
	recognizer Recognizer,
) *Service {
	t.Helper()

	service, err := NewService(captures, recognitions, storage, recognizer, Config{})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

func newCaptureRecord(captureID uuid.UUID) capturemodel.Record {
	return capturemodel.Record{
		UUID:           captureID,
		Type:           capturemodel.TypeImageCrop,
		Bucket:         "hoppify",
		ObjectKey:      "captures/crops/parent/" + captureID.String() + ".jpg",
		ContentType:    capturemodel.ContentTypeJPEG,
		SizeBytes:      123,
		ChecksumSHA256: "checksum",
		Metadata:       map[string]any{},
	}
}

func newRecognitionRecord(captureID uuid.UUID) beerlabelmodel.Record {
	return beerlabelmodel.Record{
		CaptureUUID:   captureID,
		Model:         "chatgpt-5.4-mini",
		PromptVersion: "beer-label-v1",
		Result: beerlabelmodel.Result{
			Status:     beerlabelmodel.StatusUnreadable,
			Container:  beerlabelmodel.ContainerUnknown,
			Confidence: 0.1,
			Evidence:   []string{},
		},
		CreatedAt: time.Unix(1, 0).UTC(),
	}
}

func assertBeerLabelError(t *testing.T, err error, code ErrorCode) {
	t.Helper()

	var labelErr *Error
	if !errors.As(err, &labelErr) {
		t.Fatalf("expected beer label error, got %v", err)
	}
	if labelErr.Code != code {
		t.Fatalf("expected error code %q, got %q", code, labelErr.Code)
	}
}

type fakeCaptureRepository struct {
	record capturemodel.Record
	err    error
}

func (repo *fakeCaptureRepository) FindCaptureByUUID(_ context.Context, id uuid.UUID) (capturemodel.Record, error) {
	if repo.err != nil {
		return capturemodel.Record{}, repo.err
	}
	if repo.record.UUID == id {
		return repo.record, nil
	}

	return capturemodel.Record{}, capturemodel.ErrNotFound
}

type fakeRecognitionRepository struct {
	record   beerlabelmodel.Record
	inserted int
	err      error
}

func (repo *fakeRecognitionRepository) FindBeerLabelRecognition(
	_ context.Context,
	captureID uuid.UUID,
) (beerlabelmodel.Record, error) {
	if repo.err != nil {
		return beerlabelmodel.Record{}, repo.err
	}
	if repo.record.CaptureUUID == captureID {
		return repo.record, nil
	}

	return beerlabelmodel.Record{}, beerlabelmodel.ErrNotFound
}

func (repo *fakeRecognitionRepository) InsertBeerLabelRecognition(
	_ context.Context,
	record beerlabelmodel.Record,
) error {
	if repo.err != nil {
		return repo.err
	}
	repo.inserted++
	record.CreatedAt = time.Unix(2, 0).UTC()
	repo.record = record

	return nil
}

type fakeStorage struct {
	object capturemodel.Object
	gets   int
	err    error
}

func (storage *fakeStorage) GetObject(
	_ context.Context,
	_ string,
	_ string,
	_ int64,
) (capturemodel.Object, error) {
	storage.gets++
	if storage.err != nil {
		return capturemodel.Object{}, storage.err
	}

	return storage.object, nil
}

type fakeRecognizer struct {
	result beerlabelmodel.Result
	calls  int
	err    error
}

func (recognizer *fakeRecognizer) IdentifyBeerLabel(_ context.Context, _ []byte) (beerlabelmodel.Result, error) {
	recognizer.calls++
	if recognizer.err != nil {
		return beerlabelmodel.Result{}, recognizer.err
	}

	return recognizer.result, nil
}

func (recognizer *fakeRecognizer) Model() string {
	return "chatgpt-5.4-mini"
}

func (recognizer *fakeRecognizer) PromptVersion() string {
	return "beer-label-v1"
}
