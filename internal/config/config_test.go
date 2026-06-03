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
	t.Setenv("FSQR_WEB_DEFAULT_LAT", "34.790000")
	t.Setenv("FSQR_WEB_DEFAULT_LON", "32.460000")
	t.Setenv("FSQR_WEB_DEFAULT_ZOOM", "11")

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
	if cfg.Web.DefaultLat != 34.790000 {
		t.Fatalf("expected default map lat 34.790000, got %f", cfg.Web.DefaultLat)
	}
	if cfg.Web.DefaultLon != 32.460000 {
		t.Fatalf("expected default map lon 32.460000, got %f", cfg.Web.DefaultLon)
	}
	if cfg.Web.DefaultZoom != 11 {
		t.Fatalf("expected default map zoom 11, got %f", cfg.Web.DefaultZoom)
	}
}
