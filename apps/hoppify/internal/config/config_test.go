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
	t.Setenv("HOPPIFY_DETECTOR_CONFIDENCE_THRESHOLD", "0.4")
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
	assertBeerLabelConfig(t, cfg)
	assertObservabilityConfig(t, cfg)
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

	if cfg.Detector.ModelPath != "models/sku110k-yolo11-s640.onnx" {
		t.Fatalf("expected detector model path from config, got %q", cfg.Detector.ModelPath)
	}
	if cfg.Detector.RuntimeLibraryPath != "" {
		t.Fatalf("expected empty detector runtime library path, got %q", cfg.Detector.RuntimeLibraryPath)
	}
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
