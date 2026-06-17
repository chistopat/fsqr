package detection

type Request struct {
	UUID string `json:"uuid"`
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
	UUID             string  `json:"uuid"`
	ImageCount       int     `json:"imageCount"`
	FunctionTimeCall float64 `json:"functionTimeCall"`
}
