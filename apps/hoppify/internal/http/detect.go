package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"

	"go.uber.org/zap"
)

type DetectorService interface {
	Detect(ctx context.Context, request detectionmodel.Request) (detectionmodel.Response, error)
}

func detectObjects(service DetectorService, limits capturemodel.Limits, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(detectservice.InternalError), "internal server error")
			return
		}

		request, ok := readDetectRequest(w, r, normalizeHTTPLimits(limits))
		if !ok {
			return
		}

		loggerOrNop(log).Info(
			"detect request accepted",
			zap.String("uuid", request.UUID),
			zap.String("url", request.ImageURL()),
			zap.Bool("file", request.File != nil),
		)
		response, err := service.Detect(r.Context(), request)
		if err != nil {
			loggerOrNop(log).Error(
				"detect request failed",
				zap.String("uuid", request.UUID),
				zap.String("url", request.ImageURL()),
				zap.Error(err),
			)
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

func readDetectRequest(
	w http.ResponseWriter,
	r *http.Request,
	limits capturemodel.Limits,
) (detectionmodel.Request, bool) {
	if isMultipartRequest(r) {
		return readDetectMultipartRequest(w, r, limits)
	}

	var request detectionmodel.Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "invalid json request body")
		return detectionmodel.Request{}, false
	}

	return request, true
}

func readDetectMultipartRequest(
	w http.ResponseWriter,
	r *http.Request,
	limits capturemodel.Limits,
) (detectionmodel.Request, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limits.MaxRequestBytes)
	if err := r.ParseMultipartForm(limits.MaxRequestBytes); err != nil {
		writeDetectMultipartError(w, err)
		return detectionmodel.Request{}, false
	}
	if r.MultipartForm == nil {
		writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "file field is required")
		return detectionmodel.Request{}, false
	}
	defer func() {
		_ = r.MultipartForm.RemoveAll()
	}()

	fileHeaders := r.MultipartForm.File["file"]
	if len(fileHeaders) == 0 {
		fileHeaders = r.MultipartForm.File["files"]
	}
	if len(fileHeaders) != 1 {
		writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "exactly one file is required")
		return detectionmodel.Request{}, false
	}

	file, ok := readDetectMultipartFile(w, fileHeaders[0], limits.MaxFileBytes)
	if !ok {
		return detectionmodel.Request{}, false
	}

	return detectionmodel.Request{File: &file}, true
}

func readDetectMultipartFile(
	w http.ResponseWriter,
	fileHeader *multipart.FileHeader,
	maxFileBytes int64,
) (capturemodel.UploadFile, bool) {
	if fileHeader.Size > maxFileBytes {
		writeError(w, http.StatusRequestEntityTooLarge, string(detectservice.InvalidRequest), "file exceeds maximum size")
		return capturemodel.UploadFile{}, false
	}

	file, err := fileHeader.Open()
	if err != nil {
		writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "invalid multipart file")
		return capturemodel.UploadFile{}, false
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "invalid multipart file")
		return capturemodel.UploadFile{}, false
	}
	if int64(len(data)) > maxFileBytes {
		writeError(w, http.StatusRequestEntityTooLarge, string(detectservice.InvalidRequest), "file exceeds maximum size")
		return capturemodel.UploadFile{}, false
	}

	return capturemodel.UploadFile{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		SizeBytes:   int64(len(data)),
		Data:        data,
	}, true
}

func isMultipartRequest(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")
	if contentType == "" {
		return false
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	return strings.HasPrefix(mediaType, "multipart/")
}

func writeDetectMultipartError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, string(detectservice.InvalidRequest), "request body is too large")
		return
	}

	writeError(w, http.StatusBadRequest, string(detectservice.InvalidRequest), "invalid multipart form data")
}
