package capture

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	TypeImage       = "image"
	ContentTypeJPEG = "image/jpeg"
)

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
	Type           string
	Bucket         string
	ObjectKey      string
	ContentType    string
	SizeBytes      int64
	ChecksumSHA256 string
	Metadata       map[string]any
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
}

type CapturesResponse struct {
	Captures []Response `json:"captures"`
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

func (record *Record) Response() Response {
	return Response{
		UUID: record.UUID.String(),
		Type: record.Type,
		URI:  URI(record.Bucket, record.ObjectKey),
	}
}

func URI(bucket, objectKey string) string {
	return fmt.Sprintf("s3://%s/%s", bucket, objectKey)
}
