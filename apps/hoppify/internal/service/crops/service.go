package crops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"math"

	"github.com/chistopat/hoppify/internal/imageutil"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	cropmodel "github.com/chistopat/hoppify/internal/models/crop"

	"github.com/google/uuid"
)

const (
	defaultMaxObjectBytes = 15 * 1024 * 1024
	defaultJPEGQuality    = 95
	defaultMaxBoxes       = 300
	defaultListLimit      = 30
	maxListLimit          = 100
	cropNamespace         = "60c4f4c9-4c4c-4115-8914-9c1d7fe4a49e"
)

type Repository interface {
	FindCaptureByUUID(ctx context.Context, id uuid.UUID) (capturemodel.Record, error)
	FindCapturesByParentUUID(ctx context.Context, parentID uuid.UUID) ([]capturemodel.Record, error)
	InsertCaptures(ctx context.Context, records []capturemodel.Record) error
	ListCaptures(ctx context.Context, query capturemodel.ListQuery) (capturemodel.ListResult, error)
}

type ObjectStorage interface {
	GetObject(ctx context.Context, bucket string, objectKey string, maxBytes int64) (capturemodel.Object, error)
	PutObject(ctx context.Context, object capturemodel.Object) error
}

type Config struct {
	MaxObjectBytes int64
	JPEGQuality    int
	MaxBoxes       int
}

type Service struct {
	repository     Repository
	storage        ObjectStorage
	maxObjectBytes int64
	jpegQuality    int
	maxBoxes       int
	namespace      uuid.UUID
}

type cropSpec struct {
	id        uuid.UUID
	rect      image.Rectangle
	objectKey string
}

func NewService(repository Repository, storage ObjectStorage, cfg Config) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("crops repository is required")
	}
	if storage == nil {
		return nil, fmt.Errorf("crops object storage is required")
	}

	return &Service{
		repository:     repository,
		storage:        storage,
		maxObjectBytes: normalizeMaxObjectBytes(cfg.MaxObjectBytes),
		jpegQuality:    normalizeJPEGQuality(cfg.JPEGQuality),
		maxBoxes:       normalizeMaxBoxes(cfg.MaxBoxes),
		namespace:      uuid.MustParse(cropNamespace),
	}, nil
}

func (svc *Service) CreateCrops(ctx context.Context, request cropmodel.Request) (cropmodel.Response, error) {
	parentID, err := uuid.Parse(request.UUID)
	if err != nil {
		return cropmodel.Response{}, newError(InvalidRequest, "uuid must be a valid UUID", err)
	}
	if len(request.Boxes) == 0 {
		return cropmodel.Response{}, newError(InvalidRequest, "boxes field is required", nil)
	}
	if len(request.Boxes) > svc.maxBoxes {
		message := fmt.Sprintf("boxes field accepts at most %d boxes", svc.maxBoxes)
		return cropmodel.Response{}, newError(InvalidRequest, message, nil)
	}

	parent, err := svc.repository.FindCaptureByUUID(ctx, parentID)
	if errors.Is(err, capturemodel.ErrNotFound) {
		return cropmodel.Response{}, newError(NotFound, "capture was not found", err)
	}
	if err != nil {
		return cropmodel.Response{}, newError(InternalError, "internal server error", err)
	}
	if parent.ContentType != capturemodel.ContentTypeJPEG {
		return cropmodel.Response{}, newError(UnsupportedMediaType, "capture is not a supported image", nil)
	}

	object, err := svc.storage.GetObject(ctx, parent.Bucket, parent.ObjectKey, svc.maxObjectBytes)
	if err != nil {
		return cropmodel.Response{}, newError(StorageError, "object storage read failed", err)
	}

	img, err := decodeImage(object.Body)
	if err != nil {
		return cropmodel.Response{}, newError(InvalidRequest, "capture image cannot be decoded", err)
	}

	specs, err := svc.cropSpecs(parentID, img.Bounds(), request.Boxes)
	if err != nil {
		return cropmodel.Response{}, err
	}

	existing, err := svc.repository.FindCapturesByParentUUID(ctx, parentID)
	if err != nil {
		return cropmodel.Response{}, newError(InternalError, "internal server error", err)
	}
	if err := svc.createMissingCrops(ctx, &parent, img, specs, existing); err != nil {
		return cropmodel.Response{}, err
	}

	records, err := svc.repository.FindCapturesByParentUUID(ctx, parentID)
	if err != nil {
		return cropmodel.Response{}, newError(InternalError, "internal server error", err)
	}

	return buildResponse(parentID, specs, records)
}

