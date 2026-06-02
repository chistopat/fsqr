package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/chistopat/fsqr/internal/models"
	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	"github.com/chistopat/fsqr/internal/models/category/level"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	placeservice "github.com/chistopat/fsqr/internal/service/place"
)

func TestSearchPlacesReturnsPlaces(t *testing.T) {
	searchService := &stubSearchService{
		response: models.SearchResponse{
			TookMS: 12,
			Places: []models.SearchPlace{
				{
					UUID: "fsq-1",
					Name: "Example Coffee",
					Lat:  34.772013,
					Lon:  32.429736,
				},
			},
		},
	}
	app := NewRouter(Dependencies{
		SearchService: searchService,
	})

	resp, err := app.Test(httptest.NewRequest(
		http.MethodGet,
		"/api/v1/search?query=coffee&location=34.772013,32.429736&limit=64&distance_meters=3000",
		nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if !searchService.called {
		t.Fatal("expected search service to be called")
	}
	if searchService.input.Query != "coffee" {
		t.Fatalf("expected query coffee, got %q", searchService.input.Query)
	}
	if searchService.input.Limit != 64 {
		t.Fatalf("expected limit 64, got %d", searchService.input.Limit)
	}
	if searchService.input.Location.Lat() != 34.772013 {
		t.Fatalf("expected lat 34.772013, got %f", searchService.input.Location.Lat())
	}
	if searchService.input.DistanceMeters != 3000 {
		t.Fatalf("expected distance_meters 3000, got %d", searchService.input.DistanceMeters)
	}

	var response models.SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if len(response.Places) != 1 {
		t.Fatalf("expected 1 place, got %d", len(response.Places))
	}
	if response.Places[0].UUID != "fsq-1" {
		t.Fatalf("expected fsq-1, got %q", response.Places[0].UUID)
	}
}

func TestSearchPlacesRejectsInvalidLocation(t *testing.T) {
	searchService := &stubSearchService{}
	app := NewRouter(Dependencies{
		SearchService: searchService,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/search?query=coffee&location=bad", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	if searchService.called {
		t.Fatal("expected search service not to be called")
	}

	var response models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != models.InvalidRequest {
		t.Fatalf("expected error code %q, got %q", models.InvalidRequest, response.Error.Code)
	}
}

func TestSearchPlacesRejectsInvalidLimit(t *testing.T) {
	searchService := &stubSearchService{}
	app := NewRouter(Dependencies{
		SearchService: searchService,
	})

	resp, err := app.Test(httptest.NewRequest(
		http.MethodGet,
		"/api/v1/search?query=coffee&location=34.772013,32.429736&limit=bad",
		nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	if searchService.called {
		t.Fatal("expected search service not to be called")
	}
}

func TestSearchPlacesRejectsInvalidDistance(t *testing.T) {
	searchService := &stubSearchService{}
	app := NewRouter(Dependencies{
		SearchService: searchService,
	})

	resp, err := app.Test(httptest.NewRequest(
		http.MethodGet,
		"/api/v1/search?query=coffee&location=34.772013,32.429736&distance_meters=0",
		nil,
	))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	if searchService.called {
		t.Fatal("expected search service not to be called")
	}
}

func TestSearchCategoriesReturnsCategories(t *testing.T) {
	categoryLevel, err := level.New(3)
	if err != nil {
		t.Fatal(err)
	}

	categoryService := &stubCategoryService{
		categories: []categorymodel.Category{
			{
				ID:            42,
				FSQCategoryID: "4bf58dd8d48988d1e0931735",
				Name:          "Coffee Shop",
				Label:         "Dining and Drinking > Cafe, Coffee, and Tea House > Coffee Shop",
				Level:         categoryLevel,
			},
		},
	}
	app := NewRouter(Dependencies{
		CategoryService: categoryService,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/categories?query=coffee&limit=3", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if categoryService.query.String() != "coffee" {
		t.Fatalf("expected query coffee, got %q", categoryService.query.String())
	}
	if categoryService.query.Limit() != 3 {
		t.Fatalf("expected limit 3, got %d", categoryService.query.Limit())
	}

	var categories []categorymodel.Category
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		t.Fatal(err)
	}
	if len(categories) != 1 {
		t.Fatalf("expected 1 category, got %d", len(categories))
	}
	if categories[0].Name != "Coffee Shop" {
		t.Fatalf("expected Coffee Shop, got %q", categories[0].Name)
	}
}

func TestSearchCategoriesRejectsInvalidRequest(t *testing.T) {
	app := NewRouter(Dependencies{
		CategoryService: &stubCategoryService{},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/categories?query=&limit=3", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}

	var response models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != models.InvalidRequest {
		t.Fatalf("expected error code %q, got %q", models.InvalidRequest, response.Error.Code)
	}
}

func TestGetPlaceReturnsPlaceDetails(t *testing.T) {
	placeService := &stubPlaceService{
		place: models.PlaceDetails{
			UUID: "fsq-1",
			Name: "Example Coffee",
			Lat:  34.772013,
			Lon:  32.429736,
		},
	}
	app := NewRouter(Dependencies{
		PlaceService: placeService,
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/places/fsq-1", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if placeService.uuid != "fsq-1" {
		t.Fatalf("expected uuid fsq-1, got %q", placeService.uuid)
	}

	var response models.PlaceDetails
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.UUID != "fsq-1" {
		t.Fatalf("expected fsq-1, got %q", response.UUID)
	}
}

func TestGetPlaceReturnsNotFound(t *testing.T) {
	app := NewRouter(Dependencies{
		PlaceService: &stubPlaceService{err: placeservice.ErrPlaceNotFound},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/places/missing", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}

	var response models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != models.NotFound {
		t.Fatalf("expected error code %q, got %q", models.NotFound, response.Error.Code)
	}
}

func TestLiveReturnsOK(t *testing.T) {
	app := NewRouter(Dependencies{})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/live", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var response models.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "ok" {
		t.Fatalf("expected ok status, got %q", response.Status)
	}
}

func TestHealthReturnsOK(t *testing.T) {
	app := NewRouter(Dependencies{
		HealthChecker: &stubHealthChecker{},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
}

func TestHealthReturnsUnavailable(t *testing.T) {
	app := NewRouter(Dependencies{
		HealthChecker: &stubHealthChecker{err: errors.New("db password leaked")},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/health", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	var response models.HealthResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Status != "unhealthy" {
		t.Fatalf("expected unhealthy status, got %q", response.Status)
	}
}

func TestInternalErrorsDoNotLeakDetails(t *testing.T) {
	app := NewRouter(Dependencies{
		CategoryService: &stubCategoryService{err: errors.New("db password leaked")},
	})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/categories?query=coffee&limit=3", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	var response models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response.Error.Code != models.InternalError {
		t.Fatalf("expected error code %q, got %q", models.InternalError, response.Error.Code)
	}
	if response.Error.Message != "internal server error" {
		t.Fatalf("expected safe internal error message, got %q", response.Error.Message)
	}
}

func TestPublicRouterDoesNotExposeMetrics(t *testing.T) {
	app := NewRouter(Dependencies{})

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, resp.StatusCode)
	}
}

func TestMetricsRouterExposesHTTPMetrics(t *testing.T) {
	registry := newMetricsRegistry(nil)
	app := NewRouter(Dependencies{
		CategoryService: &stubCategoryService{},
		MetricsRegistry: registry,
	})
	metricsApp := NewMetricsRouter(registry, "/metrics")

	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/api/v1/categories?query=coffee&limit=3", nil))
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	metricsResp, err := metricsApp.Test(httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = metricsResp.Body.Close()
	}()
	if metricsResp.StatusCode != http.StatusOK {
		t.Fatalf("expected metrics status %d, got %d", http.StatusOK, metricsResp.StatusCode)
	}

	body, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	metrics := string(body)
	for _, expected := range []string{
		"fsqr_http_requests_total",
		"fsqr_http_request_duration_seconds",
		`route="/api/v1/categories"`,
		`status="200"`,
	} {
		if !strings.Contains(metrics, expected) {
			t.Fatalf("expected metrics to contain %q\n%s", expected, metrics)
		}
	}
}

type stubSearchService struct {
	response models.SearchResponse
	input    placeservice.SearchPlacesInput
	err      error
	called   bool
}

func (service *stubSearchService) SearchPlaces(
	_ context.Context,
	input placeservice.SearchPlacesInput,
) (models.SearchResponse, error) {
	service.called = true
	service.input = input
	if service.err != nil {
		return models.SearchResponse{}, service.err
	}

	return service.response, nil
}

type stubCategoryService struct {
	categories []categorymodel.Category
	query      querymodel.Query
	err        error
}

func (service *stubCategoryService) SearchCategories(
	_ context.Context,
	query querymodel.Query,
) ([]categorymodel.Category, error) {
	service.query = query
	if service.err != nil {
		return nil, service.err
	}

	return service.categories, nil
}

type stubPlaceService struct {
	place models.PlaceDetails
	uuid  string
	err   error
}

type stubHealthChecker struct {
	err error
}

func (checker *stubHealthChecker) PingContext(_ context.Context) error {
	return checker.err
}

func (service *stubPlaceService) GetPlace(
	_ context.Context,
	uuid string,
) (models.PlaceDetails, error) {
	service.uuid = uuid
	if service.err != nil {
		return models.PlaceDetails{}, service.err
	}

	return service.place, nil
}
