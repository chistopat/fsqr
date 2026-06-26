package beers

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	beermodel "github.com/chistopat/hoppify/internal/models/beer"

	"go.uber.org/zap"
)

const ftsSearchSQL = `
WITH query AS (
	SELECT
		websearch_to_tsquery('simple', $1) AS ts_query,
		btrim(regexp_replace(lower($1), '[^[:alnum:]]+', ' ', 'g')) AS normalized_query
)
SELECT
	beer.untappd_id,
	beer.url,
	beer.untappd_slug,
	beer.brewery_prefix,
	beer.search_text,
	beer.last_modified_at,
	ts_rank_cd(to_tsvector('simple', beer.search_text), query.ts_query) AS text_rank,
	similarity(beer.search_text, query.normalized_query) AS fuzzy_rank
FROM untappd_beers beer, query
WHERE to_tsvector('simple', beer.search_text) @@ query.ts_query
ORDER BY
	text_rank DESC,
	fuzzy_rank DESC,
	beer.untappd_id
LIMIT $2`

const trigramSearchPrefix = `
WITH query AS (
	SELECT
		websearch_to_tsquery('simple', $1) AS ts_query,
		btrim(regexp_replace(lower($1), '[^[:alnum:]]+', ' ', 'g')) AS normalized_query
)
SELECT
	beer.untappd_id,
	beer.url,
	beer.untappd_slug,
	beer.brewery_prefix,
	beer.search_text,
	beer.last_modified_at,
	ts_rank_cd(to_tsvector('simple', beer.search_text), query.ts_query) AS text_rank,
	similarity(beer.search_text, query.normalized_query) AS fuzzy_rank
FROM untappd_beers beer, query
WHERE query.normalized_query <> ''
	AND beer.search_text % query.normalized_query`

const trigramSearchSuffix = `
ORDER BY
	fuzzy_rank DESC,
	text_rank DESC,
	beer.untappd_id
LIMIT $2`

type Repository struct {
	db  *sql.DB
	log *zap.Logger
}

func New(database *sql.DB, log *zap.Logger) (*Repository, error) {
	if database == nil {
		return nil, fmt.Errorf("beers postgres db is required")
	}

	return &Repository{db: database, log: loggerOrNop(log)}, nil
}

func (repo *Repository) Search(ctx context.Context, query string, limit int) ([]beermodel.Record, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("beer search limit must be positive")
	}

	started := time.Now()
	repo.log.Debug("pg beer search started", zap.String("query", query), zap.Int("limit", limit))

	records, err := repo.search(ctx, "fts", ftsSearchSQL, []any{query, limit}, limit)
	if err != nil {
		return nil, err
	}
	if len(records) < limit {
		statement, args := trigramSearchStatement(query, limit-len(records), recordIDs(records))
		fuzzyRecords, err := repo.search(ctx, "trigram", statement, args, limit-len(records))
		if err != nil {
			return nil, err
		}
		records = append(records, fuzzyRecords...)
	}

	repo.log.Debug(
		"pg beer search completed",
		zap.Int("record_count", len(records)),
		zap.Duration("duration", time.Since(started)),
	)

	return records, nil
}

func (repo *Repository) search(
	ctx context.Context,
	operation string,
	statement string,
	args []any,
	capacity int,
) ([]beermodel.Record, error) {
	started := time.Now()
	rows, err := repo.db.QueryContext(ctx, statement, args...)
	if err != nil {
		repo.log.Error(
			"pg beer search query failed",
			zap.String("operation", operation),
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return nil, fmt.Errorf("query beers by %s: %w", operation, err)
	}
	defer func() {
		_ = rows.Close()
	}()

	records := make([]beermodel.Record, 0, capacity)
	for rows.Next() {
		record, err := scanBeer(rows)
		if err != nil {
			repo.log.Error(
				"pg scan beer failed",
				zap.String("operation", operation),
				zap.Error(err),
				zap.Duration("duration", time.Since(started)),
			)
			return nil, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		repo.log.Error(
			"pg iterate beers failed",
			zap.String("operation", operation),
			zap.Error(err),
			zap.Duration("duration", time.Since(started)),
		)
		return nil, fmt.Errorf("iterate beers by %s: %w", operation, err)
	}

	repo.log.Debug(
		"pg beer search query completed",
		zap.String("operation", operation),
		zap.Int("record_count", len(records)),
		zap.Duration("duration", time.Since(started)),
	)

	return records, nil
}

func trigramSearchStatement(query string, limit int, excludedIDs []int64) (statement string, args []any) {
	args = []any{query, limit}
	if len(excludedIDs) == 0 {
		return trigramSearchPrefix + trigramSearchSuffix, args
	}

	placeholders := make([]string, 0, len(excludedIDs))
	for index, id := range excludedIDs {
		placeholders = append(placeholders, "$"+strconv.Itoa(index+3))
		args = append(args, id)
	}
	statement = trigramSearchPrefix + "\n\tAND beer.untappd_id NOT IN (" +
		strings.Join(placeholders, ", ") +
		")" + trigramSearchSuffix

	return statement, args
}

func recordIDs(records []beermodel.Record) []int64 {
	ids := make([]int64, 0, len(records))
	for _, record := range records {
		ids = append(ids, record.UntappdID)
	}

	return ids
}

type beerScanner interface {
	Scan(dest ...any) error
}

func scanBeer(scanner beerScanner) (beermodel.Record, error) {
	var record beermodel.Record
	var breweryPrefix sql.NullString
	err := scanner.Scan(
		&record.UntappdID,
		&record.URL,
		&record.UntappdSlug,
		&breweryPrefix,
		&record.SearchText,
		&record.LastModifiedAt,
		&record.TextRank,
		&record.FuzzyRank,
	)
	if err != nil {
		return beermodel.Record{}, fmt.Errorf("scan beer: %w", err)
	}
	if breweryPrefix.Valid {
		record.BreweryPrefix = breweryPrefix.String
	}

	return record, nil
}

func loggerOrNop(log *zap.Logger) *zap.Logger {
	if log == nil {
		return zap.NewNop()
	}

	return log
}
