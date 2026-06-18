-- +goose Up
CREATE INDEX IF NOT EXISTS captures_type_created_at_uuid_idx
    ON captures (type, created_at DESC, uuid DESC);

-- +goose Down
DROP INDEX IF EXISTS captures_type_created_at_uuid_idx;
