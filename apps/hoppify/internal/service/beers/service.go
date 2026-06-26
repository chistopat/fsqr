package beers

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	beermodel "github.com/chistopat/hoppify/internal/models/beer"
)

const (
	SearchLimit         = 10
	MaxSearchQueryRunes = 256
)

type Repository interface {
	Search(ctx context.Context, query string, limit int) ([]beermodel.Record, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) (*Service, error) {
	if repository == nil {
		return nil, fmt.Errorf("beers repository is required")
	}

	return &Service{repository: repository}, nil
}

func (svc *Service) SearchBeers(ctx context.Context, rawQuery string) (beermodel.SearchResponse, error) {
	query, err := normalizeSearchQuery(rawQuery)
	if err != nil {
		return beermodel.SearchResponse{}, newError(InvalidRequest, err.Error(), err)
	}

	records, err := svc.repository.Search(ctx, query, SearchLimit)
	if err != nil {
		return beermodel.SearchResponse{}, newError(InternalError, "internal server error", err)
	}

	return beermodel.SearchResponseFromRecords(query, records), nil
}

func normalizeSearchQuery(rawQuery string) (string, error) {
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return "", fmt.Errorf("query must not be empty")
	}
	if utf8.RuneCountInString(query) > MaxSearchQueryRunes {
		return "", fmt.Errorf("query must be at most %d characters", MaxSearchQueryRunes)
	}

	return query, nil
}
