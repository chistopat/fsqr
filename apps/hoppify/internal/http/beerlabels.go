package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	beerlabelservice "github.com/chistopat/hoppify/internal/service/beerlabels"

	"go.uber.org/zap"
)

type BeerLabelIdentifier interface {
	Identify(ctx context.Context, request beerlabelmodel.Request) (beerlabelmodel.Response, error)
}

func identifyBeerLabel(service BeerLabelIdentifier, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(beerlabelservice.InternalError), "internal server error")
			return
		}

		var request beerlabelmodel.Request
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, string(beerlabelservice.InvalidRequest), "invalid json request body")
			return
		}

		loggerOrNop(log).Info(
			"beer label identify request accepted",
			zap.String("uuid", request.UUID),
			zap.String("url", request.ImageURL()),
		)
		response, err := service.Identify(r.Context(), request)
		if err != nil {
			loggerOrNop(log).Error(
				"beer label identify request failed",
				zap.String("uuid", request.UUID),
				zap.String("url", request.ImageURL()),
				zap.Error(err),
			)
			writeBeerLabelError(w, err)
			return
		}

		loggerOrNop(log).Info(
			"beer label identify request completed",
			zap.String("uuid", request.UUID),
			zap.Bool("cached", response.Cached),
			zap.String("status", response.Result.Status),
		)
		writeJSON(w, http.StatusOK, response)
	}
}
