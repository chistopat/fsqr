//go:build e2e

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"
)

type detectResponse struct {
	Images   []detectImageResult `json:"images"`
	Metadata detectMetadata      `json:"metadata"`
}

type detectImageResult struct {
	Shape   []int             `json:"shape"`
	Results []detectDetection `json:"results"`
}

type detectDetection struct {
	Class      int       `json:"class"`
	Name       string    `json:"name"`
	Confidence float64   `json:"confidence"`
	Box        detectBox `json:"box"`
}

type detectBox struct {
	X1 float64 `json:"x1"`
	Y1 float64 `json:"y1"`
	X2 float64 `json:"x2"`
	Y2 float64 `json:"y2"`
}

type detectMetadata struct {
	UUID       string `json:"uuid"`
	ImageCount int    `json:"imageCount"`
}

func TestDetectCaptureReturnsRealBoundingBoxes(t *testing.T) {
	db := openDatabase(t)
	truncateCaptures(t, db)
	s3Client := newS3Client(t)
	baseURL := envDefault("BASE_URL", defaultBaseURL)

	response := postCapture(t, baseURL, []multipartFile{{
		Filename:    "detect-shelf.jpg",
		ContentType: "image/jpeg",
		Body:        readDetectFixture(t),
	}})
	if len(response.Captures) != 1 {
		t.Fatalf("expected one capture, got %d", len(response.Captures))
	}

	capture := response.Captures[0]
	row := queryCapture(t, db, capture.UUID)
	t.Cleanup(func() {
		deleteCapture(t, db, capture.UUID)
		deleteObject(t, s3Client, row.Bucket, row.ObjectKey)
	})

	detect := postDetect(t, baseURL, capture.UUID)

	if detect.Metadata.UUID != capture.UUID {
		t.Fatalf("expected metadata uuid %q, got %q", capture.UUID, detect.Metadata.UUID)
	}
	if detect.Metadata.ImageCount != 1 {
		t.Fatalf("expected image count 1, got %d", detect.Metadata.ImageCount)
	}
	if len(detect.Images) != 1 {
		t.Fatalf("expected one image result, got %d", len(detect.Images))
	}

	imageResult := detect.Images[0]
	assertShape(t, imageResult.Shape, []int{1024, 768})
	if len(imageResult.Results) != 24 {
		t.Fatalf("expected 24 detections, got %d", len(imageResult.Results))
	}

	assertDetection(t, imageResult.Results[0], detectDetection{
		Class:      1,
		Name:       "object",
		Confidence: 0.94035,
		Box: detectBox{
			X1: 427.97,
			Y1: 350.98,
			X2: 529.01,
			Y2: 613.88,
		},
	})
	assertDetection(t, imageResult.Results[2], detectDetection{
		Class:      1,
		Name:       "object",
		Confidence: 0.92315,
		Box: detectBox{
			X1: 329.37,
			Y1: 349.24,
			X2: 428.32,
			Y2: 611.58,
		},
	})
}

func postDetect(t *testing.T, baseURL string, captureUUID string) detectResponse {
	t.Helper()

	requestBody, err := json.Marshal(map[string]string{"uuid": captureUUID})
	if err != nil {
		t.Fatalf("marshal detect request: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		baseURL+"/api/v1/detect",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		t.Fatalf("build detect request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("post detect: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read detect response: %v", err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d with body %q", http.StatusOK, response.StatusCode, string(body))
	}

	var detected detectResponse
	if err := json.Unmarshal(body, &detected); err != nil {
		t.Fatalf("decode detect response: %v", err)
	}

	return detected
}

func assertShape(t *testing.T, actual []int, expected []int) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("expected shape %v, got %v", expected, actual)
	}
	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("expected shape %v, got %v", expected, actual)
		}
	}
}

func assertDetection(t *testing.T, actual detectDetection, expected detectDetection) {
	t.Helper()

	if actual.Class != expected.Class {
		t.Fatalf("expected class %d, got %d", expected.Class, actual.Class)
	}
	if actual.Name != expected.Name {
		t.Fatalf("expected name %q, got %q", expected.Name, actual.Name)
	}
	assertApprox(t, "confidence", actual.Confidence, expected.Confidence, 0.03)
	assertApprox(t, "box.x1", actual.Box.X1, expected.Box.X1, 5)
	assertApprox(t, "box.y1", actual.Box.Y1, expected.Box.Y1, 5)
	assertApprox(t, "box.x2", actual.Box.X2, expected.Box.X2, 5)
	assertApprox(t, "box.y2", actual.Box.Y2, expected.Box.Y2, 5)
}

func assertApprox(t *testing.T, field string, actual float64, expected float64, tolerance float64) {
	t.Helper()

	diff := actual - expected
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("expected %s %.5f +/- %.5f, got %.5f", field, expected, tolerance, actual)
	}
}

func readDetectFixture(t *testing.T) []byte {
	t.Helper()

	body, err := os.ReadFile("fixtures/detect-shelf.jpg")
	if err != nil {
		t.Fatalf("read detect fixture: %v", err)
	}

	return body
}
