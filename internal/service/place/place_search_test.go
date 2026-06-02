package place

import (
	"context"
	"errors"
	"testing"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	"github.com/chistopat/fsqr/internal/models/category/level"
	locationmodel "github.com/chistopat/fsqr/internal/models/location"
	placemodel "github.com/chistopat/fsqr/internal/models/place"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	placesrepo "github.com/chistopat/fsqr/internal/repository/places"
)

func TestPlaceSearchSearchesCategoriesThenPlaces(t *testing.T) {
	categoryLevel, err := level.New(3)
	if err != nil {
		t.Fatal(err)
	}

	categories := &stubPlaceSearchCategories{
		categories: []categorymodel.Category{
			{ID: 42, Name: "Coffee Shop", Level: categoryLevel},
			{ID: 7, Name: "Cafe", Level: categoryLevel},
		},
	}
	places := &stubPlaceRepository{
		places: []placemodel.Place{
			{
				UUID:       "fsq-1",
				Name:       "Example Coffee",
				CategoryID: 42,
				Lat:        34.772013,
				Lon:        32.429736,
			},
		},
	}
	service := NewPlaceSearch(categories, places)
	location := mustLocation(t)

	response, err := service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:          "coffee nearby",
		Location:       location,
		Limit:          64,
		DistanceMeters: 3000,
	})
	if err != nil {
		t.Fatal(err)
	}

	if categories.query.String() != "coffee nearby" {
		t.Fatalf("expected category query coffee nearby, got %q", categories.query.String())
	}
	if categories.query.Limit() != SearchCategoryLimit {
		t.Fatalf("expected category limit %d, got %d", SearchCategoryLimit, categories.query.Limit())
	}
	assertCategoryIDs(t, places.input.CategoryIDs, []int64{42, 7})
	if places.input.Limit != 64 {
		t.Fatalf("expected places limit 64, got %d", places.input.Limit)
	}
	if places.input.Location != location {
		t.Fatal("expected places input location to match")
	}
	if places.input.BBox == nil || !places.input.BBox.Valid() {
		t.Fatal("expected computed bbox")
	}
	if len(response.Places) != 1 {
		t.Fatalf("expected 1 place, got %d", len(response.Places))
	}
	if response.Places[0].UUID != "fsq-1" {
		t.Fatalf("expected fsq-1, got %q", response.Places[0].UUID)
	}
}

func TestPlaceSearchUsesDefaultDistance(t *testing.T) {
	categoryLevel, err := level.New(3)
	if err != nil {
		t.Fatal(err)
	}

	categories := &stubPlaceSearchCategories{
		categories: []categorymodel.Category{
			{ID: 42, Name: "Coffee Shop", Level: categoryLevel},
		},
	}
	places := &stubPlaceRepository{}
	service := NewPlaceSearch(categories, places)
	location := mustLocation(t)

	_, err = service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:    "coffee nearby",
		Location: location,
	})
	if err != nil {
		t.Fatal(err)
	}

	expectedBBox, err := locationmodel.NewBBoxAround(location, DefaultSearchDistanceMeters)
	if err != nil {
		t.Fatal(err)
	}
	if places.input.BBox == nil {
		t.Fatal("expected computed bbox")
	}
	if places.input.BBox.Min().Lat() != expectedBBox.Min().Lat() {
		t.Fatalf("expected default bbox min lat %f, got %f", expectedBBox.Min().Lat(), places.input.BBox.Min().Lat())
	}
}

func TestPlaceSearchReturnsEmptyResponseWhenNoCategoriesMatch(t *testing.T) {
	categories := &stubPlaceSearchCategories{}
	places := &stubPlaceRepository{}
	service := NewPlaceSearch(categories, places)

	response, err := service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:    "coffee nearby",
		Location: mustLocation(t),
		Limit:    64,
	})
	if err != nil {
		t.Fatal(err)
	}

	if places.called {
		t.Fatal("expected place repository not to be called")
	}
	if len(response.Places) != 0 {
		t.Fatalf("expected empty places, got %d", len(response.Places))
	}
}

func TestPlaceSearchRejectsInvalidInput(t *testing.T) {
	service := NewPlaceSearch(&stubPlaceSearchCategories{}, &stubPlaceRepository{})

	_, err := service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:    "",
		Location: mustLocation(t),
	})
	if !IsInvalidSearchInput(err) {
		t.Fatalf("expected invalid search input error, got %v", err)
	}
}

func TestPlaceSearchRejectsInvalidDistance(t *testing.T) {
	service := NewPlaceSearch(&stubPlaceSearchCategories{}, &stubPlaceRepository{})

	_, err := service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:          "coffee nearby",
		Location:       mustLocation(t),
		DistanceMeters: -1,
	})
	if !IsInvalidSearchInput(err) {
		t.Fatalf("expected invalid search input error, got %v", err)
	}
}

func TestPlaceSearchReturnsCategoryError(t *testing.T) {
	expected := errors.New("category search failed")
	service := NewPlaceSearch(
		&stubPlaceSearchCategories{err: expected},
		&stubPlaceRepository{},
	)

	_, err := service.SearchPlaces(context.Background(), SearchPlacesInput{
		Query:    "coffee nearby",
		Location: mustLocation(t),
	})
	if !errors.Is(err, expected) {
		t.Fatalf("expected category error, got %v", err)
	}
}

type stubPlaceSearchCategories struct {
	categories []categorymodel.Category
	query      querymodel.Query
	err        error
}

func (service *stubPlaceSearchCategories) SearchCategories(
	_ context.Context,
	query querymodel.Query,
) ([]categorymodel.Category, error) {
	service.query = query
	if service.err != nil {
		return nil, service.err
	}

	return service.categories, nil
}

type stubPlaceRepository struct {
	places []placemodel.Place
	input  placesrepo.SearchInput
	err    error
	called bool
}

func (repo *stubPlaceRepository) Search(
	_ context.Context,
	input placesrepo.SearchInput,
) ([]placemodel.Place, error) {
	repo.called = true
	repo.input = input
	if repo.err != nil {
		return nil, repo.err
	}

	return repo.places, nil
}

func mustLocation(t *testing.T) locationmodel.Location {
	t.Helper()

	location, err := locationmodel.New(34.772013, 32.429736)
	if err != nil {
		t.Fatal(err)
	}

	return location
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
