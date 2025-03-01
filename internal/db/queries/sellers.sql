-- name: GetSellerByID :one
SELECT *
FROM users
WHERE id = $1
  AND role = 'seller';

-- name: ListGundamsBySellerID :many
SELECT g.id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name             AS grade,
       g.condition,
       g.condition_description,
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
WHERE owner_id = $1
  AND (sqlc.narg('name')::text IS NULL OR name ILIKE concat('%', sqlc.narg('name')::text, '%'))
ORDER BY g.created_at DESC;