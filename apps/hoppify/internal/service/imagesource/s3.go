package imagesource

import (
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/google/uuid"
)

func ParseS3URI(raw string) (bucket, objectKey string, err error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", "", fmt.Errorf("s3 uri is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", "", fmt.Errorf("parse s3 uri: %w", err)
	}
	if parsed.Scheme != "s3" {
		return "", "", fmt.Errorf("uri must use s3 scheme")
	}
	if parsed.Host == "" {
		return "", "", fmt.Errorf("s3 uri bucket is required")
	}

	key := strings.TrimPrefix(parsed.Path, "/")
	if key == "" {
		return "", "", fmt.Errorf("s3 uri object key is required")
	}

	return parsed.Host, key, nil
}

func UUIDFromObjectKey(objectKey string) (uuid.UUID, bool) {
	filename := path.Base(objectKey)
	extension := path.Ext(filename)
	if extension != "" {
		filename = strings.TrimSuffix(filename, extension)
	}

	id, err := uuid.Parse(filename)
	if err != nil {
		return uuid.UUID{}, false
	}

	return id, true
}
