-- name: ListExchangeOfferNotes :many
SELECT *
FROM exchange_offer_notes
WHERE offer_id = $1
ORDER BY created_at DESC;