package crop

import capturemodel "github.com/chistopat/hoppify/internal/models/capture"

type Request struct {
	UUID  string       `json:"uuid"`
	Boxes []BoxRequest `json:"boxes"`
}

type BoxRequest struct {
	BBox       []float64 `json:"bbox"`
	Confidence float64   `json:"confidence,omitempty"`
}

type Response struct {
	UUID  string         `json:"uuid"`
	Crops []CropResponse `json:"crops"`
}

type CropResponse struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	URI  string `json:"uri"`
	URL  string `json:"url,omitempty"`
}

func ResponseFromCapture(record *capturemodel.Record) CropResponse {
	uri := capturemodel.URI(record.Bucket, record.ObjectKey)

	return CropResponse{
		UUID: record.UUID.String(),
		Type: record.Type,
		URI:  uri,
		URL:  uri,
	}
}
