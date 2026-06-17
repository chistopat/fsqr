-- +goose Up
CREATE TABLE IF NOT EXISTS beer_label_recognitions (
    capture_uuid uuid PRIMARY KEY REFERENCES captures(uuid) ON DELETE CASCADE,
    model text not null,
    prompt_version text not null,
    result jsonb not null,
    created_at timestamptz not null default now()
);

-- +goose Down
DROP TABLE IF EXISTS beer_label_recognitions;
