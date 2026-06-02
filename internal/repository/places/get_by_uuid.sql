SELECT p.fsq_place_id AS uuid,
       p.name,
       p.lat,
       p.lon,
       c.category_id AS category_fsq_category_id,
       c.category_name,
       c.category_label AS category_path,
       p.address,
       p.locality,
       p.region,
       p.country,
       p.tel,
       p.website,
       p.email,
       p.facebook_id,
       p.instagram,
       p.twitter
FROM places p
JOIN categories c ON c.id = p.category_id
WHERE p.fsq_place_id = $1
LIMIT 1