func (svc *Service) ListCrops(
	ctx context.Context,
	query capturemodel.ListQuery,
) (cropmodel.ListResponse, error) {
	query = normalizeListQuery(query)
	query.Type = capturemodel.TypeImageCrop

	result, err := svc.repository.ListCaptures(ctx, query)
	if err != nil {
		return cropmodel.ListResponse{}, newError(InternalError, "internal server error", err)
	}

	return cropmodel.ListResponseFromRecords(result.Records, query, result.HasMore), nil
}

func (svc *Service) cropSpecs(
	parentID uuid.UUID,
	bounds image.Rectangle,
	boxes []cropmodel.BoxRequest,
) ([]cropSpec, error) {
	specs := make([]cropSpec, 0, len(boxes))
	for index := range boxes {
		rect, err := cropRect(bounds, boxes[index].BBox)
		if err != nil {
			return nil, newError(InvalidRequest, fmt.Sprintf("boxes[%d].bbox is invalid", index), err)
		}

		cropID := svc.cropUUID(parentID, rect, bounds)
		specs = append(specs, cropSpec{
			id:        cropID,
			rect:      rect,
			objectKey: cropObjectKey(parentID, cropID),
		})
	}

	return specs, nil
}

func (svc *Service) cropUUID(parentID uuid.UUID, rect, bounds image.Rectangle) uuid.UUID {
	data := fmt.Sprintf(
		"hoppify-crop-v1|%s|%d,%d,%d,%d",
		parentID.String(),
		rect.Min.X-bounds.Min.X,
		rect.Min.Y-bounds.Min.Y,
		rect.Max.X-bounds.Min.X,
		rect.Max.Y-bounds.Min.Y,
	)

	return uuid.NewSHA1(svc.namespace, []byte(data))
}

func (svc *Service) createMissingCrops(
	ctx context.Context,
	parent *capturemodel.Record,
	img image.Image,
	specs []cropSpec,
	existing []capturemodel.Record,
) error {
	existingByUUID := recordsByUUID(existing)
	prepared := make([]capturemodel.Record, 0, len(specs))
	objects := make([]capturemodel.Object, 0, len(specs))
	pending := make(map[uuid.UUID]struct{}, len(specs))

	for index := range specs {
		spec := specs[index]
		if _, ok := existingByUUID[spec.id]; ok {
			continue
		}
		if _, ok := pending[spec.id]; ok {
			continue
		}
		pending[spec.id] = struct{}{}

		body, err := encodeCropJPEG(img, spec.rect, svc.jpegQuality)
		if err != nil {
			return newError(InvalidRequest, "capture image cannot be cropped", err)
		}

		checksum := sha256.Sum256(body)
		checksumHex := hex.EncodeToString(checksum[:])
		record := capturemodel.Record{
			UUID: spec.id,
			ParentUUID: uuid.NullUUID{
				UUID:  parent.UUID,
				Valid: true,
			},
			Type:           capturemodel.TypeImageCrop,
			Bucket:         parent.Bucket,
			ObjectKey:      spec.objectKey,
			ContentType:    capturemodel.ContentTypeJPEG,
			SizeBytes:      int64(len(body)),
			ChecksumSHA256: checksumHex,
			Metadata:       map[string]any{},
		}
		prepared = append(prepared, record)
		objects = append(objects, capturemodel.Object{
			Bucket:         record.Bucket,
			ObjectKey:      record.ObjectKey,
			ContentType:    record.ContentType,
			ChecksumSHA256: record.ChecksumSHA256,
			Body:           body,
		})
	}
	if len(prepared) == 0 {
		return nil
	}

	for index := range objects {
		if err := svc.storage.PutObject(ctx, objects[index]); err != nil {
			return newError(StorageError, "object storage upload failed", err)
		}
	}
	if err := svc.repository.InsertCaptures(ctx, prepared); err != nil {
		records, findErr := svc.repository.FindCapturesByParentUUID(ctx, parent.UUID)
		if findErr == nil && specsExist(specs, records) {
			return nil
		}

		return newError(InternalError, "internal server error", err)
	}

	return nil
}

