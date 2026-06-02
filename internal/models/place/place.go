package place

type Place struct {
	UUID       string  `json:"uuid"`
	Name       string  `json:"name"`
	CategoryID int64   `json:"category_id"`
	Lat        float64 `json:"lat"`
	Lon        float64 `json:"lon"`
	Distance   float64 `json:"distance"`
}
