package capture

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	TypeImage       = "image"
	TypeImageCrop   = "image_crop"
	ContentTypeJPEG = "image/jpeg"
)

var ErrNotFound = errors.New("capture not found")

type Limits struct {
	MaxFiles        int
	MaxFileBytes    int64
	MaxRequestBytes int64
}

type UploadFile struct {
	Filename    string
	ContentType string
	SizeBytes   int64
	Data        []byte
}

type Record struct {
	UUID           uuid.UUID
	ParentUUID     uuid.NullUUID
	Type           string
	Bucket         string
	ObjectKey      string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	Metadata       map[string]any
	CreatedAt      time.Time
}

type Object struct {
	Bucket         string
	ObjectKey      string
	ContentType    string
	ChecksumSHA256 string
	Body           []byte
}

type Response struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	URI  string `json:"uri"`
	URL  string `json:"url,omitempty"`
}

type CapturesResponse struct {
	Captures []Response `json:"captures"`
}

type ListQuery struct {
	Limit  int
	Offset int
}

type ListResult struct {
	Records []Record
	HasMore bool
}

type ListResponse struct {
	Captures   []ListItem `json:"captures"`
	Limit      int        `json:"limit"`
	Offset     int        `json:"offset"`
	NextOffset int        `json:"nextOffset,omitempty"`
	HasMore    bool       `json:"hasMore"`
}

type ListItem struct {
	UUID              string    `json:"uuid"`
	Type              string    `json:"type"`
	URI               string    `json:"uri"`
	ImageURL          string    `json:"imageUrl"`
	CreatedAt         time.Time `json:"createdAt"`
	SizeBytes         int64     `json:"sizeBytes"`
	Width             int       `json:"width,omitempty"`
	Height            int       `json:"height,omitempty"`
	OriginalFilename  string    `json:"originalFilename,omitempty"`
	OriginalSizeBytes int64     `json:"originalSizeBytes,omitempty"`
}

type Stats struct {
	ImageCount          int64
	ImageSizeBytesTotal int64
}

func (record *Record) Object() Object {
	return Object{
		Bucket:         record.Bucket,
		ObjectKey:      record.ObjectKey,
		ContentType:    record.ContentType,
		ChecksumSHA256: record.ChecksumSHA256,
	}
}

func ListResponseFromRecords(records []Record, query ListQuery, hasMore bool) ListResponse {
	captures := make([]ListItem, 0, len(records))
	for index := range records {
		captures = append(captures, records[index].ListItem())
	}

	response := ListResponse{
		Captures: captures,
		Limit:    query.Limit,
		Offset:   query.Offset,
		HasMore:  hasMore,
	}
	if hasMore {
		response.NextOffset = query.Offset + len(records)
	}

	return response
}

func (record *Record) ListItem() ListItem {
	uri := URI(record.Bucket, record.ObjectKey)
	width, height := dimensionsFromMetadata(record.Metadata)

	return ListItem{
		UUID:              record.UUID.String(),
		Type:              record.Type,
		URI:               uri,
		ImageURL:          fmt.Sprintf("/api/v1/captures/%s/image", record.UUID.String()),
		CreatedAt:         record.CreatedAt,
		SizeBytes:         record.SizeBytes,
		Width:             width,
		Height:            height,
		OriginalFilename:  stringFromMetadata(record.Metadata, "original", "filename"),
		OriginalSizeBytes: int64FromMetadata(record.Metadata, "original", "size_bytes"),
	}
}

func (record *Record) Response() Response {
	uri := URI(record.Bucket, record.ObjectKey)

	return Response{
		UUID: record.UUID.String(),
		Type: record.Type,
		URI:  uri,
		URL:  uri,
	}
}

func URI(bucket, objectKey string) string {
	return fmt.Sprintf("s3://%s/%s", bucket, objectKey)
}

func dimensionsFromMetadata(metadata map[string]any) (width, height int) {
	return intFromMetadata(metadata, "dimensions", "width"), intFromMetadata(metadata, "dimensions", "height")
}

func stringFromMetadata(metadata map[string]any, keys ...string) string {
	value, ok := nestedMetadata(metadata, keys...)
	if !ok {
		return ""
	}

	text, _ := value.(string)

	return text
}

func intFromMetadata(metadata map[string]any, keys ...string) int {
	value := int64FromMetadata(metadata, keys...)
	if value <= 0 {
		return 0
	}

	return int(value)
}

func int64FromMetadata(metadata map[string]any, keys ...string) int64 {
	value, ok := nestedMetadata(metadata, keys...)
	if !ok {
		return 0
	}

	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func nestedMetadata(metadata map[string]any, keys ...string) (any, bool) {
	current := any(metadata)
	for _, key := range keys {
		asMap, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = asMap[key]
		if !ok {
			return nil, false
		}
	}

	return current, true
}
