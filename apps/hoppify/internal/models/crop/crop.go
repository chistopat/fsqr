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

type ListResponse struct {
	Crops      []capturemodel.ListItem `json:"crops"`
	Limit      int                     `json:"limit"`
	Offset     int                     `json:"offset"`
	NextOffset int                     `json:"nextOffset,omitempty"`
	HasMore    bool                    `json:"hasMore"`
}

type CropResponse struct {
	UUID string `json:"uuid"`
	Type string `json:"type"`
	URI  string `json:"uri"`
	URL  string `json:"url,omitempty"`
}

func ListResponseFromRecords(records []capturemodel.Record, query capturemodel.ListQuery, hasMore bool) ListResponse {
	crops := make([]capturemodel.ListItem, 0, len(records))
	for index := range records {
		crops = append(crops, records[index].ListItem())
	}

	response := ListResponse{
		Crops:   crops,
		Limit:   query.Limit,
		Offset:  query.Offset,
		HasMore: hasMore,
	}
	if hasMore {
		response.NextOffset = query.Offset + len(records)
	}

	return response
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
