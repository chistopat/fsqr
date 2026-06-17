//go:build e2e

package e2e

import "testing"

func TestE2E_CAT_FTS_001(t *testing.T) {
	runCategoryCase(t, categoryCase{
		path:          "/api/v1/categories?query=coffee%20shop&limit=3",
		wantFirstID:   1001,
		wantFirstFSQ:  "4bf58dd8d48988d1e0931735",
		wantFirstName: "Coffee Shop",
	})
}

func TestE2E_CAT_FTS_002(t *testing.T) {
	runCategoryCase(t, categoryCase{
		path:          "/api/v1/categories?query=diesel%20gasoline%20petrol&limit=3",
		wantFirstID:   2001,
		wantFirstFSQ:  "4bf58dd8d48988d113951735",
		wantFirstName: "Fuel Station",
	})
}

func TestE2E_CAT_VEC_001(t *testing.T) {
	runCategoryCase(t, categoryCase{
		path:          "/api/v1/categories?query=where%20can%20I%20charge%20my%20Tesla&limit=3",
		wantFirstID:   2002,
		wantFirstName: "Electric Vehicle Charging Station",
	})
}

func TestE2E_CAT_RU_001(t *testing.T) {
	runCategoryCase(t, categoryCase{
		path:          "/api/v1/categories?query=%D0%B0%D0%BF%D1%82%D0%B5%D0%BA%D0%B0%20%D1%80%D1%8F%D0%B4%D0%BE%D0%BC&limit=3",
		wantFirstID:   3001,
		wantFirstName: "Pharmacy",
	})
}

func TestE2E_CAT_LIMIT_001(t *testing.T) {
	runCategoryCase(t, categoryCase{
		path:          "/api/v1/categories?query=coffee&limit=1",
		wantLength:    1,
		wantFirstID:   1001,
		wantFirstName: "Coffee Shop",
	})
}
