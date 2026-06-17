-- Fixture for geographic intent around Paphos: beach/park/restaurant separation.
-- Requires fixtures/categories-core.sql.
DELETE FROM places;

INSERT INTO places (fsq_place_id, name, lat, lon, category_id, address, locality, region, country)
VALUES
    ('tc-beach-paphos-near', 'Near Test Beach', 34.772500, 32.425500, 6002,
     'Beach Road', 'Paphos', 'Paphos', 'CY'),
    ('tc-beach-paphos-far', 'Far Test Beach', 34.810000, 32.429736, 6002,
     'Far Beach Road', 'Paphos', 'Paphos', 'CY'),
    ('tc-park-paphos-distractor', 'Beach Query Park Distractor', 34.772400, 32.425400, 6001,
     'Park Road', 'Paphos', 'Paphos', 'CY'),
    ('tc-restaurant-paphos-distractor', 'Beach Query Restaurant Distractor', 34.772300, 32.425300, 7001,
     'Food Road', 'Paphos', 'Paphos', 'CY');
