-- Fixture for antimeridian behavior around 0.0,179.99.
-- Requires fixtures/categories-core.sql.
-- This exposes the current point(lon,lat) limitation: bbox falls back to all longitudes,
-- while ordering is not antimeridian-aware.
DELETE FROM places;

INSERT INTO places (fsq_place_id, name, lat, lon, category_id, address, locality, region, country)
VALUES
    ('tc-coffee-antimeridian-east', 'Antimeridian East Coffee', 0.000000, 179.990000, 1001,
     'East Date Line', 'Date Line Test', 'Test Region', 'ZZ'),
    ('tc-coffee-antimeridian-west', 'Antimeridian West Coffee', 0.000000, -179.990000, 1001,
     'West Date Line', 'Date Line Test', 'Test Region', 'ZZ'),
    ('tc-coffee-antimeridian-zero', 'Zero Meridian False Neighbor Coffee', 0.000000, 0.000000, 1001,
     'Zero Meridian', 'Date Line Test', 'Test Region', 'ZZ');
