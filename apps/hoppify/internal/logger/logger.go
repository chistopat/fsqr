package logger

import (
	"fmt"
	"strings"

	"github.com/chistopat/hoppify/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func New(cfg config.LoggerConfig) (*zap.Logger, error) {
	level := zapcore.InfoLevel
	if cfg.Level != "" {
		if err := level.UnmarshalText([]byte(strings.ToLower(cfg.Level))); err != nil {
			return nil, fmt.Errorf("parse logger level: %w", err)
		}
	}

	zapConfig := zap.NewProductionConfig()
	if cfg.Development {
		zapConfig = zap.NewDevelopmentConfig()
	}

	if cfg.Encoding != "" {
		zapConfig.Encoding = cfg.Encoding
	}
	zapConfig.Level = zap.NewAtomicLevelAt(level)
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	return log, nil
}
