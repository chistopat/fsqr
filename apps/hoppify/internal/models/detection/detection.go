package detection

import capturemodel "github.com/chistopat/hoppify/internal/models/capture"

type Request struct {
	UUID string                   `json:"uuid,omitempty"`
	URL  string                   `json:"url,omitempty"`
	URI  string                   `json:"uri,omitempty"`
	File *capturemodel.UploadFile `json:"-"`
}

func (request Request) ImageURL() string {
	if request.URL != "" {
		return request.URL
	}

	return request.URI
}

type Response struct {
	Images   []ImageResult `json:"images"`
	Metadata Metadata      `json:"metadata"`
}

type ImageResult struct {
	Shape   [2]int      `json:"shape"`
	Results []Detection `json:"results"`
	Speed   Speed       `json:"speed"`
}

type Detection struct {
	Class      int     `json:"class"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	Box        Box     `json:"box"`
}

type Box struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

type Speed struct {
	Preprocess  float64 `json:"preprocess"`
	Inference   float64 `json:"inference"`
	Postprocess float64 `json:"postprocess"`
}

type Metadata struct {
	UUID             string  `json:"uuid,omitempty"`
	URL              string  `json:"url,omitempty"`
	ImageCount       int     `json:"imageCount"`
	FunctionTimeCall float64 `json:"functionTimeCall"`
}
