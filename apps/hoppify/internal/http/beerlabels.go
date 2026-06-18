package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	beerlabelservice "github.com/chistopat/hoppify/internal/service/beerlabels"

	"go.uber.org/zap"
)

type BeerLabelIdentifier interface {
	Identify(ctx context.Context, request beerlabelmodel.Request) (beerlabelmodel.Response, error)
}

type BeerLabelRecognitionLister interface {
	ListRecognitions(ctx context.Context, query capturemodel.ListQuery) (beerlabelmodel.ListResponse, error)
}

func listBeerLabelRecognitions(service BeerLabelRecognitionLister, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(beerlabelservice.InternalError), "internal server error")
			return
		}

		query, ok := readCaptureListQuery(w, r)
		if !ok {
			return
		}

		loggerOrNop(log).Debug(
			"list beer label recognitions request accepted",
			zap.Int("limit", query.Limit),
			zap.Int("offset", query.Offset),
		)
		response, err := service.ListRecognitions(r.Context(), query)
		if err != nil {
			loggerOrNop(log).Error("list beer label recognitions request failed", zap.Error(err))
			writeBeerLabelError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response)
	}
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
