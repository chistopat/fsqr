package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	cropmodel "github.com/chistopat/hoppify/internal/models/crop"
	cropservice "github.com/chistopat/hoppify/internal/service/crops"

	"go.uber.org/zap"
)

type CropCreator interface {
	CreateCrops(ctx context.Context, request cropmodel.Request) (cropmodel.Response, error)
}

func createCrops(service CropCreator, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(cropservice.InternalError), "internal server error")
			return
		}

		var request cropmodel.Request
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, string(cropservice.InvalidRequest), "invalid json request body")
			return
		}

		loggerOrNop(log).Info(
			"create crops request accepted",
			zap.String("uuid", request.UUID),
			zap.Int("box_count", len(request.Boxes)),
		)
		response, err := service.CreateCrops(r.Context(), request)
		if err != nil {
			loggerOrNop(log).Error("create crops request failed", zap.String("uuid", request.UUID), zap.Error(err))
			writeCropError(w, err)
			return
		}

		loggerOrNop(log).Info(
			"create crops request completed",
			zap.String("uuid", request.UUID),
			zap.Int("crop_count", len(response.Crops)),
		)
		writeJSON(w, http.StatusCreated, response)
	}
}
