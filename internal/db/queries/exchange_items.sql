-- name: CreateExchangeItem :one
INSERT INTO exchange_items (id,
                            exchange_id,
                            gundam_id,
                            name,
                            slug,
                            grade,
                            scale,
                            quantity,
                            weight,
                            image_url,
                            owner_id,
                            is_from_poster)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING *;

-- name: ListExchangeItems :many
SELECT *
FROM exchange_items
WHERE exchange_id = $1
  AND (sqlc.narg('is_from_poster')::boolean IS NULL OR is_from_poster = sqlc.narg('is_from_poster')::boolean)
ORDER BY created_at DESC;