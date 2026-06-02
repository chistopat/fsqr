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
    SELECT candidate.fsq_place_id,
           candidate.name,
           candidate.category_id,
           candidate.lat,
           candidate.lon,
           candidate.dist
    FROM (
        SELECT p.fsq_place_id,
               p.name,
               p.category_id,
               p.lat,
               p.lon,
               sqrt(
                   power(LEAST(abs(p.lon - $2), 360.0 - abs(p.lon - $2)), 2)
                   + power(p.lat - $3, 2)
               ) AS dist
        FROM places p
        WHERE p.category_id = c.category_id
          AND point(p.lon, p.lat) <@ box(point($6, $4), point(180.0, $5))
        UNION ALL
        SELECT p.fsq_place_id,
               p.name,
               p.category_id,
               p.lat,
               p.lon,
               sqrt(
                   power(LEAST(abs(p.lon - $2), 360.0 - abs(p.lon - $2)), 2)
                   + power(p.lat - $3, 2)
               ) AS dist
        FROM places p
        WHERE p.category_id = c.category_id
          AND point(p.lon, p.lat) <@ box(point(-180.0, $4), point($7, $5))
    ) candidate
    ORDER BY candidate.dist
    LIMIT $8
) p
ORDER BY p.dist
LIMIT $8
