SELECT p.fsq_place_id,
       p.name,
       p.category_id,
       p.lat,
       p.lon,
       point(p.lon, p.lat) <-> point($2, $3) AS dist
FROM places p
WHERE p.category_id = ANY ($1::bigint[])
ORDER BY point(p.lon, p.lat) <-> point($2, $3)
LIMIT $4
