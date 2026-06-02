WITH q AS (
    SELECT websearch_to_tsquery('pg_catalog.simple', $1) AS query
),
scored AS (
    SELECT c.id,
           c.category_id,
           c.category_name,
           c.category_label,
           c.category_level,
           ts_rank_cd(c.fts, q.query, 32)::double precision AS score
    FROM categories c
    CROSS JOIN q
    WHERE c.fts @@ q.query
),
ranked AS (
    SELECT id,
           category_id,
           category_name,
           category_label,
           category_level,
           row_number() OVER (ORDER BY score DESC, id ASC)::int AS rank,
           score
    FROM scored
)
SELECT id,
       category_id,
       category_name,
       category_label,
       category_level,
       rank,
       score
FROM ranked
ORDER BY rank ASC
LIMIT $2
