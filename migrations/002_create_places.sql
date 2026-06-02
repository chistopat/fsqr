CREATE TABLE IF NOT EXISTS places (
    fsq_place_id TEXT NOT NULL,
    name TEXT,

    lat DOUBLE PRECISION NOT NULL,
    lon DOUBLE PRECISION NOT NULL,

    category_id BIGINT NOT NULL REFERENCES categories(id),

    date_created DATE,
    date_refreshed DATE,

    address TEXT,
    locality TEXT,
    region TEXT,
    country TEXT,

    tel TEXT,
    website TEXT,
    email TEXT,
    facebook_id BIGINT,
    instagram TEXT,
    twitter TEXT
);

DROP INDEX IF EXISTS places_h3_r6_category_idx;

ALTER TABLE places
    DROP COLUMN IF EXISTS h3_r5,
    DROP COLUMN IF EXISTS h3_r6,
    DROP COLUMN IF EXISTS h3_r7,
    DROP COLUMN IF EXISTS h3_r8;

ALTER TABLE places SET LOGGED;

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE INDEX IF NOT EXISTS places_location_category_gist_idx
    ON places USING gist ((point(lon, lat)), category_id);
