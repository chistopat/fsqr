//go:build onnxsmoke

package onnx

import (
	"bytes"
	"image"
	"image/jpeg"
	"os"
	"testing"
)

func TestDetectorSmoke(t *testing.T) {
	modelPath := envDefault("HOPPIFY_DETECTOR_MODEL_PATH", "../../../models/sku110k-yolo11-s640.onnx")
	runtimeLibraryPath := os.Getenv("HOPPIFY_DETECTOR_RUNTIME_LIBRARY_PATH")

	file, err := os.Open("../../../tests/fixtures/detect-shelf.jpg")
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	img = reencodeJPEG(t, img)

	detector, err := NewDetector(Config{
		ModelPath:           modelPath,
		RuntimeLibraryPath:  runtimeLibraryPath,
		ImageSize:           640,
		ConfidenceThreshold: 0.25,
		IOUThreshold:        0.7,
		MaxDetections:       300,
	})
	if err != nil {
		t.Fatalf("new detector: %v", err)
	}
	defer func() {
		_ = detector.Close()
	}()

	result, err := detector.Detect(t.Context(), img)
	if err != nil {
		t.Fatalf("detect fixture: %v", err)
	}
	if len(result.Results) == 0 {
		t.Fatalf("expected at least one detection")
	}
	t.Logf("detected %d objects; speed=%+v", len(result.Results), result.Speed)
	for index, detection := range result.Results[:min(5, len(result.Results))] {
		t.Logf("detection[%d]=%+v", index, detection)
	}
}

func reencodeJPEG(t *testing.T, img image.Image) image.Image {
	t.Helper()

	var body bytes.Buffer
	if err := jpeg.Encode(&body, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatalf("encode jpeg: %v", err)
	}
	reencoded, _, err := image.Decode(&body)
	if err != nil {
		t.Fatalf("decode reencoded jpeg: %v", err)
	}

	return reencoded
}

func envDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value != "" {
		return value
	}

	return fallback
}
