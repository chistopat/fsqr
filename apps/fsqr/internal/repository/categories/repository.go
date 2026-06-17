package categories

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	"github.com/chistopat/fsqr/internal/models/category/level"
	embeddingmodel "github.com/chistopat/fsqr/internal/models/embedding"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
	"github.com/chistopat/fsqr/internal/observability"

	"github.com/jmoiron/sqlx"
	pgvector "github.com/pgvector/pgvector-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

//go:embed search_fts.sql
var searchFTSSQL string

//go:embed search_vector.sql
var searchVectorSQL string

type Repository struct {
	db     *sqlx.DB
	logger *zap.Logger
}

func New(db *sqlx.DB, loggers ...*zap.Logger) (*Repository, error) {
	if db == nil {
		return nil, fmt.Errorf("categories postgres db is required")
	}

	return &Repository{
		db:     db,
		logger: optionalLogger(loggers),
	}, nil
}

func (repo *Repository) SearchText(
	ctx context.Context,
	text string,
	limit int,
) ([]searchmodel.Document, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("category text search query is empty")
	}

	return repo.selectDocuments(ctx, "fts", searchFTSSQL, text, limit)
}

func (repo *Repository) SearchVector(
	ctx context.Context,
	embedding embeddingmodel.Embedding,
	limit int,
) ([]searchmodel.Document, error) {
	if err := validateLimit(limit); err != nil {
		return nil, err
	}
	if embedding.Len() == 0 {
		return nil, fmt.Errorf("category vector search query is empty")
	}

	return repo.selectVectorDocuments(ctx, pgvector.NewVector(embedding.Values()), limit)
}

func validateLimit(limit int) error {
	if limit <= 0 {
		return fmt.Errorf("category search limit must be positive")
	}

	return nil
}

func (repo *Repository) selectDocuments(
	ctx context.Context,
	operation string,
	statement string,
	args ...any,
) ([]searchmodel.Document, error) {
	return repo.selectMappedDocuments(ctx, repo.db, operation, statement, mapDocument, args...)
}

func (repo *Repository) selectVectorDocuments(
	ctx context.Context,
	embedding pgvector.Vector,
	limit int,
) ([]searchmodel.Document, error) {
	tx, err := repo.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin category vector search transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, "SET LOCAL enable_seqscan = off"); err != nil {
		return nil, fmt.Errorf("set category vector search planner options: %w", err)
	}

	documents, err := repo.selectMappedDocuments(ctx, tx, "vector", searchVectorSQL, mapVectorDocument, embedding, limit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit category vector search transaction: %w", err)
	}

	return documents, nil
}

type documentMapper func(row row, index int) (searchmodel.Document, error)

type documentSelector interface {
	SelectContext(ctx context.Context, dest any, query string, args ...any) error
}

func (repo *Repository) selectMappedDocuments(
	ctx context.Context,
	selector documentSelector,
	operation string,
	statement string,
	mapper documentMapper,
	args ...any,
) ([]searchmodel.Document, error) {
	started := time.Now()
	compactStatement := compactSQL(statement)
	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/repository/categories").Start(
		ctx,
		"categories.repository.search."+operation,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("db.system.name", "postgresql"),
			attribute.String("db.operation.name", "SELECT"),
			attribute.String("db.query.text", compactStatement),
			attribute.String("category.search.operation", operation),
		),
	)
	defer span.End()

	var rows []row
	if err := selector.SelectContext(ctx, &rows, statement, args...); err != nil {
		err = fmt.Errorf("select category matches: %w", err)
		observability.RecordSpanError(span, err)
		repo.log().Debug(
			fmt.Sprintf("category %s query failed", operation),
			zap.String("operation", operation),
			zap.String("sql", compactStatement),
			zap.Any("args", summarizeSQLArgs(args)),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return nil, err
	}

	documents := make([]searchmodel.Document, 0, len(rows))
	for index, row := range rows {
		document, err := mapper(row, index)
		if err != nil {
			observability.RecordSpanError(span, err)
			return nil, err
		}

		documents = append(documents, document)
	}

	repo.log().Debug(
		fmt.Sprintf("category %s query", operation),
		zap.String("operation", operation),
		zap.String("sql", compactStatement),
		zap.Any("args", summarizeSQLArgs(args)),
		zap.Int("rows", len(rows)),
		zap.Duration("elapsed", time.Since(started)),
	)

	span.SetAttributes(attribute.Int("db.response.returned_rows", len(rows)))

	return documents, nil
}

func (repo *Repository) log() *zap.Logger {
	if repo.logger != nil {
		return repo.logger
	}

	return zap.NewNop()
}

func optionalLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}

	return zap.NewNop()
}

type sqlArgSummary struct {
	Position   int    `json:"position"`
	Type       string `json:"type,omitempty"`
	Value      any    `json:"value,omitempty"`
	Dimensions int    `json:"dimensions,omitempty"`
}

func summarizeSQLArgs(args []any) []sqlArgSummary {
	summary := make([]sqlArgSummary, 0, len(args))
	for index, arg := range args {
		item := sqlArgSummary{
			Position: index + 1,
		}

		switch value := arg.(type) {
		case pgvector.Vector:
			item.Type = "vector"
			item.Dimensions = len(value.Slice())
		default:
			item.Value = value
		}

		summary = append(summary, item)
	}

	return summary
}

func compactSQL(statement string) string {
	return strings.Join(strings.Fields(statement), " ")
}

func mapDocument(row row, _ int) (searchmodel.Document, error) {
	categoryLevel, err := level.New(row.CategoryLevel)
	if err != nil {
		return searchmodel.Document{}, fmt.Errorf("decode category %d level: %w", row.ID, err)
	}

	return searchmodel.NewDocument(
		categorymodel.Category{
			ID:            row.ID,
			FSQCategoryID: row.CategoryID,
			Name:          row.CategoryName,
			Label:         row.CategoryLabel,
			Level:         categoryLevel,
		},
		row.Rank,
		row.Score,
	), nil
}

func mapVectorDocument(row row, index int) (searchmodel.Document, error) {
	row.Rank = index + 1
	row.Score = 1 - row.Distance

	return mapDocument(row, index)
}
