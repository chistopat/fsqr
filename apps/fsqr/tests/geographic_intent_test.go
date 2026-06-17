//go:build e2e

package e2e

import "testing"

func TestE2E_GEO_PAPHOS_BEACH_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: geographyFixture,
		path:    "/api/v1/search?query=beach&location=34.772013,32.429736&distance_meters=2000&limit=10",
		wantUUIDs: []string{
			"tc-beach-paphos-near",
		},
		excluded: []string{"tc-beach-paphos-far", "tc-park-paphos-distractor", "tc-restaurant-paphos-distractor"},
	})
}

func TestE2E_GEO_ANTIMERIDIAN_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: antimeridianFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=0.0,179.99&distance_meters=5000&limit=3",
		wantUUIDs: []string{
			"tc-coffee-antimeridian-east",
			"tc-coffee-antimeridian-west",
		},
		excluded: []string{"tc-coffee-antimeridian-zero"},
	})
}
