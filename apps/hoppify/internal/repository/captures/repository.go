package captures

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	capturemodel "github.com/chistopat/hoppify/internal/models/capture"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Repository struct {
	db  *sql.DB
	log *zap.Logger
}

func New(database *sql.DB, log *zap.Logger) (*Repository, error) {
	if database == nil {
		return nil, fmt.Errorf("captures postgres db is required")
	}

	return &Repository{db: database, log: loggerOrNop(log)}, nil
}

func (repo *Repository) InsertCaptures(ctx context.Context, records []capturemodel.Record) error {
	if len(records) == 0 {
		return nil
	}

	started := time.Now()
	repo.log.Info("pg insert captures started", zap.Int("record_count", len(records)))

	tx, err := repo.db.BeginTx(ctx, nil)
	if err != nil {
		repo.log.Error("pg insert captures failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return fmt.Errorf("begin captures insert transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	for index := range records {
		if err := insertCapture(ctx, tx, &records[index]); err != nil {
			repo.log.Error("pg insert captures failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		repo.log.Error("pg insert captures failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return fmt.Errorf("commit captures insert transaction: %w", err)
	}

	repo.log.Info(
		"pg insert captures completed",
		zap.Int("record_count", len(records)),
		zap.Duration("duration", time.Since(started)),
	)

	return nil
}

func (repo *Repository) FindCaptureByUUID(ctx context.Context, id uuid.UUID) (capturemodel.Record, error) {
	started := time.Now()
	repo.log.Debug("pg find capture started", zap.String("uuid", id.String()))

	var record capturemodel.Record
	var metadata []byte
	err := repo.db.QueryRowContext(
		ctx,
		`SELECT uuid, type, bucket, object_key, content_type, size_bytes, checksum_sha256, metadata
		FROM captures
		WHERE uuid = $1`,
		id.String(),
	).Scan(
		&record.UUID,
		&record.Type,
		&record.Bucket,
		&record.ObjectKey,
		&record.ContentType,
		&record.SizeBytes,
		&record.ChecksumSHA256,
		&metadata,
	)
	if errors.Is(err, sql.ErrNoRows) {
		repo.log.Debug("pg find capture not found", zap.String("uuid", id.String()))
		return capturemodel.Record{}, fmt.Errorf("%w: %s", capturemodel.ErrNotFound, id.String())
	}
	if err != nil {
		repo.log.Error("pg find capture failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return capturemodel.Record{}, fmt.Errorf("query capture by uuid: %w", err)
	}
	if err := json.Unmarshal(metadata, &record.Metadata); err != nil {
		repo.log.Error("pg decode capture metadata failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return capturemodel.Record{}, fmt.Errorf("decode capture metadata: %w", err)
	}

	repo.log.Debug(
		"pg find capture completed",
		zap.String("uuid", id.String()),
		zap.Duration("duration", time.Since(started)),
	)

	return record, nil
}

func (repo *Repository) CaptureStats(ctx context.Context) (capturemodel.Stats, error) {
	started := time.Now()
	repo.log.Debug("pg capture stats started")

	var stats capturemodel.Stats
	err := repo.db.QueryRowContext(
		ctx,
		`SELECT count(*), COALESCE(sum(size_bytes), 0)
		FROM captures
		WHERE type = 'image'`,
	).Scan(&stats.ImageCount, &stats.ImageSizeBytesTotal)
	if err != nil {
		repo.log.Error("pg capture stats failed", zap.Error(err), zap.Duration("duration", time.Since(started)))
		return capturemodel.Stats{}, fmt.Errorf("query capture stats: %w", err)
	}

	repo.log.Debug(
		"pg capture stats completed",
		zap.Int64("image_count", stats.ImageCount),
		zap.Int64("image_size_bytes_total", stats.ImageSizeBytesTotal),
		zap.Duration("duration", time.Since(started)),
	)

	return stats, nil
}

func insertCapture(ctx context.Context, tx *sql.Tx, record *capturemodel.Record) error {
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return fmt.Errorf("marshal capture metadata: %w", err)
	}

	_, err = tx.ExecContext(
		ctx,
		`INSERT INTO captures (
			uuid,
			type,
			bucket,
			object_key,
			content_type,
			size_bytes,
			checksum_sha256,
			metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb)`,
		record.UUID.String(),
		record.Type,
		record.Bucket,
		record.ObjectKey,
		record.ContentType,
		record.SizeBytes,
		record.ChecksumSHA256,
		string(metadata),
	)
	if err != nil {
		return fmt.Errorf("insert capture: %w", err)
	}

	return nil
}

func loggerOrNop(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}

	return log
}
