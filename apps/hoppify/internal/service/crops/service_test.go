package crops

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"testing"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	cropmodel "github.com/chistopat/hoppify/internal/models/crop"

	"github.com/google/uuid"
)

const testParentUUID = "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"

func TestServiceCreatesImageCropsIdempotently(t *testing.T) {
	t.Parallel()

	parentID := uuid.MustParse(testParentUUID)
	repository := &fakeRepository{parent: newParentRecord(parentID)}
	storage := &fakeStorage{object: capturemodel.Object{
		Bucket:      "hoppify",
		ObjectKey:   "captures/image/" + testParentUUID + ".jpg",
		ContentType: capturemodel.ContentTypeJPEG,
		Body:        newJPEG(t),
	}}
	service := newTestService(t, repository, storage)
	request := cropmodel.Request{
		UUID: testParentUUID,
		Boxes: []cropmodel.BoxRequest{{
			BBox: []float64{1, 1, 3, 4},
		}},
	}

	first, err := service.CreateCrops(context.Background(), request)
	if err != nil {
		t.Fatalf("create crops: %v", err)
	}
	second, err := service.CreateCrops(context.Background(), request)
	if err != nil {
		t.Fatalf("create crops again: %v", err)
	}

	if len(first.Crops) != 1 {
		t.Fatalf("expected one crop, got %d", len(first.Crops))
	}
	if len(second.Crops) != 1 {
		t.Fatalf("expected one crop on retry, got %d", len(second.Crops))
	}
	if first.Crops[0] != second.Crops[0] {
		t.Fatalf("expected retry to return same crop, got first=%#v second=%#v", first.Crops[0], second.Crops[0])
	}
	if len(storage.puts) != 1 {
		t.Fatalf("expected one uploaded crop across retries, got %d", len(storage.puts))
	}
	if len(repository.children) != 1 {
		t.Fatalf("expected one inserted crop across retries, got %d", len(repository.children))
	}

	record := repository.children[0]
	if record.Type != capturemodel.TypeImageCrop {
		t.Fatalf("expected image_crop type, got %q", record.Type)
	}
	if !record.ParentUUID.Valid || record.ParentUUID.UUID != parentID {
		t.Fatalf("expected parent uuid %s, got %#v", parentID, record.ParentUUID)
	}
	expectedKey := "captures/crops/" + testParentUUID + "/" + record.UUID.String() + ".jpg"
	if record.ObjectKey != expectedKey {
		t.Fatalf("expected object key %q, got %q", expectedKey, record.ObjectKey)
	}

	cropped, _, err := image.Decode(bytes.NewReader(storage.puts[0].Body))
	if err != nil {
		t.Fatalf("decode uploaded crop: %v", err)
	}
	if cropped.Bounds().Dx() != 2 || cropped.Bounds().Dy() != 3 {
		t.Fatalf("expected crop dimensions 2x3, got %dx%d", cropped.Bounds().Dx(), cropped.Bounds().Dy())
	}
}

func TestServiceRejectsInvalidBoxes(t *testing.T) {
	t.Parallel()

	parentID := uuid.MustParse(testParentUUID)
	service := newTestService(
		t,
		&fakeRepository{parent: newParentRecord(parentID)},
		&fakeStorage{object: capturemodel.Object{Body: newJPEG(t)}},
	)

	_, err := service.CreateCrops(context.Background(), cropmodel.Request{
		UUID: testParentUUID,
		Boxes: []cropmodel.BoxRequest{{
			BBox: []float64{3, 3, 1, 1},
		}},
	})

	assertCropError(t, err, InvalidRequest)
}

func TestServiceReturnsNotFoundForMissingParent(t *testing.T) {
	t.Parallel()

	service := newTestService(t, &fakeRepository{err: capturemodel.ErrNotFound}, &fakeStorage{})

	_, err := service.CreateCrops(context.Background(), cropmodel.Request{
		UUID: testParentUUID,
		Boxes: []cropmodel.BoxRequest{{
			BBox: []float64{0, 0, 1, 1},
		}},
	})

	assertCropError(t, err, NotFound)
}

func newTestService(t *testing.T, repository Repository, storage ObjectStorage) *Service {
	t.Helper()

	service, err := NewService(repository, storage, Config{JPEGQuality: 95})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

func newParentRecord(parentID uuid.UUID) capturemodel.Record {
	return capturemodel.Record{
		UUID:           parentID,
		Type:           capturemodel.TypeImage,
		Bucket:         "hoppify",
		ObjectKey:      "captures/image/" + parentID.String() + ".jpg",
		ContentType:    capturemodel.ContentTypeJPEG,
		SizeBytes:      123,
		ChecksumSHA256: "checksum",
		Metadata:       map[string]any{},
	}
}

func newJPEG(t *testing.T) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := range 4 {
		for x := range 4 {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 180, A: 255})
		}
	}

	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return body.Bytes()
}

func assertCropError(t *testing.T, err error, code ErrorCode) {
	t.Helper()

	var cropErr *Error
	if !errors.As(err, &cropErr) {
		t.Fatalf("expected crop error, got %v", err)
	}
	if cropErr.Code != code {
		t.Fatalf("expected error code %q, got %q", code, cropErr.Code)
	}
}

type fakeRepository struct {
	parent   capturemodel.Record
	children []capturemodel.Record
	err      error
}

func (repo *fakeRepository) FindCaptureByUUID(_ context.Context, id uuid.UUID) (capturemodel.Record, error) {
	if repo.err != nil {
		return capturemodel.Record{}, repo.err
	}
	if repo.parent.UUID == id {
		return repo.parent, nil
	}
	for index := range repo.children {
		if repo.children[index].UUID == id {
			return repo.children[index], nil
		}
	}

	return capturemodel.Record{}, capturemodel.ErrNotFound
}

func (repo *fakeRepository) FindCapturesByParentUUID(
	_ context.Context,
	parentID uuid.UUID,
) ([]capturemodel.Record, error) {
	if repo.err != nil {
		return nil, repo.err
	}

	children := make([]capturemodel.Record, 0, len(repo.children))
	for index := range repo.children {
		if repo.children[index].ParentUUID.Valid && repo.children[index].ParentUUID.UUID == parentID {
			children = append(children, repo.children[index])
		}
	}

	return children, nil
}

func (repo *fakeRepository) InsertCaptures(_ context.Context, records []capturemodel.Record) error {
	repo.children = append(repo.children, records...)

	return nil
}

type fakeStorage struct {
	object capturemodel.Object
	puts   []capturemodel.Object
	err    error
}

func (storage *fakeStorage) GetObject(
	_ context.Context,
	_ string,
	_ string,
	_ int64,
) (capturemodel.Object, error) {
	if storage.err != nil {
		return capturemodel.Object{}, storage.err
	}

	return storage.object, nil
}

func (storage *fakeStorage) PutObject(_ context.Context, object capturemodel.Object) error {
	if storage.err != nil {
		return storage.err
	}
	storage.puts = append(storage.puts, object)

	return nil
}
