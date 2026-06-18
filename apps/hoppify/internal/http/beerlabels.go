package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"
	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	beerlabelservice "github.com/chistopat/hoppify/internal/service/beerlabels"

	"go.uber.org/zap"
)

type BeerLabelIdentifier interface {
	Identify(ctx context.Context, request beerlabelmodel.Request) (beerlabelmodel.Response, error)
}

type BeerLabelBatchIdentifier interface {
	IdentifyBatch(ctx context.Context, request beerlabelmodel.BatchRequest) (beerlabelmodel.BatchResponse, error)
}

type BeerLabelBatchStreamer interface {
	IdentifyBatchStream(
		ctx context.Context,
		request beerlabelmodel.BatchRequest,
		emit func(beerlabelmodel.BatchItem) error,
	) error
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

func identifyBeerLabels(
	service BeerLabelBatchIdentifier,
	streamer BeerLabelBatchStreamer,
	log *zap.Logger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(beerlabelservice.InternalError), "internal server error")
			return
		}

		var request beerlabelmodel.BatchRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, string(beerlabelservice.InvalidRequest), "invalid json request body")
			return
		}

		if acceptsNDJSON(r) {
			streamBeerLabels(w, r, streamer, request, log)
			return
		}

		loggerOrNop(log).Info("beer label batch identify request accepted", zap.Int("uuid_count", len(request.UUIDs)))
		response, err := service.IdentifyBatch(r.Context(), request)
		if err != nil {
			loggerOrNop(log).Error(
				"beer label batch identify request failed",
				zap.Int("uuid_count", len(request.UUIDs)),
				zap.Error(err),
			)
			writeBeerLabelError(w, err)
			return
		}

		loggerOrNop(log).Info(
			"beer label batch identify request completed",
			zap.Int("uuid_count", len(request.UUIDs)),
			zap.Int("recognition_count", len(response.Recognitions)),
		)
		writeJSON(w, http.StatusOK, response)
	}
}

func streamBeerLabels(
	w http.ResponseWriter,
	r *http.Request,
	service BeerLabelBatchStreamer,
	request beerlabelmodel.BatchRequest,
	log *zap.Logger,
) {
	if service == nil {
		writeError(w, http.StatusInternalServerError, string(beerlabelservice.InternalError), "internal server error")
		return
	}

	flusher, _ := w.(http.Flusher)

	started := false
	encoder := json.NewEncoder(w)
	loggerOrNop(log).Info("beer label batch stream request accepted", zap.Int("uuid_count", len(request.UUIDs)))
	err := service.IdentifyBatchStream(r.Context(), request, func(item beerlabelmodel.BatchItem) error {
		if !started {
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusOK)
			started = true
		}
		if err := encoder.Encode(item); err != nil {
			return fmt.Errorf("encode beer label stream item: %w", err)
		}
		if flusher != nil {
			flusher.Flush()
		}

		return nil
	})
	if err != nil {
		if !started {
			writeBeerLabelError(w, err)
			return
		}
		if !errors.Is(r.Context().Err(), context.Canceled) {
			loggerOrNop(log).Error("beer label batch stream failed", zap.Int("uuid_count", len(request.UUIDs)), zap.Error(err))
		}
		return
	}

	loggerOrNop(log).Info("beer label batch stream completed", zap.Int("uuid_count", len(request.UUIDs)))
}

func acceptsNDJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/x-ndjson")
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
