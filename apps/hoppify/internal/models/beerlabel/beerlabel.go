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
)

var ErrNotFound = errors.New("beer label recognition not found")

type Request struct {
	UUID string `json:"uuid"`
}

type Result struct {
	Status     string   `json:"status"`
	Container  string   `json:"container"`
	BeerName   *string  `json:"beerName"`
	Brewery    *string  `json:"brewery"`
	Style      *string  `json:"style"`
	Country    *string  `json:"country"`
	ABV        *float64 `json:"abv"`
	Confidence float64  `json:"confidence"`
	Evidence   []string `json:"evidence"`
	Notes      *string  `json:"notes"`
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
