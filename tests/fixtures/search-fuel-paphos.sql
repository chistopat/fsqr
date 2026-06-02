-- Fixture for fuel station category search and place search around Paphos center.
-- Requires fixtures/categories-core.sql.
DELETE FROM places;

INSERT INTO places (fsq_place_id, name, lat, lon, category_id, address, locality, region, country)
VALUES
    ('tc-fuel-paphos-center', 'Center Test Fuel Station', 34.772013, 32.429736, 2001,
     '10 Fuel Avenue', 'Paphos', 'Paphos', 'CY'),
    ('tc-fuel-paphos-west', 'West Test Fuel Station', 34.772013, 32.428400, 2001,
     '11 Fuel Avenue', 'Paphos', 'Paphos', 'CY'),
    ('tc-ev-paphos-nearby', 'Nearby Test EV Charger', 34.772020, 32.429740, 2002,
     '12 Charger Avenue', 'Paphos', 'Paphos', 'CY'),
    ('tc-coffee-fuel-distractor', 'Fuel Query Coffee Distractor', 34.772010, 32.429730, 1001,
     '13 Coffee Avenue', 'Paphos', 'Paphos', 'CY');
