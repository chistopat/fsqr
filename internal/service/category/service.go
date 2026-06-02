package category

import (
	"context"
	"fmt"
	"time"

	categorymodel "github.com/chistopat/fsqr/internal/models/category"
	embeddingmodel "github.com/chistopat/fsqr/internal/models/embedding"
	searchmodel "github.com/chistopat/fsqr/internal/models/search"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	"github.com/chistopat/fsqr/internal/observability"
	normalizerpkg "github.com/chistopat/fsqr/internal/service/search/normalizer"
	rankerpkg "github.com/chistopat/fsqr/internal/service/search/ranker"
	scorerpkg "github.com/chistopat/fsqr/internal/service/search/scorer"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

const minRelativeScore = 0.5

type CategoryRepository interface {
	SearchText(ctx context.Context, text string, limit int) ([]searchmodel.Document, error)
	SearchVector(ctx context.Context, embedding embeddingmodel.Embedding, limit int) ([]searchmodel.Document, error)
}

type Embedder interface {
	EmbedQuery(ctx context.Context, query querymodel.Query) (embeddingmodel.Embedding, error)
}

type Normalizer interface {
	Normalize(documents []searchmodel.Document) []searchmodel.Document
}

type Scorer interface {
	Score(groups ...[]searchmodel.Document) []searchmodel.Document
}

type Ranker interface {
	Rank(documents []searchmodel.Document) []searchmodel.Document
}

type Service struct {
	categories CategoryRepository
	embedder   Embedder
	normalizer Normalizer
	scorer     Scorer
	ranker     Ranker
	logger     *zap.Logger
}

func New(categories CategoryRepository, embedder Embedder, loggers ...*zap.Logger) *Service {
	return &Service{
		categories: categories,
		embedder:   embedder,
		normalizer: normalizerpkg.NewMinMax(),
		scorer:     scorerpkg.NewRRF(),
		ranker:     rankerpkg.NewByScore(),
		logger:     optionalLogger(loggers),
	}
}

func (service *Service) SearchCategories(
	ctx context.Context,
	query querymodel.Query,
) ([]categorymodel.Category, error) {
	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/service/category").Start(
		ctx,
		"search.categories",
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("search.query", query.String()),
			attribute.Int("search.limit", query.Limit()),
		),
	)
	defer span.End()

	log := service.log()
	started := time.Now()

	if service.categories == nil {
		err := fmt.Errorf("category repository is not configured")
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if service.embedder == nil {
		err := fmt.Errorf("embedder is not configured")
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if service.normalizer == nil {
		err := fmt.Errorf("search normalizer is not configured")
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if service.scorer == nil {
		err := fmt.Errorf("search scorer is not configured")
		observability.RecordSpanError(span, err)
		return nil, err
	}
	if service.ranker == nil {
		err := fmt.Errorf("search ranker is not configured")
		observability.RecordSpanError(span, err)
		return nil, err
	}

	var ftsDocuments []searchmodel.Document
	var vectorDocuments []searchmodel.Document

	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error {
		documents, err := service.categories.SearchText(groupCtx, query.String(), query.Limit())
		if err != nil {
			return fmt.Errorf("search categories by fts: %w", err)
		}

		ftsDocuments = documents
		return nil
	})

	group.Go(func() error {
		embedding, err := service.embedder.EmbedQuery(groupCtx, query)
		if err != nil {
			return fmt.Errorf("embed category query: %w", err)
		}

		documents, err := service.categories.SearchVector(groupCtx, embedding, query.Limit())
		if err != nil {
			return fmt.Errorf("search categories by vector: %w", err)
		}

		vectorDocuments = documents
		return nil
	})

	if err := group.Wait(); err != nil {
		err = fmt.Errorf("search categories: %w", err)
		observability.RecordSpanError(span, err)
		return nil, err
	}

	ftsDocuments = service.normalizer.Normalize(ftsDocuments)
	vectorDocuments = service.normalizer.Normalize(vectorDocuments)

	documents := service.scorer.Score(ftsDocuments, vectorDocuments)
	scoredDocuments := len(documents)
	documents = service.ranker.Rank(documents)
	filteredDocuments := searchmodel.Documents(documents).
		FilterByRelativeScore(minRelativeScore).
		Limit(query.Limit())

	categories := filteredDocuments.Categories()
	span.SetAttributes(
		attribute.Int("search.fts_documents", len(ftsDocuments)),
		attribute.Int("search.vector_documents", len(vectorDocuments)),
		attribute.Int("search.scored_documents", scoredDocuments),
		attribute.Int("search.returned_categories", len(categories)),
	)
	log.Debug(
		"search categories response",
		zap.String("query", query.String()),
		zap.Int("limit", query.Limit()),
		zap.Int("fts_documents", len(ftsDocuments)),
		zap.Int("vector_documents", len(vectorDocuments)),
		zap.Int("scored_documents", scoredDocuments),
		zap.Int("returned_categories", len(categories)),
		zap.Any("categories", summarizeCategories(categories)),
		zap.Duration("elapsed", time.Since(started)),
	)

	return categories, nil
}

func (service *Service) log() *zap.Logger {
	if service.logger != nil {
		return service.logger
	}

	return zap.NewNop()
}

func optionalLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}

	return zap.NewNop()
}

type categorySummary struct {
	ID            int64  `json:"id"`
	FSQCategoryID string `json:"fsq_category_id"`
	Name          string `json:"name"`
	Label         string `json:"label"`
	Level         int    `json:"level"`
}

func summarizeCategories(categories []categorymodel.Category) []categorySummary {
	summary := make([]categorySummary, 0, len(categories))
	for _, category := range categories {
		summary = append(summary, categorySummary{
			ID:            category.ID,
			FSQCategoryID: category.FSQCategoryID,
			Name:          category.Name,
			Label:         category.Label,
			Level:         category.Level.Int(),
		})
	}

	return summary
}
