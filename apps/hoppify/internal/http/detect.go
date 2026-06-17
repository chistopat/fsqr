package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"

	"go.uber.org/zap"
)

type DetectorService interface {
	Detect(ctx context.Context, rawUUID string) (detectionmodel.Response, error)
}

func detectObjects(service DetectorService, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(detectservice.InternalError), "internal server error")
			return
		}

		var request detectionmodel.Request
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "invalid json request body")
			return
		}

		loggerOrNop(log).Info("detect request accepted", zap.String("uuid", request.UUID))
		response, err := service.Detect(r.Context(), request.UUID)
		if err != nil {
			loggerOrNop(log).Error("detect request failed", zap.String("uuid", request.UUID), zap.Error(err))
			writeDetectError(w, err)
			return
		}

		loggerOrNop(log).Info(
			"detect request completed",
			zap.String("uuid", request.UUID),
			zap.Int("image_count", response.Metadata.ImageCount),
		)
		writeJSON(w, http.StatusOK, response)
	}
}
