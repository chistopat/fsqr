//go:build e2e

package e2e

import "testing"

func TestE2E_BBOX_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: coffeePaphosFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=34.772013,32.429736&distance_meters=500&limit=10",
		wantUUIDs: []string{
			"tc-coffee-paphos-center",
			"tc-coffee-paphos-east",
			"tc-coffee-paphos-north",
		},
		excluded: []string{"tc-coffee-paphos-outside-500m"},
	})
}

func TestE2E_DIST_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: coffeePaphosFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=34.772013,32.429736&distance_meters=500&limit=10",
		wantUUIDs: []string{
			"tc-coffee-paphos-center",
			"tc-coffee-paphos-east",
			"tc-coffee-paphos-north",
		},
	})
}

func TestE2E_GEO_HIGHLAT_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: highLatitudeFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=80.0,0.0&distance_meters=1000&limit=10",
		wantUUIDs: []string{
			"tc-coffee-highlat-center",
			"tc-coffee-highlat-inside-lon",
		},
		excluded: []string{"tc-coffee-highlat-outside-lon", "tc-coffee-highlat-outside-lat"},
	})
}
