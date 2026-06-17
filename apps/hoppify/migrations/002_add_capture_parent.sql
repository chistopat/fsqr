-- +goose Up
ALTER TABLE captures
    ADD COLUMN parent_uuid uuid REFERENCES captures(uuid) ON DELETE CASCADE;

CREATE INDEX captures_parent_uuid_idx
    ON captures (parent_uuid)
    WHERE parent_uuid IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS captures_parent_uuid_idx;

ALTER TABLE captures
    DROP COLUMN IF EXISTS parent_uuid;
