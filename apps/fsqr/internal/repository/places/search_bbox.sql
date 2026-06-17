WITH category_ids AS (
    SELECT DISTINCT category_id
    FROM unnest($1::bigint[]) AS c(category_id)
    WHERE category_id IS NOT NULL
)
SELECT p.fsq_place_id,
       p.name,
       p.category_id,
       p.lat,
       p.lon,
       p.dist
FROM category_ids c
CROSS JOIN LATERAL (
    SELECT p.fsq_place_id,
           p.name,
           p.category_id,
           p.lat,
           p.lon,
           point(p.lon, p.lat) <-> point($2, $3) AS dist
    FROM places p
    WHERE p.category_id = c.category_id
      AND point(p.lon, p.lat) <@ box(point($4, $5), point($6, $7))
    ORDER BY point(p.lon, p.lat) <-> point($2, $3)
    LIMIT $8
) p
ORDER BY p.dist
LIMIT $8
