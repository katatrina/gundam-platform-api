-- name: ListExchangeOfferItems :many
SELECT * FROM exchange_offer_items
WHERE offer_id = $1
ORDER BY created_at DESC;