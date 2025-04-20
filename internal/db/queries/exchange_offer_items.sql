-- name: ListExchangeOfferItems :many
SELECT *
FROM exchange_offer_items
WHERE offer_id = $1
ORDER BY created_at DESC;

-- name: CreateExchangeOfferItem :one
INSERT INTO exchange_offer_items (id,
                                  offer_id,
                                  gundam_id)
VALUES ($1, $2, $3) RETURNING *;