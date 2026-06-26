package beer

import "time"

type Record struct {
	UntappdID      int64
	URL            string
	UntappdSlug    string
	BreweryPrefix  string
	SearchText     string
	LastModifiedAt time.Time
	TextRank       float64
	FuzzyRank      float64
}

type SearchResponse struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

type Result struct {
	UntappdID      int64     `json:"untappdId"`
	URL            string    `json:"url"`
	UntappdSlug    string    `json:"untappdSlug"`
	BreweryPrefix  string    `json:"breweryPrefix,omitempty"`
	SearchText     string    `json:"searchText"`
	LastModifiedAt time.Time `json:"lastModifiedAt"`
	TextRank       float64   `json:"textRank"`
	FuzzyRank      float64   `json:"fuzzyRank"`
}

func SearchResponseFromRecords(query string, records []Record) SearchResponse {
	results := make([]Result, 0, len(records))
	for _, record := range records {
		results = append(results, Result(record))
	}

	return SearchResponse{Query: query, Results: results}
}
