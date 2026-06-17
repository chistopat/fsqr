package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"
	detectionmodel "github.com/chistopat/hoppify/internal/models/detection"
	captureservice "github.com/chistopat/hoppify/internal/service/captures"
	detectservice "github.com/chistopat/hoppify/internal/service/detect"
)

func TestHandlerServesPlaceholderPage(t *testing.T) {
	t.Parallel()

	handler := NewHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !strings.Contains(response.Body.String(), "Hoppify") {
		t.Fatalf("expected placeholder page to mention Hoppify, got %q", response.Body.String())
	}
}

func TestHandlerServesLiveResponse(t *testing.T) {
	t.Parallel()

	handler := NewHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/live", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if strings.TrimSpace(response.Body.String()) != `{"status":"ok","service":"hoppify"}` {
		t.Fatalf("unexpected live response: %q", response.Body.String())
	}
}

func TestHandlerServesSwaggerJSON(t *testing.T) {
	t.Parallel()

	handler := NewHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger.json", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected content type: %q", response.Header().Get("Content-Type"))
	}

	var document map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &document); err != nil {
		t.Fatalf("decode swagger json: %v", err)
	}
	if document["openapi"] != "3.0.3" {
		t.Fatalf("expected OpenAPI 3.0.3 document, got %q", document["openapi"])
	}
}

func TestHandlerServesSwaggerViewer(t *testing.T) {
	t.Parallel()

	handler := NewHandler()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !strings.Contains(response.Body.String(), `url: "/swagger.json"`) {
		t.Fatalf("expected swagger viewer to load /swagger.json, got %q", response.Body.String())
	}
}

func TestCreateCapturesAcceptsMultipartFiles(t *testing.T) {
	t.Parallel()

	expectedCaptures := []capturemodel.Response{{
		UUID: "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1",
		Type: "image",
		URI:  "s3://hoppify/captures/image/018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1.jpg",
	}}
	service := &fakeCaptureService{captures: expectedCaptures}
	handler := NewHandler(
		WithCaptureService(service),
		WithCaptureLimits(capturemodel.Limits{MaxFiles: 10, MaxFileBytes: 1024, MaxRequestBytes: 10 * 1024}),
	)
	request := newMultipartRequest(t, []multipartTestFile{{
		filename:    "capture.png",
		contentType: "image/png",
		body:        []byte{1, 2, 3},
	}})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d with body %q", http.StatusCreated, response.Code, response.Body.String())
	}
	if len(service.files) != 1 {
		t.Fatalf("expected service to receive one file, got %d", len(service.files))
	}
	if service.files[0].ContentType != "image/png" {
		t.Fatalf("unexpected content type: %q", service.files[0].ContentType)
	}

	var body capturemodel.CapturesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Captures) != 1 || body.Captures[0] != expectedCaptures[0] {
		t.Fatalf("unexpected captures response: %#v", body.Captures)
	}
}

func TestCreateCapturesRejectsEmptyBatch(t *testing.T) {
	t.Parallel()

	handler := NewHandler(WithCaptureService(&fakeCaptureService{}))
	request := newMultipartRequest(t, nil)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertAPIError(t, response, http.StatusBadRequest, string(captureservice.InvalidRequest))
}

func TestCreateCapturesRejectsTooManyFiles(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		WithCaptureService(&fakeCaptureService{}),
		WithCaptureLimits(capturemodel.Limits{MaxFiles: 1, MaxFileBytes: 1024, MaxRequestBytes: 10 * 1024}),
	)
	request := newMultipartRequest(t, []multipartTestFile{
		{filename: "one.jpg", contentType: "image/jpeg", body: []byte{1}},
		{filename: "two.jpg", contentType: "image/jpeg", body: []byte{2}},
	})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertAPIError(t, response, http.StatusBadRequest, string(captureservice.InvalidRequest))
}

func TestCreateCapturesRejectsOversizedFile(t *testing.T) {
	t.Parallel()

	handler := NewHandler(
		WithCaptureService(&fakeCaptureService{}),
		WithCaptureLimits(capturemodel.Limits{MaxFiles: 1, MaxFileBytes: 2, MaxRequestBytes: 10 * 1024}),
	)
	request := newMultipartRequest(t, []multipartTestFile{{
		filename:    "large.jpg",
		contentType: "image/jpeg",
		body:        []byte{1, 2, 3},
	}})
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertAPIError(t, response, http.StatusRequestEntityTooLarge, string(captureservice.PayloadTooLarge))
}

