package httpapi

import (
	"context"
	"net/http"

	beermodel "github.com/chistopat/hoppify/internal/models/beer"
	beerservice "github.com/chistopat/hoppify/internal/service/beers"

	"go.uber.org/zap"
)

type BeerSearcher interface {
	SearchBeers(ctx context.Context, query string) (beermodel.SearchResponse, error)
}

func searchBeers(service BeerSearcher, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(beerservice.InternalError), "internal server error")
			return
		}

		query := r.URL.Query().Get("query")
		loggerOrNop(log).Debug("beer search request accepted", zap.Int("query_length", len([]rune(query))))

		response, err := service.SearchBeers(r.Context(), query)
		if err != nil {
			loggerOrNop(log).Error("beer search request failed", zap.Error(err))
			writeBeerSearchError(w, err)
			return
		}

		loggerOrNop(log).Debug("beer search request completed", zap.Int("result_count", len(response.Results)))
		writeJSON(w, http.StatusOK, response)
	}
}
