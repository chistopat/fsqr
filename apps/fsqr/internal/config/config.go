package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	defaultEnv                    = "local"
	defaultConfigDir              = "config"
	defaultEmbeddingsBaseURL      = "http://127.0.0.1:8080/v1"
	defaultEmbeddingsAPIKey       = "tei-local"
	defaultEmbeddingsModel        = "intfloat/multilingual-e5-small"
	defaultEmbeddingsTimeout      = 10 * time.Second
	defaultDatabaseDSN            = "postgres://fsqr:fsqr@127.0.0.1:5432/fsqr?sslmode=disable"
	defaultDatabaseConnectTimeout = 5 * time.Second
	defaultWebMapLat              = 34.790000
	defaultWebMapLon              = 32.460000
	defaultWebMapZoom             = 11
)

type Config struct {
	App           AppConfig           `mapstructure:"app"`
	HTTP          HTTPConfig          `mapstructure:"http"`
	Logger        LoggerConfig        `mapstructure:"logger"`
	Database      DatabaseConfig      `mapstructure:"database"`
	Embeddings    EmbeddingsConfig    `mapstructure:"embeddings"`
	Observability ObservabilityConfig `mapstructure:"observability"`
	Web           WebConfig           `mapstructure:"web"`
}

type AppConfig struct {
	Name string `mapstructure:"name"`
	Env  string `mapstructure:"env"`
}

type HTTPConfig struct {
	Addr string `mapstructure:"addr"`
}

type LoggerConfig struct {
	Level       string `mapstructure:"level"`
	Encoding    string `mapstructure:"encoding"`
	Development bool   `mapstructure:"development"`
}

