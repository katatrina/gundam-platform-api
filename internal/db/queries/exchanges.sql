-- name: CreateExchange :one
INSERT INTO exchanges (id,
                       poster_id,
                       offerer_id,
                       payer_id,
                       compensation_amount,
                       status)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetExchangeByID :one
SELECT *
FROM exchanges
WHERE id = $1 LIMIT 1;

-- name: ListUserExchanges :many
SELECT *
FROM exchanges
WHERE (poster_id = sqlc.arg(user_id) OR offerer_id = sqlc.arg(user_id))
  AND status = coalesce(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: UpdateExchange :one
UPDATE exchanges
SET poster_from_delivery_id  = COALESCE(sqlc.narg('poster_from_delivery_id'), poster_from_delivery_id),
    poster_to_delivery_id    = COALESCE(sqlc.narg('poster_to_delivery_id'), poster_to_delivery_id),
    offerer_from_delivery_id = COALESCE(sqlc.narg('offerer_from_delivery_id'), offerer_from_delivery_id),
    offerer_to_delivery_id   = COALESCE(sqlc.narg('offerer_to_delivery_id'), offerer_to_delivery_id),
    updated_at               = now()
WHERE id = $1 RETURNING *;