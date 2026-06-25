package config

import (
	"testing"
	"time"
)

func TestLoadLocalConfig(t *testing.T) {
	t.Setenv("HOPPIFY_ENV", "local")
	t.Setenv("HOPPIFY_CONFIG_DIR", "../../config")
	t.Setenv("HOPPIFY_DATABASE_DSN", "postgres://hoppify:hoppify@127.0.0.1:5432/hoppify_test?sslmode=disable")
	t.Setenv("HOPPIFY_S3_BUCKET", "test-bucket")
	t.Setenv("HOPPIFY_DETECTOR_PROVIDER", "ultralytics")
	t.Setenv("HOPPIFY_DETECTOR_ENDPOINT_URL", "https://example.test/detect")
	t.Setenv("HOPPIFY_DETECTOR_API_KEY", "test-detector-key")
	t.Setenv("HOPPIFY_DETECTOR_TIMEOUT", "15s")
	t.Setenv("HOPPIFY_DETECTOR_CONFIDENCE_THRESHOLD", "0.4")
	t.Setenv("HOPPIFY_CROP_REFINER_ENABLED", "true")
	t.Setenv("HOPPIFY_CROP_REFINER_ENDPOINT_URL", "https://example.test/obb")
	t.Setenv("HOPPIFY_CROP_REFINER_API_KEY", "test-refiner-key")
	t.Setenv("HOPPIFY_CROP_REFINER_TIMEOUT", "20s")
	t.Setenv("HOPPIFY_CROP_REFINER_IMAGE_SIZE", "512")
	t.Setenv("HOPPIFY_BEER_LABEL_GEMINI_API_KEY", "test-gemini-key")
	t.Setenv("HOPPIFY_BEER_LABEL_GEMINI_MODEL", "gemini-test")
	t.Setenv("HOPPIFY_BEER_LABEL_GEMINI_TIMEOUT", "45s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	assertAppConfig(t, cfg)
	assertDatabaseConfig(t, cfg)
	assertS3Config(t, cfg)
	assertUploadConfig(t, cfg)
	assertDetectorConfig(t, cfg)
	assertCropRefinerConfig(t, cfg)
	assertBeerLabelConfig(t, cfg)
	assertObservabilityConfig(t, cfg)
}

func TestLoadDetectorAdditionalModelPathsFromEnv(t *testing.T) {
	t.Setenv("HOPPIFY_ENV", "local")
	t.Setenv("HOPPIFY_CONFIG_DIR", "../../config")
	t.Setenv("HOPPIFY_DETECTOR_ADDITIONAL_MODEL_PATHS", "models/a.onnx,models/b.onnx")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	expected := []string{
		"models/sku110k-yolo11-s640.onnx",
		"models/a.onnx",
		"models/b.onnx",
	}
	paths := cfg.Detector.ModelPaths()
	if len(paths) != len(expected) {
		t.Fatalf("expected detector model paths %#v, got %#v", expected, paths)
	}
	for index := range expected {
		if paths[index] != expected[index] {
			t.Fatalf("expected detector model paths %#v, got %#v", expected, paths)
		}
	}
}

func assertAppConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.App.Name != "hoppify" {
		t.Fatalf("expected app name hoppify, got %q", cfg.App.Name)
	}
}

func assertDatabaseConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Database.DSN != "postgres://hoppify:hoppify@127.0.0.1:5432/hoppify_test?sslmode=disable" {
		t.Fatalf("expected database dsn from env, got %q", cfg.Database.DSN)
	}
	if cfg.Database.ConnectTimeout != 5*time.Second {
		t.Fatalf("expected postgres connect timeout 5s, got %s", cfg.Database.ConnectTimeout)
	}
}

func assertS3Config(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.S3.Bucket != "test-bucket" {
		t.Fatalf("expected s3 bucket from env, got %q", cfg.S3.Bucket)
	}
}

func assertUploadConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Upload.Limits().MaxFiles != 10 {
		t.Fatalf("expected max files 10, got %d", cfg.Upload.Limits().MaxFiles)
	}
}

func assertDetectorConfig(t *testing.T, cfg Config) {
	t.Helper()

	assertDetectorPaths(t, cfg)
	assertDetectorRemoteConfig(t, cfg)
	assertDetectorInferenceConfig(t, cfg)
}

func assertDetectorPaths(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Detector.Provider != "ultralytics" {
		t.Fatalf("expected detector provider from env, got %q", cfg.Detector.Provider)
	}
	if cfg.Detector.ModelPath != "models/sku110k-yolo11-s640.onnx" {
		t.Fatalf("expected detector model path from config, got %q", cfg.Detector.ModelPath)
	}
	if len(cfg.Detector.AdditionalModelPaths) != 1 ||
		cfg.Detector.AdditionalModelPaths[0] != "models/hoppify-yolo11-640n.onnx" {
		t.Fatalf("expected additional detector model path from config, got %#v", cfg.Detector.AdditionalModelPaths)
	}
	if paths := cfg.Detector.ModelPaths(); len(paths) != 2 ||
		paths[0] != "models/sku110k-yolo11-s640.onnx" ||
		paths[1] != "models/hoppify-yolo11-640n.onnx" {
		t.Fatalf("expected two detector model paths, got %#v", paths)
	}
	if cfg.Detector.RuntimeLibraryPath != "" {
		t.Fatalf("expected empty detector runtime library path, got %q", cfg.Detector.RuntimeLibraryPath)
	}
}

func assertDetectorRemoteConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Detector.EndpointURL != "https://example.test/detect" {
		t.Fatalf("expected detector endpoint url from env, got %q", cfg.Detector.EndpointURL)
	}
	if cfg.Detector.APIKey != "test-detector-key" {
		t.Fatalf("expected detector api key from env")
	}
	if cfg.Detector.Timeout != 15*time.Second {
		t.Fatalf("expected detector timeout 15s, got %s", cfg.Detector.Timeout)
	}
}

func assertDetectorInferenceConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Detector.ImageSize != 640 {
		t.Fatalf("expected detector image size 640, got %d", cfg.Detector.ImageSize)
	}
	if cfg.Detector.ConfidenceThreshold != 0.4 {
		t.Fatalf("expected detector confidence threshold from env, got %f", cfg.Detector.ConfidenceThreshold)
	}
	if cfg.Detector.IOUThreshold != 0.7 {
		t.Fatalf("expected detector iou threshold 0.7, got %f", cfg.Detector.IOUThreshold)
	}
	if cfg.Detector.MaxDetections != 300 {
		t.Fatalf("expected detector max detections 300, got %d", cfg.Detector.MaxDetections)
	}
}

func assertCropRefinerConfig(t *testing.T, cfg Config) {
	t.Helper()

	if !cfg.CropRefiner.Enabled {
		t.Fatalf("expected crop refiner enabled from env")
	}
	if cfg.CropRefiner.EndpointURL != "https://example.test/obb" {
		t.Fatalf("expected crop refiner endpoint url from env, got %q", cfg.CropRefiner.EndpointURL)
	}
	if cfg.CropRefiner.APIKey != "test-refiner-key" {
		t.Fatalf("expected crop refiner api key from env")
	}
	if cfg.CropRefiner.Timeout != 20*time.Second {
		t.Fatalf("expected crop refiner timeout 20s, got %s", cfg.CropRefiner.Timeout)
	}
	if cfg.CropRefiner.ImageSize != 512 {
		t.Fatalf("expected crop refiner image size from env, got %d", cfg.CropRefiner.ImageSize)
	}
	if cfg.CropRefiner.ConfidenceThreshold != 0.25 {
		t.Fatalf("expected crop refiner confidence threshold 0.25, got %f", cfg.CropRefiner.ConfidenceThreshold)
	}
	if cfg.CropRefiner.IOUThreshold != 0.7 {
		t.Fatalf("expected crop refiner iou threshold 0.7, got %f", cfg.CropRefiner.IOUThreshold)
	}
}

func assertBeerLabelConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.BeerLabel.Model != "gpt-5.4-mini" {
		t.Fatalf("expected beer label model gpt-5.4-mini, got %q", cfg.BeerLabel.Model)
	}
	if cfg.BeerLabel.OpenAIBaseURL != "https://api.openai.com/v1" {
		t.Fatalf("expected beer label openai base url, got %q", cfg.BeerLabel.OpenAIBaseURL)
	}
	if cfg.BeerLabel.OpenAITimeout != 30*time.Second {
		t.Fatalf("expected beer label openai timeout 30s, got %s", cfg.BeerLabel.OpenAITimeout)
	}
	if cfg.BeerLabel.GeminiAPIKey != "test-gemini-key" {
		t.Fatalf("expected beer label gemini api key from env")
	}
	if cfg.BeerLabel.GeminiModel != "gemini-test" {
		t.Fatalf("expected beer label gemini model from env, got %q", cfg.BeerLabel.GeminiModel)
	}
	if cfg.BeerLabel.GeminiTimeout != 45*time.Second {
		t.Fatalf("expected beer label gemini timeout 45s, got %s", cfg.BeerLabel.GeminiTimeout)
	}
	if cfg.BeerLabel.RecognitionConcurrency != 4 {
		t.Fatalf("expected beer label recognition concurrency 4, got %d", cfg.BeerLabel.RecognitionConcurrency)
	}
	if cfg.BeerLabel.RecognitionRetries != 2 {
		t.Fatalf("expected beer label recognition retries 2, got %d", cfg.BeerLabel.RecognitionRetries)
	}
	if cfg.BeerLabel.RecognitionRetryDelay != 250*time.Millisecond {
		t.Fatalf("expected beer label recognition retry delay 250ms, got %s", cfg.BeerLabel.RecognitionRetryDelay)
	}
	if cfg.BeerLabel.RecognitionMaxBatchSize != 300 {
		t.Fatalf("expected beer label recognition max batch size 300, got %d", cfg.BeerLabel.RecognitionMaxBatchSize)
	}
}

func assertObservabilityConfig(t *testing.T, cfg Config) {
	t.Helper()

	if cfg.Observability.Metrics.Path != "/metrics" {
		t.Fatalf("expected metrics path /metrics, got %q", cfg.Observability.Metrics.Path)
	}
}
