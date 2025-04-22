-- name: ListExchangeOfferItems :many
SELECT *
FROM exchange_offer_items
WHERE offer_id = $1
  AND (sqlc.narg('is_from_poster')::boolean IS NULL OR is_from_poster = sqlc.narg('is_from_poster')::boolean)
ORDER BY created_at DESC;

-- name: CreateExchangeOfferItem :one
INSERT INTO exchange_offer_items (id,
                                  offer_id,
                                  gundam_id,
                                  is_from_poster)
VALUES ($1, $2, $3, $4) RETURNING *;