-- Fixture for coffee search, bbox and distance ordering around Paphos center.
-- Requires fixtures/categories-core.sql.
DELETE FROM places;

INSERT INTO places (
    fsq_place_id, name, lat, lon, category_id,
    address, locality, region, country, tel, website, email, facebook_id, instagram, twitter
)
VALUES
    ('tc-coffee-paphos-center', 'Center Test Coffee', 34.772013, 32.429736, 1001,
     '1 Center Street', 'Paphos', 'Paphos', 'CY', '+35726000001', 'https://coffee-center.test', 'center@coffee.test', 100001, 'centercoffee', 'centercoffee'),
    ('tc-coffee-paphos-east', 'East Test Coffee', 34.772013, 32.431000, 1001,
     '2 East Street', 'Paphos', 'Paphos', 'CY', NULL, NULL, NULL, NULL, NULL, NULL),
    ('tc-coffee-paphos-north', 'North Test Coffee', 34.773500, 32.429736, 1001,
     '3 North Street', 'Paphos', 'Paphos', 'CY', NULL, NULL, NULL, NULL, NULL, NULL),
    ('tc-coffee-paphos-outside-500m', 'Outside 500m Test Coffee', 34.778100, 32.429736, 1001,
     '4 Far Street', 'Paphos', 'Paphos', 'CY', NULL, NULL, NULL, NULL, NULL, NULL),
    ('tc-fuel-paphos-distractor', 'Coffee Query Fuel Distractor', 34.772020, 32.429740, 2001,
     '5 Fuel Street', 'Paphos', 'Paphos', 'CY', NULL, NULL, NULL, NULL, NULL, NULL),
    ('tc-library-paphos-distractor', 'Coffee Query Library Distractor', 34.772030, 32.429750, 5001,
     '6 Library Street', 'Paphos', 'Paphos', 'CY', NULL, NULL, NULL, NULL, NULL, NULL);
