package ultralytics

import (
	"encoding/json"
	"image"
	"image/color"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDetectPostsMultipartRequestAndParsesBoxes(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPredictRequest(t, r, map[string]string{
			"conf":  "0.15",
			"iou":   "0.15",
			"imgsz": "1280",
		})

		writePredictResponse(t, w, map[string]any{
			"images": []map[string]any{{
				"shape": []int{20, 10},
				"results": []map[string]any{{
					"class":      1,
					"name":       "drink",
					"confidence": 0.876543,
					"box": map[string]any{
						"x1": 1,
						"y1": 2,
						"x2": 8,
						"y2": 18,
					},
				}},
				"speed": map[string]float64{
					"preprocess":  1,
					"inference":   2,
					"postprocess": 3,
				},
			}},
		})
	}))
	defer server.Close()

	client, err := NewClient(Config{
		EndpointURL:         server.URL,
		APIKey:              "test-key",
		ImageSize:           1280,
		ConfidenceThreshold: 0.15,
		IOUThreshold:        0.15,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	result, err := client.Detect(t.Context(), newTestImage(10, 20))
	if err != nil {
		t.Fatalf("detect: %v", err)
	}

	if result.Shape != [2]int{20, 10} {
		t.Fatalf("expected shape from response, got %#v", result.Shape)
	}
	if len(result.Results) != 1 {
		t.Fatalf("expected one detection, got %d", len(result.Results))
	}
	detection := result.Results[0]
	if detection.Class != 1 || detection.Name != "drink" || detection.Confidence != 0.87654 {
		t.Fatalf("unexpected detection: %#v", detection)
	}
	if detection.Box.X1 != 1 || detection.Box.Y1 != 2 || detection.Box.X2 != 8 || detection.Box.Y2 != 18 {
		t.Fatalf("unexpected detection box: %#v", detection.Box)
	}
}

func TestCropRefinerExtractsOBBCrop(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assertPredictRequest(t, r, map[string]string{
			"conf":  "0.25",
			"iou":   "0.7",
			"imgsz": "640",
		})

		writePredictResponse(t, w, map[string]any{
			"images": []map[string]any{{
				"shape": []int{4, 4},
				"results": []map[string]any{{
					"class":      0,
					"name":       "bottle",
					"confidence": 0.91,
					"obb": map[string]any{
						"x1": 1,
						"y1": 0,
						"x2": 3,
						"y2": 0,
						"x3": 3,
						"y3": 4,
						"x4": 1,
						"y4": 4,
					},
				}},
			}},
		})
	}))
	defer server.Close()

	refiner, err := NewCropRefiner(Config{EndpointURL: server.URL, APIKey: "test-key"})
	if err != nil {
		t.Fatalf("new crop refiner: %v", err)
	}

	refined, metadata, applied, err := refiner.RefineCrop(t.Context(), newTestImage(4, 4))
	if err != nil {
		t.Fatalf("refine crop: %v", err)
	}

	if !applied {
		t.Fatalf("expected refiner to apply")
	}
	if refined.Bounds().Dx() != 2 || refined.Bounds().Dy() != 4 {
		t.Fatalf("expected refined dimensions 2x4, got %dx%d", refined.Bounds().Dx(), refined.Bounds().Dy())
	}
	if metadata["applied"] != true || metadata["geometry"] != "obb" || metadata["name"] != "bottle" {
		t.Fatalf("unexpected refiner metadata: %#v", metadata)
	}
}

func assertPredictRequest(t *testing.T, r *http.Request, fields map[string]string) {
	t.Helper()

	if r.Method != http.MethodPost {
		t.Fatalf("expected POST request, got %s", r.Method)
	}
	if r.Header.Get("Authorization") != "Bearer test-key" {
		t.Fatalf("expected bearer auth header, got %q", r.Header.Get("Authorization"))
	}
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse multipart form: %v", err)
	}
	for key, expected := range fields {
		if actual := r.FormValue(key); actual != expected {
			t.Fatalf("expected field %s=%q, got %q", key, expected, actual)
		}
	}
	assertFileField(t, r.MultipartForm, "file")
}

func assertFileField(t *testing.T, form *multipart.Form, field string) {
	t.Helper()

	files := form.File[field]
	if len(files) != 1 {
		t.Fatalf("expected one %s file, got %d", field, len(files))
	}
	if files[0].Filename != "image.jpg" {
		t.Fatalf("expected image.jpg filename, got %q", files[0].Filename)
	}
}

func writePredictResponse(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encode response: %v", err)
	}
}

func newTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x * 40), G: uint8(y * 40), B: 100, A: 255})
		}
	}

	return img
}
