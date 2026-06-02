//go:build e2e

package e2e

import "testing"

func TestE2E_SEARCH_COFFEE_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: coffeePaphosFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=34.772013,32.429736&distance_meters=500&limit=10",
		wantUUIDs: []string{
			"tc-coffee-paphos-center",
			"tc-coffee-paphos-east",
			"tc-coffee-paphos-north",
		},
		excluded: []string{"tc-fuel-paphos-distractor", "tc-library-paphos-distractor"},
	})
}

func TestE2E_SEARCH_COFFEE_002(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: coffeePaphosFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=34.772013,32.429736&limit=10",
		wantUUIDs: []string{
			"tc-coffee-paphos-center",
			"tc-coffee-paphos-east",
			"tc-coffee-paphos-north",
			"tc-coffee-paphos-outside-500m",
		},
		excluded: []string{"tc-fuel-paphos-distractor", "tc-library-paphos-distractor"},
	})
}

func TestE2E_SEARCH_LIMIT_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: coffeePaphosFixture,
		path:    "/api/v1/search?query=coffee%20shop&location=34.772013,32.429736&distance_meters=500&limit=2",
		wantUUIDs: []string{
			"tc-coffee-paphos-center",
			"tc-coffee-paphos-east",
		},
	})
}

func TestE2E_SEARCH_FUEL_001(t *testing.T) {
	runSearchCase(t, searchCase{
		fixture: fuelPaphosFixture,
		path:    "/api/v1/search?query=diesel%20gasoline%20petrol&location=34.772013,32.429736&distance_meters=500&limit=10",
		wantUUIDs: []string{
			"tc-fuel-paphos-center",
			"tc-fuel-paphos-west",
		},
		excluded: []string{"tc-ev-paphos-nearby", "tc-coffee-fuel-distractor"},
	})
}
