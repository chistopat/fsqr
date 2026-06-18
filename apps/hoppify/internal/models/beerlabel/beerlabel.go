package beerlabel

import (
	"errors"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
)

const (
	StatusIdentified = "identified"
	StatusUncertain  = "uncertain"
	StatusUnreadable = "unreadable"
	StatusNotBeer    = "not_beer"

	ContainerBottle  = "bottle"
	ContainerCan     = "can"
	ContainerGlass   = "glass"
	ContainerOther   = "other"
	ContainerUnknown = "unknown"

	UntappdDirectMatch       = "direct_match"
	UntappdSearchRecommended = "search_recommended"
	UntappdAmbiguous         = "ambiguous"
	UntappdNotFound          = "not_found"
	UntappdNotApplicable     = "not_applicable"
)

var ErrNotFound = errors.New("beer label recognition not found")

type Request struct {
	UUID string `json:"uuid,omitempty"`
	URL  string `json:"url,omitempty"`
	URI  string `json:"uri,omitempty"`
}

func (request Request) ImageURL() string {
	if request.URL != "" {
		return request.URL
	}

	return request.URI
}

type Result struct {
	Status     string                 `json:"status"`
	Container  string                 `json:"container"`
	BeerName   *string                `json:"beerName"`
	Brewery    *string                `json:"brewery"`
	Style      *string                `json:"style"`
	Country    *string                `json:"country"`
	ABV        *float64               `json:"abv"`
	Confidence float64                `json:"confidence"`
	Evidence   []string               `json:"evidence"`
	Notes      *string                `json:"notes"`
	WebSearch  *WebSearchResult       `json:"webSearch,omitempty"`
	Untappd    *UntappdRecommendation `json:"untappd,omitempty"`
}

type WebSearchResult struct {
	Used                 bool        `json:"used"`
	Queries              []string    `json:"queries"`
	Sources              []WebSource `json:"sources"`
	SearchEntryPointHTML *string     `json:"searchEntryPointHtml,omitempty"`
}

type WebSource struct {
	Title *string `json:"title"`
	URL   string  `json:"url"`
}

type UntappdRecommendation struct {
	Status     string  `json:"status"`
	URL        *string `json:"url"`
	SearchURL  *string `json:"searchUrl"`
	Name       *string `json:"name"`
	Brewery    *string `json:"brewery"`
	Confidence float64 `json:"confidence"`
	Reason     *string `json:"reason"`
}

type Record struct {
	CaptureUUID   uuid.UUID
	Model         string
	PromptVersion string
	Result        Result
	CreatedAt     time.Time
}

type ListRecord struct {
	Crop        capturemodel.Record
	Recognition Record
}

type Response struct {
	UUID          string    `json:"uuid,omitempty"`
	URL           string    `json:"url,omitempty"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"promptVersion"`
	Cached        bool      `json:"cached"`
	Result        Result    `json:"result"`
	CreatedAt     time.Time `json:"createdAt"`
}

type BatchRequest struct {
	UUIDs []string `json:"uuids"`
}

type BatchResponse struct {
	Recognitions []BatchItem `json:"recognitions"`
}

type BatchItem struct {
	UUID        string      `json:"uuid"`
	Recognition *Response   `json:"recognition,omitempty"`
	Error       *BatchError `json:"error,omitempty"`
}

type BatchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ListResult struct {
	Records []ListRecord
	HasMore bool
}

type ListResponse struct {
	Recognitions []ListItem `json:"recognitions"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
	NextOffset   int        `json:"nextOffset,omitempty"`
	HasMore      bool       `json:"hasMore"`
}

type ListItem struct {
	UUID          string                `json:"uuid"`
	Crop          capturemodel.ListItem `json:"crop"`
	Model         string                `json:"model"`
	PromptVersion string                `json:"promptVersion"`
	Result        Result                `json:"result"`
	CreatedAt     time.Time             `json:"createdAt"`
}

func ResponseFromRecord(record *Record, cached bool) Response {
	return Response{
		UUID:          record.CaptureUUID.String(),
		Model:         record.Model,
		PromptVersion: record.PromptVersion,
		Cached:        cached,
		Result:        record.Result,
		CreatedAt:     record.CreatedAt,
	}
}

func ListResponseFromRecords(records []ListRecord, query capturemodel.ListQuery, hasMore bool) ListResponse {
	recognitions := make([]ListItem, 0, len(records))
	for index := range records {
		recognitions = append(recognitions, records[index].ListItem())
	}

	response := ListResponse{
		Recognitions: recognitions,
		Limit:        query.Limit,
		Offset:       query.Offset,
		HasMore:      hasMore,
	}
	if hasMore {
		response.NextOffset = query.Offset + len(records)
	}

	return response
}

func (record *ListRecord) ListItem() ListItem {
	recognition := record.Recognition

	return ListItem{
		UUID:          recognition.CaptureUUID.String(),
		Crop:          record.Crop.ListItem(),
		Model:         recognition.Model,
		PromptVersion: recognition.PromptVersion,
		Result:        recognition.Result,
		CreatedAt:     recognition.CreatedAt,
	}
}
