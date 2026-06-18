package httpapi

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	captureservice "github.com/chistopat/hoppify/internal/service/captures"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type CaptureCreator interface {
	CreateCaptures(ctx context.Context, files []capturemodel.UploadFile) ([]capturemodel.Response, error)
}

type CaptureLister interface {
	ListCaptures(ctx context.Context, query capturemodel.ListQuery) (capturemodel.ListResponse, error)
}

type CaptureImageProvider interface {
	CaptureImage(ctx context.Context, id uuid.UUID) (capturemodel.Object, error)
}

func listCaptures(service CaptureLister, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
			return
		}

		query, ok := readCaptureListQuery(w, r)
		if !ok {
			return
		}

		loggerOrNop(log).Debug(
			"list captures request accepted",
			zap.Int("limit", query.Limit),
			zap.Int("offset", query.Offset),
		)
		response, err := service.ListCaptures(r.Context(), query)
		if err != nil {
			loggerOrNop(log).Error("list captures request failed", zap.Error(err))
			writeCaptureError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, response)
	}
}

func serveCaptureImage(service CaptureImageProvider, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
			return
		}

		id, err := uuid.Parse(strings.TrimSpace(r.PathValue("uuid")))
		if err != nil {
			writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), "uuid must be a valid UUID")
			return
		}

		object, err := service.CaptureImage(r.Context(), id)
		if err != nil {
			loggerOrNop(log).Error("serve capture image request failed", zap.String("uuid", id.String()), zap.Error(err))
			writeCaptureError(w, err)
			return
		}

		w.Header().Set("Content-Type", object.ContentType)
		w.Header().Set("Cache-Control", "private, max-age=300")
		w.Header().Set("Content-Length", strconv.Itoa(len(object.Body)))
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(object.Body); err != nil {
			loggerOrNop(log).Warn("write capture image response failed", zap.String("uuid", id.String()), zap.Error(err))
		}
	}
}

func createCaptures(service CaptureCreator, limits capturemodel.Limits, log *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if service == nil {
			writeError(w, http.StatusInternalServerError, string(captureservice.InternalError), "internal server error")
			return
		}

		files, ok := readCaptureFiles(w, r, normalizeHTTPLimits(limits))
		if !ok {
			return
		}

		loggerOrNop(log).Info("create captures request accepted", zap.Int("file_count", len(files)))
		captures, err := service.CreateCaptures(r.Context(), files)
		if err != nil {
			loggerOrNop(log).Error("create captures request failed", zap.Int("file_count", len(files)), zap.Error(err))
			writeCaptureError(w, err)
			return
		}

		loggerOrNop(log).Info("create captures request completed", zap.Int("capture_count", len(captures)))
		writeJSON(w, http.StatusCreated, capturemodel.CapturesResponse{Captures: captures})
	}
}

func readCaptureListQuery(w http.ResponseWriter, r *http.Request) (capturemodel.ListQuery, bool) {
	limit, ok := readOptionalPositiveInt(w, r, "limit")
	if !ok {
		return capturemodel.ListQuery{}, false
	}
	offset, ok := readOptionalNonNegativeInt(w, r, "offset")
	if !ok {
		return capturemodel.ListQuery{}, false
	}

	return capturemodel.ListQuery{Limit: limit, Offset: offset}, true
}

func readOptionalPositiveInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), name+" must be a positive integer")
		return 0, false
	}

	return value, true
}

func readOptionalNonNegativeInt(w http.ResponseWriter, r *http.Request, name string) (int, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), name+" must be a non-negative integer")
		return 0, false
	}

	return value, true
}

func readCaptureFiles(
	w http.ResponseWriter,
	r *http.Request,
	limits capturemodel.Limits,
) ([]capturemodel.UploadFile, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, limits.MaxRequestBytes)
	if err := r.ParseMultipartForm(limits.MaxRequestBytes); err != nil {
		writeMultipartError(w, err)
		return nil, false
	}

	if r.MultipartForm == nil || len(r.MultipartForm.File["files"]) == 0 {
		writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), "files field is required")
		return nil, false
	}
	defer func() {
		_ = r.MultipartForm.RemoveAll()
	}()

	fileHeaders := r.MultipartForm.File["files"]
	if len(fileHeaders) > limits.MaxFiles {
		writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), "too many files")
		return nil, false
	}

	files, err := readMultipartFiles(fileHeaders, limits.MaxFileBytes)
	if err != nil {
		writeCaptureError(w, err)
		return nil, false
	}

	return files, true
}

func writeMultipartError(w http.ResponseWriter, err error) {
	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		writeError(w, http.StatusRequestEntityTooLarge, string(captureservice.PayloadTooLarge), "request body is too large")
		return
	}

	writeError(w, http.StatusBadRequest, string(captureservice.InvalidRequest), "invalid multipart form data")
}

func readMultipartFiles(fileHeaders []*multipart.FileHeader, maxFileBytes int64) ([]capturemodel.UploadFile, error) {
	files := make([]capturemodel.UploadFile, 0, len(fileHeaders))
	for _, fileHeader := range fileHeaders {
		file, err := readMultipartFile(fileHeader, maxFileBytes)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}

	return files, nil
}

func readMultipartFile(fileHeader *multipart.FileHeader, maxFileBytes int64) (capturemodel.UploadFile, error) {
	if fileHeader.Size > maxFileBytes {
		return capturemodel.UploadFile{}, captureservice.NewPayloadTooLargeError("file exceeds maximum size")
	}

	file, err := fileHeader.Open()
	if err != nil {
		return capturemodel.UploadFile{}, captureservice.NewInvalidRequestError("invalid multipart file", err)
	}
	defer func() {
		_ = file.Close()
	}()

	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return capturemodel.UploadFile{}, captureservice.NewInvalidRequestError("invalid multipart file", err)
	}
	if int64(len(data)) > maxFileBytes {
		return capturemodel.UploadFile{}, captureservice.NewPayloadTooLargeError("file exceeds maximum size")
	}

	return capturemodel.UploadFile{
		Filename:    fileHeader.Filename,
		ContentType: fileHeader.Header.Get("Content-Type"),
		SizeBytes:   int64(len(data)),
		Data:        data,
	}, nil
}

func normalizeHTTPLimits(limits capturemodel.Limits) capturemodel.Limits {
	if limits.MaxFiles <= 0 {
		limits.MaxFiles = 10
	}
	if limits.MaxFileBytes <= 0 {
		limits.MaxFileBytes = 15 * 1024 * 1024
	}
	if limits.MaxRequestBytes <= 0 {
		limits.MaxRequestBytes = 150 * 1024 * 1024
	}

	return limits
}