func buildResponse(
	parentID uuid.UUID,
	specs []cropSpec,
	records []capturemodel.Record,
) (cropmodel.Response, error) {
	byUUID := recordsByUUID(records)
	crops := make([]cropmodel.CropResponse, 0, len(specs))
	for index := range specs {
		record, ok := byUUID[specs[index].id]
		if !ok {
			return cropmodel.Response{}, newError(InternalError, "internal server error", nil)
		}
		crops = append(crops, cropmodel.ResponseFromCapture(&record))
	}

	return cropmodel.Response{UUID: parentID.String(), Crops: crops}, nil
}

func recordsByUUID(records []capturemodel.Record) map[uuid.UUID]capturemodel.Record {
	byUUID := make(map[uuid.UUID]capturemodel.Record, len(records))
	for index := range records {
		byUUID[records[index].UUID] = records[index]
	}

	return byUUID
}

func specsExist(specs []cropSpec, records []capturemodel.Record) bool {
	byUUID := recordsByUUID(records)
	for index := range specs {
		if _, ok := byUUID[specs[index].id]; !ok {
			return false
		}
	}

	return true
}

func cropObjectKey(parentID, cropID uuid.UUID) string {
	return fmt.Sprintf("captures/crops/%s/%s.jpg", parentID.String(), cropID.String())
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

func cropRect(bounds image.Rectangle, bbox []float64) (image.Rectangle, error) {
	if len(bbox) != 4 {
		return image.Rectangle{}, fmt.Errorf("bbox must contain four numbers")
	}
	for _, value := range bbox {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return image.Rectangle{}, fmt.Errorf("bbox contains non-finite coordinate")
		}
	}

	minX := clampFloat(bbox[0], float64(bounds.Min.X), float64(bounds.Max.X))
	minY := clampFloat(bbox[1], float64(bounds.Min.Y), float64(bounds.Max.Y))
	maxX := clampFloat(bbox[2], float64(bounds.Min.X), float64(bounds.Max.X))
	maxY := clampFloat(bbox[3], float64(bounds.Min.Y), float64(bounds.Max.Y))
	if maxX <= minX || maxY <= minY {
		return image.Rectangle{}, fmt.Errorf("bbox has no positive area")
	}

	rect := image.Rect(
		int(math.Floor(minX)),
		int(math.Floor(minY)),
		int(math.Ceil(maxX)),
		int(math.Ceil(maxY)),
	).Intersect(bounds)
	if rect.Empty() {
		return image.Rectangle{}, fmt.Errorf("bbox is outside image bounds")
	}

	return rect, nil
}

func encodeCropJPEG(img image.Image, rect image.Rectangle, quality int) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(dst, dst.Bounds(), img, rect.Min, draw.Src)

	var body bytes.Buffer
	if err := jpeg.Encode(&body, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encode crop jpeg: %w", err)
	}

	return body.Bytes(), nil
}

func decodeImage(body []byte) (image.Image, error) {
	img, _, err := imageutil.DecodeOriented(body)
	if err != nil {
		return nil, fmt.Errorf("decode oriented image: %w", err)
	}

	return img, nil
}

func clampFloat(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}

func normalizeMaxObjectBytes(maxObjectBytes int64) int64 {
	if maxObjectBytes <= 0 {
		return defaultMaxObjectBytes
	}

	return maxObjectBytes
}

func normalizeJPEGQuality(quality int) int {
	if quality <= 0 || quality > 100 {
		return defaultJPEGQuality
	}

	return quality
}

func normalizeMaxBoxes(maxBoxes int) int {
	if maxBoxes <= 0 {
		return defaultMaxBoxes
	}

	return maxBoxes
}
