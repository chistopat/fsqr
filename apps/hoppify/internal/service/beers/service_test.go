package beers

import (
	"context"
	"errors"
	"testing"
	"time"

	beermodel "github.com/chistopat/hoppify/internal/models/beer"
)

func TestSearchBeersReturnsRepositoryResults(t *testing.T) {
	t.Parallel()

	modifiedAt := time.Date(2026, 6, 26, 9, 30, 0, 0, time.UTC)
	repository := &fakeRepository{records: []beermodel.Record{{
		UntappdID:      123,
		URL:            "https://untappd.com/b/brewdog-punk-ipa/123",
		UntappdSlug:    "brewdog-punk-ipa",
		BreweryPrefix:  "brewdog",
		SearchText:     "brewdog punk ipa",
		LastModifiedAt: modifiedAt,
		TextRank:       0.42,
		FuzzyRank:      0.74,
	}}}
	service := newTestService(t, repository)

	response, err := service.SearchBeers(context.Background(), "  punk ipa  ")

	if err != nil {
		t.Fatalf("search beers: %v", err)
	}
	if repository.query != "punk ipa" {
		t.Fatalf("expected normalized query, got %q", repository.query)
	}
	if repository.limit != SearchLimit {
		t.Fatalf("expected limit %d, got %d", SearchLimit, repository.limit)
	}
	if response.Query != "punk ipa" {
		t.Fatalf("unexpected response query: %q", response.Query)
	}
	if len(response.Results) != 1 || response.Results[0].UntappdID != 123 {
		t.Fatalf("unexpected search results: %#v", response.Results)
	}
}

func TestSearchBeersRejectsEmptyQuery(t *testing.T) {
	t.Parallel()

	service := newTestService(t, &fakeRepository{})

	_, err := service.SearchBeers(context.Background(), " ")

	var serviceErr *Error
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected service error, got %v", err)
	}
	if serviceErr.Code != InvalidRequest {
		t.Fatalf("expected invalid request, got %s", serviceErr.Code)
	}
}

func TestSearchBeersWrapsRepositoryError(t *testing.T) {
	t.Parallel()

	expected := errors.New("query failed")
	service := newTestService(t, &fakeRepository{err: expected})

	_, err := service.SearchBeers(context.Background(), "punk ipa")

	var serviceErr *Error
	if !errors.As(err, &serviceErr) {
		t.Fatalf("expected service error, got %v", err)
	}
	if serviceErr.Code != InternalError {
		t.Fatalf("expected internal error, got %s", serviceErr.Code)
	}
	if !errors.Is(err, expected) {
		t.Fatalf("expected wrapped repository error, got %v", err)
	}
}

func newTestService(t *testing.T, repository Repository) *Service {
	t.Helper()

	service, err := NewService(repository)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return service
}

type fakeRepository struct {
	records []beermodel.Record
	query   string
	limit   int
	err     error
}

func (repo *fakeRepository) Search(_ context.Context, query string, limit int) ([]beermodel.Record, error) {
	repo.query = query
	repo.limit = limit
	if repo.err != nil {
		return nil, repo.err
	}

	return repo.records, nil
}
