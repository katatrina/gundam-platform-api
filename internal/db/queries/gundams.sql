-- name: CreateGundam :one
INSERT INTO gundams (owner_id, name, slug, grade_id, condition, manufacturer, scale, description, price, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: ListGundamsWithFilters :many
SELECT g.id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name             AS grade,
       g.condition,
       g.manufacturer,
       g.scale,
       g.description,
       g.price,
       g.status,
       g.created_at,
       g.updated_at,
       (SELECT array_agg(gi.url ORDER BY is_primary DESC, created_at DESC) ::TEXT[]
        FROM gundam_images gi
        WHERE gi.gundam_id = g.id) AS image_urls
FROM gundams g
         JOIN users u ON g.owner_id = u.id
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE gg.slug = COALESCE(sqlc.narg('grade_slug')::text, gg.slug)
ORDER BY g.created_at DESC;

-- name: GetGundamBySlug :one
SELECT g.id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name             AS grade,
       g.condition,
       g.manufacturer,
       g.scale,
       g.description,
       g.price,
       g.status,
       g.created_at,
       g.updated_at,
       (SELECT array_agg(gi.url ORDER BY is_primary DESC, created_at DESC) ::TEXT[]
        FROM gundam_images gi
        WHERE gi.gundam_id = g.id) AS image_urls
FROM gundams g
         JOIN users u ON g.owner_id = u.id
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE g.slug = $1
ORDER BY g.created_at DESC;