func TestDetectObjectsReturnsUltralyticsStyleResponse(t *testing.T) {
	t.Parallel()

	expected := detectionmodel.Response{
		Images: []detectionmodel.ImageResult{{
			Shape: [2]int{720, 1280},
			Results: []detectionmodel.Detection{{
				Class:      0,
				Name:       "object",
				Confidence: 0.91,
				Box:        detectionmodel.Box{X1: 10, Y1: 20, X2: 30, Y2: 40},
			}},
			Speed: detectionmodel.Speed{Preprocess: 1.2, Inference: 12.5, Postprocess: 2.3},
		}},
		Metadata: detectionmodel.Metadata{
			UUID:             "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1",
			ImageCount:       1,
			FunctionTimeCall: 0.018,
		},
	}
	service := &fakeDetectService{response: expected}
	handler := NewHandler(WithDetectService(service))
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/detect",
		strings.NewReader(`{"uuid":"018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"}`),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %q", http.StatusOK, response.Code, response.Body.String())
	}
	if service.uuid != "018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1" {
		t.Fatalf("expected service uuid, got %q", service.uuid)
	}

	var actual detectionmodel.Response
	if err := json.Unmarshal(response.Body.Bytes(), &actual); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if actual.Images[0].Results[0].Box.X2 != expected.Images[0].Results[0].Box.X2 {
		t.Fatalf("expected detection box in response, got %#v", actual.Images[0].Results[0].Box)
	}
}

func TestDetectObjectsRejectsUnknownRequestFields(t *testing.T) {
	t.Parallel()

	handler := NewHandler(WithDetectService(&fakeDetectService{}))
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/detect",
		strings.NewReader(`{"uuid":"018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1","conf":0.1}`),
	)
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	assertAPIError(t, response, http.StatusBadRequest, string(detectservice.InvalidRequest))
}

func TestDetectObjectsMapsServiceErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{
			name:   "not found",
			err:    &detectservice.Error{Code: detectservice.NotFound, Message: "missing"},
			status: http.StatusNotFound,
			code:   string(detectservice.NotFound),
		},
		{
			name:   "model unavailable",
			err:    &detectservice.Error{Code: detectservice.ModelUnavailable, Message: "not ready"},
			status: http.StatusServiceUnavailable,
			code:   string(detectservice.ModelUnavailable),
		},
		{
			name:   "inference error",
			err:    &detectservice.Error{Code: detectservice.InferenceError, Message: "failed"},
			status: http.StatusInternalServerError,
			code:   string(detectservice.InferenceError),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := NewHandler(WithDetectService(&fakeDetectService{err: tc.err}))
			request := httptest.NewRequest(
				http.MethodPost,
				"/api/v1/detect",
				strings.NewReader(`{"uuid":"018f7b8e-4d96-7b42-9f64-09e5d3a8e7c1"}`),
			)
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			assertAPIError(t, response, tc.status, tc.code)
		})
	}
}

func TestMetricsHandlerServesPrometheusMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewMetrics(&fakeCaptureStatsProvider{stats: capturemodel.Stats{
		ImageCount:          2,
		ImageSizeBytesTotal: 123,
	}}, nil)
	handler := NewMetricsHandler(metrics.Registry, "/metrics")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
	}
	if !strings.Contains(response.Body.String(), "hoppify_captures_images_total 2") {
		t.Fatalf("expected capture count metric, got %q", response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "hoppify_captures_image_size_bytes_total 123") {
		t.Fatalf("expected capture size metric, got %q", response.Body.String())
	}
}

type fakeCaptureService struct {
	captures []capturemodel.Response
	files    []capturemodel.UploadFile
	err      error
}

type fakeCaptureStatsProvider struct {
	stats capturemodel.Stats
}

type fakeDetectService struct {
	response detectionmodel.Response
	uuid     string
	err      error
}

func (provider *fakeCaptureStatsProvider) CaptureStats(_ context.Context) (capturemodel.Stats, error) {
	return provider.stats, nil
}

func (service *fakeDetectService) Detect(
	_ context.Context,
	rawUUID string,
) (detectionmodel.Response, error) {
	service.uuid = rawUUID
	if service.err != nil {
		return detectionmodel.Response{}, service.err
	}

	return service.response, nil
}

func (service *fakeCaptureService) CreateCaptures(
	_ context.Context,
	files []capturemodel.UploadFile,
) ([]capturemodel.Response, error) {
	service.files = files
	if service.err != nil {
		return nil, service.err
	}

	return service.captures, nil
}

type multipartTestFile struct {
	filename    string
	contentType string
	body        []byte
}

func newMultipartRequest(t *testing.T, files []multipartTestFile) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set(
			"Content-Disposition",
			fmt.Sprintf(`form-data; name="files"; filename="%s"`, file.filename),
		)
		header.Set("Content-Type", file.contentType)

		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("create multipart part: %v", err)
		}
		if _, err := part.Write(file.body); err != nil {
			t.Fatalf("write multipart part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/captures", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())

	return request
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()

	if response.Code != status {
		t.Fatalf("expected status %d, got %d with body %q", status, response.Code, response.Body.String())
	}

	var body errorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if body.Error.Code != code {
		t.Fatalf("expected error code %q, got %q", code, body.Error.Code)
	}
}
