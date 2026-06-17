-- +goose Up
CREATE TABLE IF NOT EXISTS captures (
    uuid uuid primary key,
    type text not null,

    bucket text not null,
    object_key text not null,

    content_type text not null,
    size_bytes bigint not null,
    checksum_sha256 text not null,

    metadata jsonb not null default '{}'::jsonb,

    created_at timestamptz not null default now(),

    unique (bucket, object_key)
);

-- +goose Down
DROP TABLE IF EXISTS captures;
