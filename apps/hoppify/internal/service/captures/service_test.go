package captures

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
)

func TestServiceCreatesCapturesAllOrNothing(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	storage := &fakeStorage{}
	service := newTestService(t, repository, storage)
	files := []capturemodel.UploadFile{newPNGUploadFile(t, "one.png"), newJPEGUploadFile(t, "two.jpg")}

	captures, err := service.CreateCaptures(context.Background(), files)
	if err != nil {
		t.Fatalf("create captures: %v", err)
	}

	if len(captures) != 2 {
		t.Fatalf("expected two captures, got %d", len(captures))
	}
	if len(storage.puts) != 2 {
		t.Fatalf("expected two uploads, got %d", len(storage.puts))
	}
	if len(repository.records) != 2 {
		t.Fatalf("expected two inserted records, got %d", len(repository.records))
	}

	record := repository.records[0]
	if record.Type != capturemodel.TypeImage {
		t.Fatalf("expected image type, got %q", record.Type)
	}
	if record.ContentType != capturemodel.ContentTypeJPEG {
		t.Fatalf("expected jpeg content type, got %q", record.ContentType)
	}
	if record.ObjectKey != "captures/image/018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1.jpg" {
		t.Fatalf("unexpected object key: %q", record.ObjectKey)
	}
	if record.ChecksumSHA256 == "" {
		t.Fatalf("expected checksum to be set")
	}
	if storage.puts[0].ContentType != capturemodel.ContentTypeJPEG {
		t.Fatalf("expected uploaded jpeg content type, got %q", storage.puts[0].ContentType)
	}
	if _, _, err := image.Decode(bytes.NewReader(storage.puts[0].Body)); err != nil {
		t.Fatalf("uploaded object is not a decodable image: %v", err)
	}
}

func TestServiceValidatesBatchBeforeUpload(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	storage := &fakeStorage{}
	service := newTestService(t, repository, storage)
	files := []capturemodel.UploadFile{
		newPNGUploadFile(t, "one.png"),
		{
			Filename:    "broken.jpg",
			ContentType: "image/jpeg",
			SizeBytes:   4,
			Data:        []byte("nope"),
		},
	}

	_, err := service.CreateCaptures(context.Background(), files)

	var captureErr *Error
	if !errors.As(err, &captureErr) || captureErr.Code != InvalidRequest {
		t.Fatalf("expected invalid_request, got %v", err)
	}
	if len(storage.puts) != 0 {
		t.Fatalf("expected no uploads before full validation, got %d", len(storage.puts))
	}
	if len(repository.records) != 0 {
		t.Fatalf("expected no inserts before full validation, got %d", len(repository.records))
	}
}

func TestServiceDeletesUploadedObjectsWhenUploadFails(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{}
	storage := &fakeStorage{failPutAt: 2, putErr: errors.New("s3 down")}
	service := newTestService(t, repository, storage)

	_, err := service.CreateCaptures(context.Background(), []capturemodel.UploadFile{
		newPNGUploadFile(t, "one.png"),
		newJPEGUploadFile(t, "two.jpg"),
	})

	var captureErr *Error
	if !errors.As(err, &captureErr) || captureErr.Code != StorageError {
		t.Fatalf("expected storage_error, got %v", err)
	}
	if len(repository.records) != 0 {
		t.Fatalf("expected no inserts after storage failure, got %d", len(repository.records))
	}
	if len(storage.deletes) == 0 {
		t.Fatalf("expected uploaded objects to be deleted")
	}
}

func TestServiceDeletesUploadedObjectsWhenInsertFails(t *testing.T) {
	t.Parallel()

	repository := &fakeRepository{err: errors.New("insert failed")}
	storage := &fakeStorage{}
	service := newTestService(t, repository, storage)

	_, err := service.CreateCaptures(context.Background(), []capturemodel.UploadFile{
		newPNGUploadFile(t, "one.png"),
		newJPEGUploadFile(t, "two.jpg"),
	})

	var captureErr *Error
	if !errors.As(err, &captureErr) || captureErr.Code != InternalError {
		t.Fatalf("expected internal_error, got %v", err)
	}
	if len(storage.deletes) != 2 {
		t.Fatalf("expected both uploaded objects to be deleted, got %d", len(storage.deletes))
	}
}

func newTestService(t *testing.T, repository Repository, storage ObjectStorage) *Service {
	t.Helper()

	ids := []uuid.UUID{
		uuid.MustParse("018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"),
		uuid.MustParse("018f7b8e-4d96-7b42-9f64-09e5d3a8e7c2"),
	}
	nextID := 0

	service, err := NewService(repository, storage, Config{
		Bucket: "hoppify",
		Limits: capturemodel.Limits{
			MaxFiles:        10,
			MaxFileBytes:    1024 * 1024,
			MaxRequestBytes: 10 * 1024 * 1024,
		},
		JPEGQuality: 95,
		NewUUID: func() (uuid.UUID, error) {
			id := ids[nextID]
			nextID++
			return id, nil
		},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

func newPNGUploadFile(t *testing.T, filename string) capturemodel.UploadFile {
	t.Helper()

	var body bytes.Buffer
	if err := png.Encode(&body, testImage()); err != nil {
		t.Fatalf("encode png: %v", err)
	}

	return capturemodel.UploadFile{
		Filename:    filename,
		ContentType: "image/png",
		SizeBytes:   int64(body.Len()),
		Data:        body.Bytes(),
	}
}

func newJPEGUploadFile(t *testing.T, filename string) capturemodel.UploadFile {
	t.Helper()

	var body bytes.Buffer
	if err := jpeg.Encode(&body, testImage(), &jpeg.Options{Quality: 90}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}

	return capturemodel.UploadFile{
		Filename:    filename,
		ContentType: "image/jpeg",
		SizeBytes:   int64(body.Len()),
		Data:        body.Bytes(),
	}
}

func testImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{G: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{R: 255, G: 255, A: 255})

	return img
}

type fakeRepository struct {
	records []capturemodel.Record
	err     error
}

func (repo *fakeRepository) InsertCaptures(_ context.Context, records []capturemodel.Record) error {
	if repo.err != nil {
		return repo.err
	}
	repo.records = append(repo.records, records...)

	return nil
}

type fakeStorage struct {
	puts      []capturemodel.Object
	deletes   []string
	failPutAt int
	putErr    error
}

func (storage *fakeStorage) PutObject(_ context.Context, object capturemodel.Object) error {
	if storage.failPutAt > 0 && len(storage.puts)+1 == storage.failPutAt {
		return storage.putErr
	}
	storage.puts = append(storage.puts, object)

	return nil
}

func (storage *fakeStorage) DeleteObject(_ context.Context, bucket string, objectKey string) error {
	storage.deletes = append(storage.deletes, capturemodel.URI(bucket, objectKey))

	return nil
}
