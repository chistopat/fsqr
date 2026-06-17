SELECT id,
       category_id,
       category_name,
       category_label,
       category_level,
       (embedding <=> $1::vector)::double precision AS distance
FROM categories
ORDER BY embedding <=> $1::vector
LIMIT $2
