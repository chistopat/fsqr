package config

import (
	"testing"
	"time"
)

func TestLoadLocalConfig(t *testing.T) {
	t.Setenv("FSQR_ENV", "local")
	t.Setenv("FSQR_CONFIG_DIR", "../../config")
	t.Setenv("FSQR_WEB_MAPBOX_ACCESS_TOKEN", "pk.test")
	t.Setenv("FSQR_WEB_MAPBOX_STYLE", "mapbox/dark-v11")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Database.DSN == "" {
		t.Fatal("expected database dsn to be set")
	}
	if cfg.Database.ConnectTimeout != 5*time.Second {
		t.Fatalf("expected postgres connect timeout 5s, got %s", cfg.Database.ConnectTimeout)
	}
	if cfg.Embeddings.BaseURL == "" {
		t.Fatal("expected embeddings base url to be set")
	}
	if cfg.Embeddings.Model == "" {
		t.Fatal("expected embeddings model to be set")
	}
	if cfg.Web.MapboxAccessToken != "pk.test" {
		t.Fatalf("expected mapbox token from env, got %q", cfg.Web.MapboxAccessToken)
	}
	if cfg.Web.MapboxStyle != "mapbox/dark-v11" {
		t.Fatalf("expected mapbox style from env, got %q", cfg.Web.MapboxStyle)
	}
}
