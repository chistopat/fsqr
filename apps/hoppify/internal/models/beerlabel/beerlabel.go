package beerlabel

import (
	"errors"
	"time"

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
	UUID string `json:"uuid"`
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
	Used    bool        `json:"used"`
	Queries []string    `json:"queries"`
	Sources []WebSource `json:"sources"`
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

type Response struct {
	UUID          string    `json:"uuid"`
	Model         string    `json:"model"`
	PromptVersion string    `json:"promptVersion"`
	Cached        bool      `json:"cached"`
	Result        Result    `json:"result"`
	CreatedAt     time.Time `json:"createdAt"`
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
