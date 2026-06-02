package models

type SearchResponse struct {
	TookMS int32         `json:"took_ms"`
	Places []SearchPlace `json:"places"`
}

type SearchPlace struct {
	UUID string  `json:"uuid"`
	Name string  `json:"name"`
	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
}

type PlaceDetails struct {
	UUID     string         `json:"uuid"`
	Name     string         `json:"name"`
	Lat      float64        `json:"lat"`
	Lon      float64        `json:"lon"`
	Category *PlaceCategory `json:"category,omitempty"`
	Address  *PlaceAddress  `json:"address,omitempty"`
	Contacts *PlaceContacts `json:"contacts,omitempty"`
}

type PlaceCategory struct {
	FSQCategoryID string `json:"fsq_category_id"`
	Name          string `json:"name"`
	Path          string `json:"path"`
}

type PlaceAddress struct {
	Line     *string `json:"line,omitempty"`
	Locality *string `json:"locality,omitempty"`
	Region   *string `json:"region,omitempty"`
	Country  *string `json:"country,omitempty"`
}

type PlaceContacts struct {
	Tel        *string `json:"tel,omitempty"`
	Website    *string `json:"website,omitempty"`
	Email      *string `json:"email,omitempty"`
	FacebookID *int64  `json:"facebook_id,omitempty"`
	Instagram  *string `json:"instagram,omitempty"`
	Twitter    *string `json:"twitter,omitempty"`
}
