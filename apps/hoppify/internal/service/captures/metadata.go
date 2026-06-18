package captures

import (
	"bytes"
	"strings"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/rwcarlsen/goexif/exif"
)

type dimensions struct {
	width  int
	height int
}

func buildMetadata(
	file capturemodel.UploadFile,
	format string,
	dims dimensions,
	storedSize int,
	quality int,
	preserved bool,
) map[string]any {
	normalized := map[string]any{
		"content_type":       capturemodel.ContentTypeJPEG,
		"format":             "jpeg",
		"preserved_original": preserved,
		"size_bytes":         storedSize,
	}
	if !preserved {
		normalized["quality"] = quality
	}

	metadata := map[string]any{
		"dimensions": map[string]any{
			"width":  dims.width,
			"height": dims.height,
		},
		"original": map[string]any{
			"filename":     file.Filename,
			"content_type": file.ContentType,
			"format":       format,
			"size_bytes":   file.SizeBytes,
		},
		"normalized": normalized,
	}

	addEXIFMetadata(metadata, file.Data)

	return metadata
}

func addEXIFMetadata(metadata map[string]any, data []byte) {
	x, err := exif.Decode(bytes.NewReader(data))
	if err != nil {
		metadata["exif"] = map[string]any{"present": false}
		return
	}

	exifMetadata := map[string]any{"present": true}
	fields := selectedEXIFFields(x)
	if len(fields) > 0 {
		exifMetadata["fields"] = fields
	}

	if capturedAt, err := x.DateTime(); err == nil {
		exifMetadata["taken_at"] = capturedAt.Format(time.RFC3339)
	}
	if lat, lon, err := x.LatLong(); err == nil {
		metadata["gps"] = map[string]any{
			"latitude":  lat,
			"longitude": lon,
		}
	}

	camera := cameraMetadata(fields)
	if len(camera) > 0 {
		metadata["camera"] = camera
	}
	if takenAt := fields["date_time_original"]; takenAt != "" {
		metadata["taken_at"] = takenAt
	}

	metadata["exif"] = exifMetadata
}

func selectedEXIFFields(x *exif.Exif) map[string]string {
	fields := make(map[string]string)
	for key, field := range map[string]exif.FieldName{
		"make":               exif.Make,
		"model":              exif.Model,
		"lens_make":          exif.LensMake,
		"lens_model":         exif.LensModel,
		"date_time":          exif.DateTime,
		"date_time_original": exif.DateTimeOriginal,
		"orientation":        exif.Orientation,
		"software":           exif.Software,
	} {
		if value := exifFieldString(x, field); value != "" {
			fields[key] = value
		}
	}

	return fields
}

func cameraMetadata(fields map[string]string) map[string]any {
	camera := make(map[string]any)
	for _, key := range []string{"make", "model", "lens_make", "lens_model"} {
		if value := fields[key]; value != "" {
			camera[key] = value
		}
	}

	return camera
}

func exifFieldString(x *exif.Exif, name exif.FieldName) string {
	tag, err := x.Get(name)
	if err != nil {
		return ""
	}

	value, err := tag.StringVal()
	if err != nil {
		value = tag.String()
	}

	value = strings.TrimSpace(value)
	value = strings.Trim(value, "\"")
	value = strings.TrimRight(value, "\x00")

	return value
}
