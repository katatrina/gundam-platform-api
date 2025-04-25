-- name: ListGundamGrades :many
SELECT *
FROM gundam_grades;

-- name: CreateGundam :one
INSERT INTO gundams (owner_id,
                     name,
                     slug,
                     grade_id,
                     series,
                     parts_total,
                     material,
                     version,
                     quantity,
                     condition,
                     condition_description,
                     manufacturer,
                     weight,
                     scale,
                     description,
                     price,
                     release_year)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING *;

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
SELECT g.id            AS gundam_id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name AS grade,
       g.series,
       g.parts_total,
       g.material,
       g.version,
       g.quantity,
       g.condition,
       g.condition_description,
       g.manufacturer,
       g.scale,
       g.weight,
       g.description,
       g.price,
       g.release_year,
       g.status,
       g.created_at,
       g.updated_at
FROM gundams g
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE (sqlc.narg('name')::text IS NULL OR g.name ILIKE '%' || sqlc.narg('name')::text || '%')
  AND gg.slug = COALESCE(sqlc.narg('grade_slug')::text, gg.slug)
  AND (sqlc.narg('status')::text IS NULL OR g.status = sqlc.narg('status')::gundam_status)
ORDER BY g.created_at DESC;

-- name: GetGundamByID :one
SELECT *
FROM gundams
WHERE id = $1;

-- name: GetGundamBySlug :one
SELECT g.id            AS gundam_id,
       g.owner_id,
       g.name,
       g.slug,
       gg.display_name AS grade,
       g.series,
       g.parts_total,
       g.material,
       g.version,
       g.quantity,
       g.condition,
       g.condition_description,
       g.manufacturer,
       g.scale,
       g.weight,
       g.description,
       g.price,
       g.release_year,
       g.status,
       g.created_at,
       g.updated_at
FROM gundams g
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE g.slug = $1
  AND (sqlc.narg('status')::text IS NULL OR g.status = sqlc.narg('status')::gundam_status);

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

-- name: ListGundamsByUserID :many
SELECT g.*,
       gg.display_name AS grade
FROM gundams g
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE owner_id = $1
  AND (sqlc.narg('name')::text IS NULL OR g.name ILIKE concat('%', sqlc.narg('name')::text, '%'))
ORDER BY g.created_at DESC, g.updated_at DESC;

-- name: BulkUpdateGundamsForExchange :exec
UPDATE gundams
SET status     = 'for exchange',
    updated_at = NOW() FROM
    (SELECT unnest(sqlc.arg(gundam_ids)::bigint[]) as id) as data
WHERE
    gundams.id = data.id
  AND gundams.status = 'in store'
  AND gundams.owner_id = sqlc.arg(owner_id);

-- name: BulkUpdateGundamsInStore :exec
UPDATE gundams
SET status     = 'in store',
    updated_at = NOW() FROM
    (SELECT unnest(sqlc.arg(gundam_ids)::bigint[]) as id) as data
WHERE
    gundams.id = data.id
  AND gundams.status = 'for exchange'
  AND gundams.owner_id = sqlc.arg(owner_id);

-- name: BulkUpdateGundamsExchanging :exec
UPDATE gundams
SET status     = 'exchanging',
    updated_at = NOW() FROM
    (SELECT unnest(sqlc.arg(gundam_ids)::bigint[]) as id) as data
WHERE
    gundams.id = data.id
  AND gundams.status = 'for exchange'
  AND gundams.owner_id = sqlc.arg(owner_id);
