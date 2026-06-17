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

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.App.Name != "hoppify" {
		t.Fatalf("expected app name hoppify, got %q", cfg.App.Name)
	}
	if cfg.Database.DSN != "postgres://hoppify:hoppify@127.0.0.1:5432/hoppify_test?sslmode=disable" {
		t.Fatalf("expected database dsn from env, got %q", cfg.Database.DSN)
	}
	if cfg.Database.ConnectTimeout != 5*time.Second {
		t.Fatalf("expected postgres connect timeout 5s, got %s", cfg.Database.ConnectTimeout)
	}
	if cfg.S3.Bucket != "test-bucket" {
		t.Fatalf("expected s3 bucket from env, got %q", cfg.S3.Bucket)
	}
	if cfg.Upload.Limits().MaxFiles != 10 {
		t.Fatalf("expected max files 10, got %d", cfg.Upload.Limits().MaxFiles)
	}
	if cfg.Observability.Metrics.Path != "/metrics" {
		t.Fatalf("expected metrics path /metrics, got %q", cfg.Observability.Metrics.Path)
	}
}
