//go:build e2e

package e2e

import (
	"strings"
	"testing"
)

func TestE2E_VAL_SEARCH_001(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/search?location=34.772013,32.429736",
		messageContains: "search query must not be empty",
	})
}

func TestE2E_VAL_SEARCH_002(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/search?query=coffee&location=bad",
		messageContains: "location must be in \"lat,lon\" format",
	})
}

func TestE2E_VAL_SEARCH_003(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/search?query=coffee&location=91,32.429736",
		messageContains: "latitude must be between -90 and 90",
	})
}

func TestE2E_VAL_SEARCH_004(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/search?query=coffee&location=34.772013,32.429736&limit=129",
		messageContains: "limit must be between 1 and 128",
	})
}

func TestE2E_VAL_SEARCH_005(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/search?query=coffee&location=34.772013,32.429736&distance_meters=0",
		messageContains: "distance_meters must be positive",
	})
}

func TestE2E_VAL_CAT_001(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/categories?query=&limit=3",
		messageContains: "search query must not be empty",
	})
}

func TestE2E_VAL_CAT_002(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/categories?query=coffee&limit=101",
		messageContains: "search limit must be between 1 and 100",
	})
}

func TestE2E_VAL_CAT_003(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/categories?query=coffee&limit=bad",
		messageContains: "limit must be an integer",
	})
}

func TestE2E_VAL_PLACE_001(t *testing.T) {
	runValidationCase(t, validationCase{
		path:            "/api/v1/places/" + strings.Repeat("x", 129),
		messageContains: "place uuid must be at most 128 characters",
	})
}
