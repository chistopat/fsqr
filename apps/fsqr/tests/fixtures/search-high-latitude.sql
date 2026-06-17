-- Fixture for high-latitude bbox behavior around 80.0,0.0.
-- Requires fixtures/categories-core.sql.
DELETE FROM places;

INSERT INTO places (fsq_place_id, name, lat, lon, category_id, address, locality, region, country)
VALUES
    ('tc-coffee-highlat-center', 'High Latitude Center Coffee', 80.000000, 0.000000, 1001,
     'Polar Center', 'Test Arctic', 'Test Region', 'ZZ'),
    ('tc-coffee-highlat-inside-lon', 'High Latitude Inside Longitude Coffee', 80.000000, 0.040000, 1001,
     'Polar East', 'Test Arctic', 'Test Region', 'ZZ'),
    ('tc-coffee-highlat-outside-lon', 'High Latitude Outside Longitude Coffee', 80.000000, 0.060000, 1001,
     'Polar Far East', 'Test Arctic', 'Test Region', 'ZZ'),
    ('tc-coffee-highlat-outside-lat', 'High Latitude Outside Latitude Coffee', 80.010000, 0.000000, 1001,
     'Polar North', 'Test Arctic', 'Test Region', 'ZZ');
