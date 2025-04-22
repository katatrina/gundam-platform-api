-- name: CountExchangeOffers :one
SELECT COUNT(*) AS count
FROM exchange_offers
WHERE post_id = $1;

-- name: GetUserExchangeOfferForPost :one
SELECT *
FROM exchange_offers
WHERE post_id = $1
  AND offerer_id = $2 LIMIT 1;

-- name: ListExchangeOffers :many
SELECT *
FROM exchange_offers
WHERE post_id = $1
ORDER BY created_at DESC, updated_at DESC;

-- name: CreateExchangeOffer :one
INSERT INTO exchange_offers (id,
                             post_id,
                             offerer_id,
                             payer_id,
                             compensation_amount)
VALUES ($1, $2, $3, $4, $5) RETURNING *;