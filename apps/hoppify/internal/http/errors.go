package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	captureservice "github.com/chistopat/hoppify/internal/service/captures"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"
)

type errorResponse struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{
		Error: apiError{
			Code:    code,
			Message: message,
		},
	})
}

func writeCaptureError(w http.ResponseWriter, err error) {
	var captureErr *captureservice.Error
	if !errors.As(err, &captureErr) {
		writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
		return
	}

	switch captureErr.Code {
	case captureservice.InvalidRequest:
		writeError(w, http.StatusBadRequest, string(captureErr.Code), captureErr.Message)
	case captureservice.PayloadTooLarge:
		writeError(w, http.StatusRequestEntityTooLarge, string(captureErr.Code), captureErr.Message)
	case captureservice.UnsupportedMediaType:
		writeError(w, http.StatusUnsupportedMediaType, string(captureErr.Code), captureErr.Message)
	case captureservice.StorageError:
		writeError(w, http.StatusBadGateway, string(captureErr.Code), captureErr.Message)
	default:
		writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
	}
}

func writeDetectError(w http.ResponseWriter, err error) {
	var detectErr *detectservice.Error
	if !errors.As(err, &detectErr) {
		writeError(w, http.StatusInternalServerError, string(detectservice.InternalError), "internal server error")
		return
	}

	switch detectErr.Code {
	case detectservice.InvalidRequest:
		writeError(w, http.StatusBadRequest, string(detectErr.Code), detectErr.Message)
	case detectservice.UnsupportedMediaType:
		writeError(w, http.StatusUnsupportedMediaType, string(detectErr.Code), detectErr.Message)
	case detectservice.StorageError:
		writeError(w, http.StatusBadGateway, string(detectErr.Code), detectErr.Message)
	case detectservice.NotFound:
		writeError(w, http.StatusNotFound, string(detectErr.Code), detectErr.Message)
	case detectservice.ModelUnavailable:
		writeError(w, http.StatusServiceUnavailable, string(detectErr.Code), detectErr.Message)
	case detectservice.InferenceError:
		writeError(w, http.StatusInternalServerError, string(detectErr.Code), detectErr.Message)
	default:
		writeError(w, http.StatusInternalServerError, string(detectservice.InternalError), "internal server error")
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		http.Error(w, "encode response", http.StatusInternalServerError)
	}
}
