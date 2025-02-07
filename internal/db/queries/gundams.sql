-- name: CreateGundam :one
INSERT INTO gundams (owner_id, name, slug, grade_id, condition, manufacturer, scale, description, price, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: ListGundamsWithFilters :many
SELECT g.id,
       g.name,
       g.slug,
       g.description,
       g.price,
       g.created_at,
       g.updated_at,
       gr.id   as grade_id,
       gr.name as grade_name,
       gr.slug as grade_slug
FROM gundams g
         JOIN gundam_grades gr ON g.grade_id = gr.id
WHERE CASE
          WHEN sqlc.narg('grade_slug')::text IS NULL THEN true
        ELSE gr.slug = sqlc.narg('grade_slug')::text
END
ORDER BY g.created_at DESC;

-- name: GetGundamBySlug :one
SELECT g.id,
       g.owner_id,
       g.name,
       g.slug,
       g.manufacturer,
       g.scale,
       g.condition,
       g.status,
       g.description,
       g.price,
       g.created_at,
       g.updated_at,
       sqlc.embed(gr)
FROM gundams g
         JOIN gundam_grades gr ON g.grade_id = gr.id
WHERE g.slug = $1;