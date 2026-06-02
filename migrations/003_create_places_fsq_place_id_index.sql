CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS places_fsq_place_id_idx
    ON places (fsq_place_id);