type DatabaseConfig struct {
	DSN             string        `mapstructure:"dsn"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
}

type EmbeddingsConfig struct {
	BaseURL string        `mapstructure:"base_url"`
	APIKey  string        `mapstructure:"api_key"`
	Model   string        `mapstructure:"model"`
	Timeout time.Duration `mapstructure:"timeout"`
}

type WebConfig struct {
	MapboxAccessToken string  `mapstructure:"mapbox_access_token"`
	MapboxStyle       string  `mapstructure:"mapbox_style"`
	DefaultLat        float64 `mapstructure:"default_lat"`
	DefaultLon        float64 `mapstructure:"default_lon"`
	DefaultZoom       float64 `mapstructure:"default_zoom"`
}

type ObservabilityConfig struct {
	ServiceName string        `mapstructure:"service_name"`
	Metrics     MetricsConfig `mapstructure:"metrics"`
	Tracing     TracingConfig `mapstructure:"tracing"`
}

type MetricsConfig struct {
	Path string `mapstructure:"path"`
	Addr string `mapstructure:"addr"`
}

type TracingConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	OTLPEndpointURL string `mapstructure:"otlp_endpoint_url"`
	OTLPInsecure    bool   `mapstructure:"otlp_insecure"`
}

func Load() (Config, error) {
	env := firstNonEmpty(os.Getenv("FSQR_ENV"), os.Getenv("APP_ENV"), defaultEnv)
	configFile := os.Getenv("FSQR_CONFIG_FILE")
	configDir := firstNonEmpty(os.Getenv("FSQR_CONFIG_DIR"), os.Getenv("CONFIG_DIR"), defaultConfigDir)

	loader := viper.New()
	loader.SetConfigType("yaml")
	loader.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	loader.AutomaticEnv()

	if configFile != "" {
		loader.SetConfigFile(configFile)
	} else {
		loader.SetConfigName(env)
		loader.AddConfigPath(configDir)
	}

	setDefaults(loader, env)
	bindEnv(loader)

	if err := loader.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := loader.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	if cfg.App.Env == "" {
		cfg.App.Env = env
	}
	if cfg.Observability.ServiceName == "" {
		cfg.Observability.ServiceName = cfg.App.Name
	}

	return cfg, nil
}

func setDefaults(loader *viper.Viper, env string) {
	loader.SetDefault("app.name", "fsqr")
	loader.SetDefault("app.env", env)
	loader.SetDefault("http.addr", ":3000")
	loader.SetDefault("logger.level", "info")
	loader.SetDefault("logger.encoding", "json")
	loader.SetDefault("logger.development", false)
	loader.SetDefault("database.dsn", defaultDatabaseDSN)
	loader.SetDefault("database.max_open_conns", 8)
	loader.SetDefault("database.max_idle_conns", 4)
	loader.SetDefault("database.conn_max_lifetime", 30*time.Minute)
	loader.SetDefault("database.conn_max_idle_time", 5*time.Minute)
	loader.SetDefault("database.connect_timeout", defaultDatabaseConnectTimeout)
	loader.SetDefault("embeddings.base_url", defaultEmbeddingsBaseURL)
	loader.SetDefault("embeddings.api_key", defaultEmbeddingsAPIKey)
	loader.SetDefault("embeddings.model", defaultEmbeddingsModel)
	loader.SetDefault("embeddings.timeout", defaultEmbeddingsTimeout)
	loader.SetDefault("observability.service_name", "fsqr")
	loader.SetDefault("observability.metrics.path", "/metrics")
	loader.SetDefault("observability.metrics.addr", "127.0.0.1:3001")
	loader.SetDefault("observability.tracing.enabled", false)
	loader.SetDefault("observability.tracing.otlp_endpoint_url", "")
	loader.SetDefault("observability.tracing.otlp_insecure", true)
	loader.SetDefault("web.mapbox_access_token", "")
	loader.SetDefault("web.mapbox_style", "mapbox/light-v11")
	loader.SetDefault("web.default_lat", defaultWebMapLat)
	loader.SetDefault("web.default_lon", defaultWebMapLon)
	loader.SetDefault("web.default_zoom", defaultWebMapZoom)
}

func bindEnv(loader *viper.Viper) {
	_ = loader.BindEnv("app.env", "FSQR_ENV", "APP_ENV")
	_ = loader.BindEnv("http.addr", "FSQR_HTTP_ADDR", "HTTP_ADDR")
	_ = loader.BindEnv("logger.level", "FSQR_LOGGER_LEVEL", "LOGGER_LEVEL")
	_ = loader.BindEnv("logger.encoding", "FSQR_LOGGER_ENCODING", "LOGGER_ENCODING")
	_ = loader.BindEnv("logger.development", "FSQR_LOGGER_DEVELOPMENT", "LOGGER_DEVELOPMENT")
	_ = loader.BindEnv("database.dsn", "FSQR_DATABASE_DSN", "DATABASE_URL")
	_ = loader.BindEnv("database.max_open_conns", "FSQR_DATABASE_MAX_OPEN_CONNS")
	_ = loader.BindEnv("database.max_idle_conns", "FSQR_DATABASE_MAX_IDLE_CONNS")
	_ = loader.BindEnv("database.conn_max_lifetime", "FSQR_DATABASE_CONN_MAX_LIFETIME")
	_ = loader.BindEnv("database.conn_max_idle_time", "FSQR_DATABASE_CONN_MAX_IDLE_TIME")
	_ = loader.BindEnv("database.connect_timeout", "FSQR_DATABASE_CONNECT_TIMEOUT")
	_ = loader.BindEnv("embeddings.base_url", "FSQR_EMBEDDINGS_BASE_URL", "OPENAI_BASE_URL")
	_ = loader.BindEnv("embeddings.api_key", "FSQR_EMBEDDINGS_API_KEY", "OPENAI_API_KEY")
	_ = loader.BindEnv("embeddings.model", "FSQR_EMBEDDINGS_MODEL")
	_ = loader.BindEnv("embeddings.timeout", "FSQR_EMBEDDINGS_TIMEOUT")
	_ = loader.BindEnv("observability.service_name", "FSQR_SERVICE_NAME", "OTEL_SERVICE_NAME")
	_ = loader.BindEnv("observability.metrics.path", "FSQR_METRICS_PATH")
	_ = loader.BindEnv("observability.metrics.addr", "FSQR_METRICS_ADDR")
	_ = loader.BindEnv("observability.tracing.enabled", "FSQR_TRACING_ENABLED")
	_ = loader.BindEnv(
		"observability.tracing.otlp_endpoint_url",
		"FSQR_TRACING_OTLP_ENDPOINT_URL",
		"OTEL_EXPORTER_OTLP_TRACES_ENDPOINT",
		"OTEL_EXPORTER_OTLP_ENDPOINT",
	)
	_ = loader.BindEnv("observability.tracing.otlp_insecure", "FSQR_TRACING_OTLP_INSECURE", "OTEL_EXPORTER_OTLP_INSECURE")
	_ = loader.BindEnv(
		"web.mapbox_access_token",
		"FSQR_WEB_MAPBOX_ACCESS_TOKEN",
		"MAPBOX_ACCESS_TOKEN",
		"MAPBOX_API_KEY",
	)
	_ = loader.BindEnv("web.mapbox_style", "FSQR_WEB_MAPBOX_STYLE", "MAPBOX_STYLE")
	_ = loader.BindEnv("web.default_lat", "FSQR_WEB_DEFAULT_LAT")
	_ = loader.BindEnv("web.default_lon", "FSQR_WEB_DEFAULT_LON")
	_ = loader.BindEnv("web.default_zoom", "FSQR_WEB_DEFAULT_ZOOM")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
