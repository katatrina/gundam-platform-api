-- name: CountExchangeOffers :one
SELECT COUNT(*) AS count
FROM exchange_offers
WHERE post_id = $1;

-- name: GetUserExchangeOfferForPost :one
SELECT *
FROM exchange_offers
WHERE post_id = $1
  AND offerer_id = $2
  AND status = 'pending' LIMIT 1;