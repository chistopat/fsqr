package beerlabels

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	beerlabelmodel "github.com/chistopat/hoppify/internal/models/beerlabel"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Repository struct {
	db  *sql.DB
	log *zap.Logger
}

func New(database *sql.DB, log *zap.Logger) (*Repository, error) {
	if database == nil {
		return nil, fmt.Errorf("beer labels postgres db is required")
	}

	return &Repository{db: database, log: loggerOrNop(log)}, nil
}

func (repo *Repository) FindBeerLabelRecognition(
	ctx context.Context,
	captureID uuid.UUID,
	promptVersion string,
) (beerlabelmodel.Record, error) {
	started := time.Now()
	repo.log.Debug(
		"pg find beer label recognition started",
		zap.String("capture_uuid", captureID.String()),
		zap.String("prompt_version", promptVersion),
	)

	var record beerlabelmodel.Record
	var result []byte
	err := repo.db.QueryRowContext(
		ctx,
		`SELECT capture_uuid,
			model,
			prompt_version,
			result,
			created_at
		FROM beer_label_recognitions
		WHERE capture_uuid = $1
			AND prompt_version = $2`,
		captureID.String(),
		promptVersion,
	).Scan(
		&record.CaptureUUID,
		&record.Model,
		&record.PromptVersion,
		&result,
		&record.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		repo.log.Debug(
			"pg find beer label recognition not found",
			zap.String("capture_uuid", captureID.String()),
			zap.String("prompt_version", promptVersion),
		)
		return beerlabelmodel.Record{}, fmt.Errorf(
			"%w: %s %s",
			beerlabelmodel.ErrNotFound,
			captureID.String(),
			promptVersion,
		)
	}
	if err != nil {
		repo.log.Error("pg find beer label recognition failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return beerlabelmodel.Record{}, fmt.Errorf("query beer label recognition: %w", err)
	}
	if err := json.Unmarshal(result, &record.Result); err != nil {
		repo.log.Error("pg decode beer label result failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return beerlabelmodel.Record{}, fmt.Errorf("decode beer label result: %w", err)
	}

	repo.log.Debug(
		"pg find beer label recognition completed",
		zap.String("capture_uuid", captureID.String()),
		zap.String("prompt_version", promptVersion),
		zap.Duration("duration", time.Since(started)),
	)

	return record, nil
}

func (repo *Repository) InsertBeerLabelRecognition(ctx context.Context, record *beerlabelmodel.Record) error {
	if record == nil {
		return fmt.Errorf("beer label recognition record is required")
	}

	started := time.Now()
	repo.log.Info(
		"pg insert beer label recognition started",
		zap.String("capture_uuid", record.CaptureUUID.String()),
		zap.String("prompt_version", record.PromptVersion),
	)

	result, err := json.Marshal(record.Result)
	if err != nil {
		return fmt.Errorf("marshal beer label result: %w", err)
	}

	_, err = repo.db.ExecContext(
		ctx,
		`INSERT INTO beer_label_recognitions (
			capture_uuid,
			model,
			prompt_version,
			result
		) VALUES ($1, $2, $3, $4::jsonb)`,
		record.CaptureUUID.String(),
		record.Model,
		record.PromptVersion,
		string(result),
	)
	if err != nil {
		repo.log.Error(
			"pg insert beer label recognition failed",
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return fmt.Errorf("insert beer label recognition: %w", err)
	}

	repo.log.Info(
		"pg insert beer label recognition completed",
		zap.String("capture_uuid", record.CaptureUUID.String()),
		zap.String("prompt_version", record.PromptVersion),
		zap.Duration("duration", time.Since(started)),
	)

	return nil
}

func loggerOrNop(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}

	return log
}
