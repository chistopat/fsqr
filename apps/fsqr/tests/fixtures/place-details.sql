-- Fixture for GET /api/v1/places/{uuid} details contract.
-- Requires fixtures/categories-core.sql.
DELETE FROM places;

INSERT INTO places (
    fsq_place_id, name, lat, lon, category_id,
    date_created, date_refreshed,
    address, locality, region, country,
    tel, website, email, facebook_id, instagram, twitter
)
VALUES
    ('tc-place-details-full', 'Details Test Coffee', 34.772013, 32.429736, 1001,
     DATE '2026-01-01', DATE '2026-02-01',
     '42 Contract Street', 'Paphos', 'Paphos', 'CY',
     '+35726000042', 'https://details.test', 'hello@details.test', 424242, 'detailscoffee', 'detailscoffee'),
    ('tc-place-details-minimal', 'Minimal Details Test Coffee', 34.773000, 32.430000, 1001,
     NULL, NULL,
     NULL, NULL, NULL, NULL,
     NULL, NULL, NULL, NULL, NULL, NULL);
