package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/spf13/viper"
)

const (
	defaultEnv                    = "local"
	defaultConfigDir              = "config"
	defaultDatabaseDSN            = "postgres://hoppify@127.0.0.1:5432/hoppify?sslmode=disable"
	defaultDatabaseConnectTimeout = 5 * time.Second
	defaultS3Bucket               = "hoppify"
	defaultS3Region               = "us-east-1"
	defaultJPEGQuality            = 95
	defaultMaxFiles               = 10
	defaultMaxFileBytes           = 15 * 1024 * 1024
	defaultMaxRequestSize         = 150 * 1024 * 1024
)

type Config struct {
	App           AppConfig           `mapstructure:"app"`
	HTTP          HTTPConfig          `mapstructure:"http"`
	Logger        LoggerConfig        `mapstructure:"logger"`
	Database      DatabaseConfig      `mapstructure:"database"`
	S3            S3Config            `mapstructure:"s3"`
	Upload        UploadConfig        `mapstructure:"upload"`
	Observability ObservabilityConfig `mapstructure:"observability"`
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

type S3Config struct {
	Bucket          string `mapstructure:"bucket"`
	Region          string `mapstructure:"region"`
	EndpointURL     string `mapstructure:"endpoint_url"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	SessionToken    string `mapstructure:"session_token"`
	ForcePathStyle  bool   `mapstructure:"force_path_style"`
}

type UploadConfig struct {
	MaxFiles        int   `mapstructure:"max_files"`
	MaxFileBytes    int64 `mapstructure:"max_file_bytes"`
	MaxRequestBytes int64 `mapstructure:"max_request_bytes"`
	JPEGQuality     int   `mapstructure:"jpeg_quality"`
}

type ObservabilityConfig struct {
	ServiceName string        `mapstructure:"service_name"`
	Metrics     MetricsConfig `mapstructure:"metrics"`
}

type MetricsConfig struct {
	Path string `mapstructure:"path"`
	Addr string `mapstructure:"addr"`
}

func Load() (Config, error) {
	env := firstNonEmpty(os.Getenv("HOPPIFY_ENV"), os.Getenv("APP_ENV"), defaultEnv)
	configFile := os.Getenv("HOPPIFY_CONFIG_FILE")
	configDir := firstNonEmpty(os.Getenv("HOPPIFY_CONFIG_DIR"), os.Getenv("CONFIG_DIR"), defaultConfigDir)

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

func (cfg UploadConfig) Limits() capturemodel.Limits {
	return capturemodel.Limits{
		MaxFiles:        cfg.MaxFiles,
		MaxFileBytes:    cfg.MaxFileBytes,
		MaxRequestBytes: cfg.MaxRequestBytes,
	}
}

func setDefaults(loader *viper.Viper, env string) {
	loader.SetDefault("app.name", "hoppify")
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
	loader.SetDefault("s3.bucket", defaultS3Bucket)
	loader.SetDefault("s3.region", defaultS3Region)
	loader.SetDefault("s3.endpoint_url", "")
	loader.SetDefault("s3.access_key_id", "")
	loader.SetDefault("s3.secret_access_key", "")
	loader.SetDefault("s3.session_token", "")
	loader.SetDefault("s3.force_path_style", false)
	loader.SetDefault("upload.max_files", defaultMaxFiles)
	loader.SetDefault("upload.max_file_bytes", defaultMaxFileBytes)
	loader.SetDefault("upload.max_request_bytes", defaultMaxRequestSize)
	loader.SetDefault("upload.jpeg_quality", defaultJPEGQuality)
	loader.SetDefault("observability.service_name", "hoppify")
	loader.SetDefault("observability.metrics.path", "/metrics")
	loader.SetDefault("observability.metrics.addr", "127.0.0.1:3001")
}

func bindEnv(loader *viper.Viper) {
	_ = loader.BindEnv("app.env", "HOPPIFY_ENV", "APP_ENV")
	_ = loader.BindEnv("http.addr", "HOPPIFY_HTTP_ADDR", "HTTP_ADDR")
	_ = loader.BindEnv("logger.level", "HOPPIFY_LOGGER_LEVEL", "LOGGER_LEVEL")
	_ = loader.BindEnv("logger.encoding", "HOPPIFY_LOGGER_ENCODING", "LOGGER_ENCODING")
	_ = loader.BindEnv("logger.development", "HOPPIFY_LOGGER_DEVELOPMENT", "LOGGER_DEVELOPMENT")
	_ = loader.BindEnv("database.dsn", "HOPPIFY_DATABASE_DSN", "DATABASE_URL")
	_ = loader.BindEnv("database.max_open_conns", "HOPPIFY_DATABASE_MAX_OPEN_CONNS")
	_ = loader.BindEnv("database.max_idle_conns", "HOPPIFY_DATABASE_MAX_IDLE_CONNS")
	_ = loader.BindEnv("database.conn_max_lifetime", "HOPPIFY_DATABASE_CONN_MAX_LIFETIME")
	_ = loader.BindEnv("database.conn_max_idle_time", "HOPPIFY_DATABASE_CONN_MAX_IDLE_TIME")
	_ = loader.BindEnv("database.connect_timeout", "HOPPIFY_DATABASE_CONNECT_TIMEOUT")
	_ = loader.BindEnv("s3.bucket", "HOPPIFY_S3_BUCKET")
	_ = loader.BindEnv("s3.region", "HOPPIFY_S3_REGION")
	_ = loader.BindEnv("s3.endpoint_url", "HOPPIFY_S3_ENDPOINT_URL", "AWS_ENDPOINT_URL_S3")
	_ = loader.BindEnv("s3.access_key_id", "HOPPIFY_S3_ACCESS_KEY_ID", "AWS_ACCESS_KEY_ID")
	_ = loader.BindEnv("s3.secret_access_key", "HOPPIFY_S3_SECRET_ACCESS_KEY", "AWS_SECRET_ACCESS_KEY")
	_ = loader.BindEnv("s3.session_token", "HOPPIFY_S3_SESSION_TOKEN", "AWS_SESSION_TOKEN")
	_ = loader.BindEnv("s3.force_path_style", "HOPPIFY_S3_FORCE_PATH_STYLE")
	_ = loader.BindEnv("upload.max_files", "HOPPIFY_UPLOAD_MAX_FILES")
	_ = loader.BindEnv("upload.max_file_bytes", "HOPPIFY_UPLOAD_MAX_FILE_BYTES")
	_ = loader.BindEnv("upload.max_request_bytes", "HOPPIFY_UPLOAD_MAX_REQUEST_BYTES")
	_ = loader.BindEnv("upload.jpeg_quality", "HOPPIFY_UPLOAD_JPEG_QUALITY")
	_ = loader.BindEnv("observability.service_name", "HOPPIFY_SERVICE_NAME", "OTEL_SERVICE_NAME")
	_ = loader.BindEnv("observability.metrics.path", "HOPPIFY_METRICS_PATH")
	_ = loader.BindEnv("observability.metrics.addr", "HOPPIFY_METRICS_ADDR")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}
