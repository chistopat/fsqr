-- +goose Up
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS untappd_beers (
    untappd_id bigint PRIMARY KEY CHECK (untappd_id > 0),
    url text NOT NULL UNIQUE CHECK (url ~ '^https://untappd\.com/b/[^/]+/[0-9]+$'),

    untappd_slug text NOT NULL,
    brewery_prefix text,

    search_text text NOT NULL,
    last_modified_at timestamptz NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now(),

    CHECK (btrim(untappd_slug) <> ''),
    CHECK (brewery_prefix IS NULL OR btrim(brewery_prefix) <> ''),
    CHECK (btrim(search_text) <> '')
);

CREATE INDEX IF NOT EXISTS untappd_beers_slug_untappd_id_idx
    ON untappd_beers (untappd_slug, untappd_id);

CREATE INDEX IF NOT EXISTS untappd_beers_brewery_prefix_idx
    ON untappd_beers (brewery_prefix)
    WHERE brewery_prefix IS NOT NULL;

CREATE INDEX IF NOT EXISTS untappd_beers_last_modified_at_idx
    ON untappd_beers (last_modified_at DESC, untappd_id DESC);

CREATE INDEX IF NOT EXISTS untappd_beers_search_text_fts_idx
    ON untappd_beers USING gin (to_tsvector('simple', search_text));

CREATE INDEX IF NOT EXISTS untappd_beers_search_text_trgm_idx
    ON untappd_beers USING gin (search_text gin_trgm_ops);

-- +goose Down
DROP TABLE IF EXISTS untappd_beers;
