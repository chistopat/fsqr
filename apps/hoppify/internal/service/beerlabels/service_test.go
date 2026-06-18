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
	recognitions := newFakeRecognitionRepository(newRecognitionRecord(captureID))
	storage := &fakeStorage{}
	recognizer := &fakeRecognizer{}
	service := newTestService(t, &fakeCaptureRepository{}, recognitions, storage, recognizer)

	response, err := service.Identify(context.Background(), beerlabelmodel.Request{UUID: testCaptureUUID})
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

func TestServiceCachesRecognitionByPromptVersion(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	captures := &fakeCaptureRepository{record: newCaptureRecord(captureID)}
	oldRecord := newRecognitionRecord(captureID)
	oldRecord.PromptVersion = "beer-label-v1"
	recognitions := newFakeRecognitionRepository(oldRecord)
	storage := &fakeStorage{object: capturemodel.Object{Body: []byte("jpeg")}}
	recognizer := &fakeRecognizer{
		promptVersion: "beer-label-v3-gemini-2.5-flash-lite",
		result: beerlabelmodel.Result{
			Status:     beerlabelmodel.StatusIdentified,
			Container:  beerlabelmodel.ContainerCan,
			Confidence: 0.9,
			Evidence:   []string{"gemini label"},
		},
	}
	service := newTestService(t, captures, recognitions, storage, recognizer)

	response, err := service.Identify(context.Background(), beerlabelmodel.Request{UUID: testCaptureUUID})
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if response.Cached {
		t.Fatalf("expected v3 response to bypass v1 cache")
	}
	if response.PromptVersion != "beer-label-v3-gemini-2.5-flash-lite" {
		t.Fatalf("expected v3 prompt version, got %q", response.PromptVersion)
	}
	if recognizer.calls != 1 {
		t.Fatalf("expected recognizer to be called, got %d calls", recognizer.calls)
	}
	if recognitions.inserted != 1 {
		t.Fatalf("expected one inserted recognition, got %d", recognitions.inserted)
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
	recognitions := newFakeRecognitionRepository()
	storage := &fakeStorage{object: capturemodel.Object{Body: []byte("jpeg")}}
	recognizer := &fakeRecognizer{result: beerlabelmodel.Result{
		Status:     beerlabelmodel.StatusIdentified,
		Container:  beerlabelmodel.ContainerBottle,
		Confidence: 0.82,
		Evidence:   []string{"label text appears readable"},
	}}
	service := newTestService(t, captures, recognitions, storage, recognizer)

	first, err := service.Identify(context.Background(), beerlabelmodel.Request{UUID: testCaptureUUID})
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}
	second, err := service.Identify(context.Background(), beerlabelmodel.Request{UUID: testCaptureUUID})
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
		newFakeRecognitionRepository(),
		&fakeStorage{object: capturemodel.Object{Body: []byte("jpeg")}},
		&fakeRecognizer{err: ErrModelUnavailable},
	)

	_, err := service.Identify(context.Background(), beerlabelmodel.Request{UUID: testCaptureUUID})

	assertBeerLabelError(t, err, ModelUnavailable)
}

func TestServiceReturnsCachedRecognitionForS3URL(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	captures := &fakeCaptureRepository{record: newCaptureRecord(captureID)}
	recognitions := newFakeRecognitionRepository(newRecognitionRecord(captureID))
	storage := &fakeStorage{}
	recognizer := &fakeRecognizer{}
	service := newTestService(t, captures, recognitions, storage, recognizer)

	response, err := service.Identify(context.Background(), beerlabelmodel.Request{
		URL: "s3://hoppify/captures/crops/parent/" + testCaptureUUID + ".jpg",
	})
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if !response.Cached {
		t.Fatalf("expected cached response")
	}
	if response.URL != "s3://hoppify/captures/crops/parent/"+testCaptureUUID+".jpg" {
		t.Fatalf("expected response url, got %q", response.URL)
	}
	if storage.gets != 0 {
		t.Fatalf("expected storage not to be read, got %d reads", storage.gets)
	}
	if recognizer.calls != 0 {
		t.Fatalf("expected recognizer not to be called, got %d calls", recognizer.calls)
	}
}

func TestServiceIdentifiesS3URLWithoutCaptureCache(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	captures := &fakeCaptureRepository{record: capturemodel.Record{}}
	recognitions := newFakeRecognitionRepository()
	storage := &fakeStorage{object: capturemodel.Object{
		ContentType: capturemodel.ContentTypeJPEG,
		Body:        []byte("jpeg"),
	}}
	recognizer := &fakeRecognizer{result: beerlabelmodel.Result{
		Status:     beerlabelmodel.StatusIdentified,
		Container:  beerlabelmodel.ContainerBottle,
		Confidence: 0.82,
		Evidence:   []string{"label text appears readable"},
	}}
	service := newTestService(t, captures, recognitions, storage, recognizer)

	response, err := service.Identify(context.Background(), beerlabelmodel.Request{
		URL: "s3://external/images/" + captureID.String() + ".jpg",
	})
	if err != nil {
		t.Fatalf("identify beer label: %v", err)
	}

	if response.Cached {
		t.Fatalf("expected uncached response")
	}
	if response.UUID != captureID.String() {
		t.Fatalf("expected uuid from s3 key, got %q", response.UUID)
	}
	if response.URL != "s3://external/images/"+captureID.String()+".jpg" {
		t.Fatalf("expected response url, got %q", response.URL)
	}
	if recognitions.inserted != 0 {
		t.Fatalf("expected recognition not to be cached, got %d inserts", recognitions.inserted)
	}
	if storage.gets != 1 || storage.bucket != "external" || storage.objectKey != "images/"+captureID.String()+".jpg" {
		t.Fatalf("expected one s3 read, got %d reads for %s/%s", storage.gets, storage.bucket, storage.objectKey)
	}
}

