package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	beerlabelservice "github.com/chistopat/hoppify/internal/service/beerlabels"
	beerservice "github.com/chistopat/hoppify/internal/service/beers"
	captureservice "github.com/chistopat/hoppify/internal/service/captures"
	cropservice "github.com/chistopat/hoppify/internal/service/crops"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"
)

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

const internalErrorCode = "internal_error"

var httpStatusByErrorCode = map[string]int{
	"invalid_request":        http.StatusBadRequest,
	"payload_too_large":      http.StatusRequestEntityTooLarge,
	"unsupported_media_type": http.StatusUnsupportedMediaType,
	"storage_error":          http.StatusBadGateway,
	"not_found":              http.StatusNotFound,
	"model_unavailable":      http.StatusServiceUnavailable,
	"inference_error":        http.StatusInternalServerError,
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeKnownError(w http.ResponseWriter, code, message string) {
	status, ok := httpStatusByErrorCode[code]
	if !ok {
		writeError(w, http.StatusInternalServerError, internalErrorCode, "internal server error")
		return
	}

	writeError(w, status, code, message)
}

func writeCaptureError(w http.ResponseWriter, err error) {
	var captureErr *captureservice.Error
	if !errors.As(err, &captureErr) {
		writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
		return
	}

	writeKnownError(w, string(captureErr.Code), captureErr.Message)
}

func writeDetectError(w http.ResponseWriter, err error) {
	var detectErr *detectservice.Error
	if !errors.As(err, &detectErr) {
		writeError(w, http.StatusInternalServerError, string(detectservice.InternalError), "internal server error")
		return
	}

	writeKnownError(w, string(detectErr.Code), detectErr.Message)
}

func writeCropError(w http.ResponseWriter, err error) {
	var cropErr *cropservice.Error
	if !errors.As(err, &cropErr) {
		writeError(w, http.StatusInternalServerError, string(cropservice.InternalError), "internal server error")
		return
	}

	writeKnownError(w, string(cropErr.Code), cropErr.Message)
}

func writeBeerSearchError(w http.ResponseWriter, err error) {
	var beerErr *beerservice.Error
	if !errors.As(err, &beerErr) {
		writeError(w, http.StatusInternalServerError, string(beerservice.InternalError), "internal server error")
		return
	}

	writeKnownError(w, string(beerErr.Code), beerErr.Message)
}

func writeBeerLabelError(w http.ResponseWriter, err error) {
	var beerLabelErr *beerlabelservice.Error
	if !errors.As(err, &beerLabelErr) {
		writeError(w, http.StatusInternalServerError, string(beerlabelservice.InternalError), "internal server error")
		return
	}

	writeKnownError(w, string(beerLabelErr.Code), beerLabelErr.Message)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}
