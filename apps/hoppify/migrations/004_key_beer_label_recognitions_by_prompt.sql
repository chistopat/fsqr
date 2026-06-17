-- +goose Up
ALTER TABLE beer_label_recognitions
    DROP CONSTRAINT beer_label_recognitions_pkey;

ALTER TABLE beer_label_recognitions
    ADD PRIMARY KEY (capture_uuid, prompt_version);

-- +goose Down
DELETE FROM beer_label_recognitions
WHERE prompt_version <> 'beer-label-v1';

ALTER TABLE beer_label_recognitions
    DROP CONSTRAINT beer_label_recognitions_pkey;

ALTER TABLE beer_label_recognitions
    ADD PRIMARY KEY (capture_uuid);