func TestServiceListsRecognitions(t *testing.T) {
	t.Parallel()

	captureID := uuid.MustParse(testCaptureUUID)
	recognitions := newFakeRecognitionRepository()
	recognitions.listResult = beerlabelmodel.ListResult{
		Records: []beerlabelmodel.ListRecord{{
			Crop:        newCaptureRecord(captureID),
			Recognition: newRecognitionRecord(captureID),
		}},
		HasMore: true,
	}
	service := newTestService(t, &fakeCaptureRepository{}, recognitions, &fakeStorage{}, &fakeRecognizer{})

	response, err := service.ListRecognitions(context.Background(), capturemodel.ListQuery{Limit: 0, Offset: -1})
	if err != nil {
		t.Fatalf("list recognitions: %v", err)
	}

	if recognitions.listQuery != (capturemodel.ListQuery{Limit: defaultListLimit, Offset: 0}) {
		t.Fatalf("unexpected list query: %#v", recognitions.listQuery)
	}
	if len(response.Recognitions) != 1 || response.Recognitions[0].Crop.ImageURL == "" || !response.HasMore {
		t.Fatalf("unexpected recognition list response: %#v", response)
	}
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
		Model:         "gemini-2.5-flash-lite",
		PromptVersion: "beer-label-v3-gemini-2.5-flash-lite",
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
	records    map[string]beerlabelmodel.Record
	listResult beerlabelmodel.ListResult
	listQuery  capturemodel.ListQuery
	inserted   int
	err        error
}

func newFakeRecognitionRepository(records ...beerlabelmodel.Record) *fakeRecognitionRepository {
	repo := &fakeRecognitionRepository{records: map[string]beerlabelmodel.Record{}}
	for _, record := range records {
		repo.records[recognitionKey(record.CaptureUUID, record.PromptVersion)] = record
	}

	return repo
}

func (repo *fakeRecognitionRepository) FindBeerLabelRecognition(
	_ context.Context,
	captureID uuid.UUID,
	promptVersion string,
) (beerlabelmodel.Record, error) {
	if repo.err != nil {
		return beerlabelmodel.Record{}, repo.err
	}
	if record, ok := repo.records[recognitionKey(captureID, promptVersion)]; ok {
		return record, nil
	}

	return beerlabelmodel.Record{}, beerlabelmodel.ErrNotFound
}

func (repo *fakeRecognitionRepository) InsertBeerLabelRecognition(
	_ context.Context,
	record *beerlabelmodel.Record,
) error {
	if repo.err != nil {
		return repo.err
	}
	repo.inserted++
	record.CreatedAt = time.Unix(2, 0).UTC()
	repo.records[recognitionKey(record.CaptureUUID, record.PromptVersion)] = *record

	return nil
}

func (repo *fakeRecognitionRepository) ListBeerLabelRecognitions(
	_ context.Context,
	query capturemodel.ListQuery,
) (beerlabelmodel.ListResult, error) {
	repo.listQuery = query
	if repo.err != nil {
		return beerlabelmodel.ListResult{}, repo.err
	}

	return repo.listResult, nil
}

func recognitionKey(captureID uuid.UUID, promptVersion string) string {
	return captureID.String() + ":" + promptVersion
}

type fakeStorage struct {
	object    capturemodel.Object
	gets      int
	bucket    string
	objectKey string
	err       error
}

func (storage *fakeStorage) GetObject(
	_ context.Context,
	bucket string,
	objectKey string,
	_ int64,
) (capturemodel.Object, error) {
	storage.gets++
	storage.bucket = bucket
	storage.objectKey = objectKey
	if storage.err != nil {
		return capturemodel.Object{}, storage.err
	}

	return storage.object, nil
}

type fakeRecognizer struct {
	result        beerlabelmodel.Result
	promptVersion string
	calls         int
	err           error
}

func (recognizer *fakeRecognizer) IdentifyBeerLabel(_ context.Context, _ []byte) (beerlabelmodel.Result, error) {
	recognizer.calls++
	if recognizer.err != nil {
		return beerlabelmodel.Result{}, recognizer.err
	}

	return recognizer.result, nil
}

func (recognizer *fakeRecognizer) Model() string {
	return "gemini-2.5-flash-lite"
}

func (recognizer *fakeRecognizer) PromptVersion() string {
	if recognizer.promptVersion != "" {
		return recognizer.promptVersion
	}

	return "beer-label-v3-gemini-2.5-flash-lite"
}
