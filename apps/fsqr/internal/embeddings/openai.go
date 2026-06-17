package embeddings

import (
	"context"
	"fmt"
	"strings"
	"time"

	embeddingmodel "github.com/chistopat/fsqr/internal/models/embedding"
	querymodel "github.com/chistopat/fsqr/internal/models/search/query"
	"github.com/chistopat/fsqr/internal/observability"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type OpenAIConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	Timeout time.Duration
}

type OpenAIClient struct {
	client openai.Client
	model  string
	logger *zap.Logger
}

func NewOpenAIClient(config OpenAIConfig, loggers ...*zap.Logger) (*OpenAIClient, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("embeddings base url is required")
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("embeddings api key is required")
	}
	if config.Model == "" {
		return nil, fmt.Errorf("embeddings model is required")
	}
	if config.Timeout <= 0 {
		return nil, fmt.Errorf("embeddings timeout must be positive")
	}

	return &OpenAIClient{
		client: openai.NewClient(
			option.WithBaseURL(strings.TrimRight(config.BaseURL, "/")),
			option.WithAPIKey(config.APIKey),
			option.WithRequestTimeout(config.Timeout),
		),
		model:  config.Model,
		logger: optionalLogger(loggers),
	}, nil
}

func (client *OpenAIClient) EmbedQuery(ctx context.Context, query querymodel.Query) (embeddingmodel.Embedding, error) {
	started := time.Now()
	queryText := query.String()
	ctx, span := otel.Tracer("github.com/chistopat/fsqr/internal/embeddings").Start(
		ctx,
		"embeddings.embed_query",
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(
			attribute.String("embeddings.model", client.model),
			attribute.Int("search.query.limit", query.Limit()),
			attribute.Int("search.query.length", len([]rune(queryText))),
		),
	)
	defer span.End()

	client.log().Debug(
		"create query embedding request",
		zap.String("model", client.model),
		zap.String("query", queryText),
		zap.Int("limit", query.Limit()),
	)

	response, err := client.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
		Input: openai.EmbeddingNewParamsInputUnion{
			OfString: openai.String("query: " + queryText),
		},
		Model:          client.model,
		EncodingFormat: openai.EmbeddingNewParamsEncodingFormatFloat,
	})
	if err != nil {
		err = fmt.Errorf("create query embedding: %w", err)
		observability.RecordSpanError(span, err)
		client.log().Debug(
			"create query embedding failed",
			zap.String("model", client.model),
			zap.String("query", queryText),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return embeddingmodel.Embedding{}, err
	}
	if len(response.Data) != 1 {
		err := fmt.Errorf("expected 1 embedding, got %d", len(response.Data))
		observability.RecordSpanError(span, err)
		client.log().Debug(
			"create query embedding invalid response",
			zap.String("model", client.model),
			zap.String("query", queryText),
			zap.Int("embeddings", len(response.Data)),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return embeddingmodel.Embedding{}, err
	}

	values := make([]float32, len(response.Data[0].Embedding))
	for index, value := range response.Data[0].Embedding {
		values[index] = float32(value)
	}

	embedding, err := embeddingmodel.New(values)
	if err != nil {
		err = fmt.Errorf("decode query embedding: %w", err)
		observability.RecordSpanError(span, err)
		client.log().Debug(
			"decode query embedding failed",
			zap.String("model", client.model),
			zap.String("query", queryText),
			zap.Int("dimensions", len(values)),
			zap.Duration("elapsed", time.Since(started)),
			zap.Error(err),
		)

		return embeddingmodel.Embedding{}, err
	}

	span.SetAttributes(attribute.Int("embeddings.dimensions", embedding.Len()))
	client.log().Debug(
		"create query embedding response",
		zap.String("model", client.model),
		zap.String("query", queryText),
		zap.Int("dimensions", embedding.Len()),
		zap.Duration("elapsed", time.Since(started)),
	)

	return embedding, nil
}

func (client *OpenAIClient) log() *zap.Logger {
	if client.logger != nil {
		return client.logger
	}

	return zap.NewNop()
}

func optionalLogger(loggers []*zap.Logger) *zap.Logger {
	if len(loggers) > 0 && loggers[0] != nil {
		return loggers[0]
	}

	return zap.NewNop()
}
