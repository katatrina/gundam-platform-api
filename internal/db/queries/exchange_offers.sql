-- name: CountExchangeOffers :one
SELECT COUNT(*) AS count
FROM exchange_offers
WHERE post_id = $1;

-- name: GetUserExchangeOfferForPost :one
SELECT *
FROM exchange_offers
WHERE post_id = $1
  AND offerer_id = $2 LIMIT 1;

-- name: GetExchangeOffer :one
SELECT *
FROM exchange_offers
WHERE id = $1 LIMIT 1;

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
                             compensation_amount,
                             negotiations_count,
                             max_negotiations,
                             negotiation_requested,
                             note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: UpdateExchangeOffer :one
UPDATE exchange_offers
SET compensation_amount   = COALESCE(sqlc.narg('compensation_amount'), compensation_amount),
    payer_id              = COALESCE(sqlc.narg('payer_id'), payer_id),
    negotiation_requested = COALESCE(sqlc.narg('negotiation_requested'), negotiation_requested),
    negotiations_count    = COALESCE(sqlc.narg('negotiations_count'), negotiations_count),
    last_negotiation_at   = COALESCE(sqlc.narg('last_negotiation_at'), last_negotiation_at),
    updated_at            = now()
WHERE id = sqlc.arg(id) RETURNING *;

-- name: ListExchangeOffersByPostExcluding :many
SELECT * FROM exchange_offers
WHERE post_id = sqlc.arg(post_id) AND id != sqlc.arg(exclude_offer_id);