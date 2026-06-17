package places

import (
	"fmt"
	"testing"

	locationmodel "github.com/chistopat/fsqr/internal/models/location"
)

func TestNormalizeSearchInputDefaultsLimitAndDeduplicatesCategories(t *testing.T) {
	input, err := normalizeSearchInput(SearchInput{
		CategoryIDs: []int64{42, 42, 7},
		Location:    testLocation(t),
	})
	if err != nil {
		t.Fatal(err)
	}

	if input.limit != DefaultSearchLimit {
		t.Fatalf("expected default limit %d, got %d", DefaultSearchLimit, input.limit)
	}
	assertCategoryIDs(t, input.categoryIDs, []int64{42, 7})
}

func TestNormalizeSearchInputRejectsInvalidCategories(t *testing.T) {
	tests := []struct {
		name        string
		categoryIDs []int64
	}{
		{
			name:        "empty",
			categoryIDs: nil,
		},
		{
			name:        "non positive",
			categoryIDs: []int64{1, 0},
		},
		{
			name:        "too many distinct",
			categoryIDs: []int64{1, 2, 3, 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := normalizeSearchInput(SearchInput{
				CategoryIDs: tt.categoryIDs,
				Location:    testLocation(t),
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalizeSearchInputRejectsMissingLocation(t *testing.T) {
	_, err := normalizeSearchInput(SearchInput{
		CategoryIDs: []int64{42},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNormalizeSearchInputRejectsInvalidLimit(t *testing.T) {
	for _, limit := range []int{-1, MaxSearchLimit + 1} {
		t.Run(fmt.Sprintf("limit_%d", limit), func(t *testing.T) {
			_, err := normalizeSearchInput(SearchInput{
				CategoryIDs: []int64{42},
				Location:    testLocation(t),
				Limit:       limit,
			})
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

func TestNormalizeSearchInputRejectsInvalidBBox(t *testing.T) {
	_, err := normalizeSearchInput(SearchInput{
		CategoryIDs: []int64{42},
		Location:    testLocation(t),
		BBox:        &locationmodel.BBox{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSelectSearchQueryUsesBBoxWhenPresent(t *testing.T) {
	input := normalizedSearchInput{
		categoryIDs: []int64{1, 2},
		location:    testLocation(t),
		limit:       64,
		bbox:        testBBox(t),
	}

	query := selectSearchQuery(input)

	if query.operation != "bbox" {
		t.Fatalf("expected bbox operation, got %q", query.operation)
	}
	if query.statement != searchBBoxSQL {
		t.Fatal("expected bbox statement")
	}
	if len(query.args) != 8 {
		t.Fatalf("expected 8 args, got %d", len(query.args))
	}
}

func TestSelectSearchQueryUsesAntimeridianBBoxWhenBBoxCrossesDateline(t *testing.T) {
	location := mustLocation(t, 0, 179.99)
	bbox, err := locationmodel.NewBBoxAround(location, 5000)
	if err != nil {
		t.Fatal(err)
	}
	input := normalizedSearchInput{
		categoryIDs: []int64{1, 2},
		location:    location,
		limit:       64,
		bbox:        &bbox,
	}

	query := selectSearchQuery(input)

	if query.operation != "bbox_antimeridian" {
		t.Fatalf("expected bbox_antimeridian operation, got %q", query.operation)
	}
	if query.statement != searchBBoxAntimeridianSQL {
		t.Fatal("expected antimeridian bbox statement")
	}
	if len(query.args) != 8 {
		t.Fatalf("expected 8 args, got %d", len(query.args))
	}
	assertApprox(t, query.args[5].(float64), 179.945033981814, 0.000001)
	assertApprox(t, query.args[6].(float64), -179.965033981814, 0.000001)
}

func TestAntimeridianLongitudeRangesNearNegativeDateline(t *testing.T) {
	location := mustLocation(t, 0, -179.99)
	bbox, err := locationmodel.NewBBoxAround(location, 5000)
	if err != nil {
		t.Fatal(err)
	}

	positiveMinLon, negativeMaxLon := antimeridianLongitudeRanges(location, bbox)

	assertApprox(t, positiveMinLon, 179.965033981814, 0.000001)
	assertApprox(t, negativeMaxLon, -179.945033981814, 0.000001)
}

func TestSelectSearchQueryUsesCategoryFirstForOneCategoryWithoutBBox(t *testing.T) {
	input := normalizedSearchInput{
		categoryIDs: []int64{1},
		location:    testLocation(t),
		limit:       64,
	}

	query := selectSearchQuery(input)

	if query.operation != "no_bbox_category_first" {
		t.Fatalf("expected category-first operation, got %q", query.operation)
	}
	if query.statement != searchNoBBoxCategoryFirstSQL {
		t.Fatal("expected category-first statement")
	}
	if len(query.args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(query.args))
	}
}

func TestSelectSearchQueryUsesAnyForMultipleCategoriesWithoutBBox(t *testing.T) {
	input := normalizedSearchInput{
		categoryIDs: []int64{1, 2, 3},
		location:    testLocation(t),
		limit:       64,
	}

	query := selectSearchQuery(input)

	if query.operation != "no_bbox_any" {
		t.Fatalf("expected any operation, got %q", query.operation)
	}
	if query.statement != searchNoBBoxAnySQL {
		t.Fatal("expected any statement")
	}
	if len(query.args) != 4 {
		t.Fatalf("expected 4 args, got %d", len(query.args))
	}
}

func testLocation(t *testing.T) locationmodel.Location {
	t.Helper()

	return mustLocation(t, 34.772013, 32.429736)
}

func mustLocation(t *testing.T, lat float64, lon float64) locationmodel.Location {
	t.Helper()

	location, err := locationmodel.New(lat, lon)
	if err != nil {
		t.Fatal(err)
	}

	return location
}

func testBBox(t *testing.T) *locationmodel.BBox {
	t.Helper()

	bbox, err := locationmodel.NewBBox(mustLocation(t, 34, 32), mustLocation(t, 35, 33))
	if err != nil {
		t.Fatal(err)
	}

	return &bbox
}

func assertCategoryIDs(t *testing.T, actual []int64, expected []int64) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected %d category ids, got %d: %v", len(expected), len(actual), actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected category ids %v, got %v", expected, actual)
		}
	}
}

func assertApprox(t *testing.T, actual float64, expected float64, tolerance float64) {
	t.Helper()

	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("expected %f within %f, got %f", expected, tolerance, actual)
	}
}
