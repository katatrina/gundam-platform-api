-- name: ListGundamGrades :many
SELECT *
FROM gundam_grades;

-- name: CreateGundam :one
INSERT INTO gundams (owner_id,
                     name,
                     slug,
                     grade_id,
                     condition,
                     condition_description,
                     manufacturer,
                     weight,
                     scale,
                     description,
                     price)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: StoreGundamImageURL :exec
INSERT INTO gundam_images (gundam_id,
                           url,
                           is_primary)
VALUES ($1, $2, $3);

-- name: CreateGundamAccessory :one
INSERT INTO gundam_accessories (name,
                                gundam_id,
                                quantity)
VALUES ($1, $2, $3) RETURNING *;

-- name: ListGundamsWithFilters :many
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
WHERE gg.slug = COALESCE(sqlc.narg('grade_slug')::text, gg.slug)
  AND (sqlc.narg('status')::text IS NULL OR g.status = sqlc.narg('status')::gundam_status)
ORDER BY g.created_at DESC;

-- name: GetGundamByID :one
SELECT *
FROM gundams
WHERE id = $1;

-- name: GetGundamBySlug :one
SELECT g.id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name             AS grade,
       g.condition,
       g.manufacturer,
       g.scale,
       g.weight,
       g.description,
       g.price,
       g.status,
       (SELECT array_agg(gi.url ORDER BY is_primary DESC, created_at DESC) ::TEXT[]
        FROM gundam_images gi
        WHERE gi.gundam_id = g.id) AS image_urls,
       g.created_at,
       g.updated_at
FROM gundams g
         JOIN users u ON g.owner_id = u.id
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE g.slug = $1
  AND (sqlc.narg('status')::text IS NULL OR g.status = sqlc.narg('status')::gundam_status)
ORDER BY g.created_at DESC;

-- name: CreateAccessory :exec
INSERT INTO gundam_accessories (gundam_id,
                                name,
                                quantity)
VALUES ($1, $2, $3);

-- name: GetGundamAccessories :many
SELECT *
FROM gundam_accessories
WHERE gundam_id = $1;

-- name: UpdateGundam :exec
UPDATE gundams
SET owner_id   = coalesce(sqlc.narg('owner_id'), owner_id),
    status     = coalesce(sqlc.narg('status'), status),
    updated_at = now()
WHERE id = sqlc.arg('id');
