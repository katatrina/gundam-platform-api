-- name: ListExchangeOfferNotes :many
SELECT *
FROM exchange_offer_notes
WHERE offer_id = $1
ORDER BY created_at DESC;

-- name: CreateExchangeOfferNote :one
INSERT INTO exchange_offer_notes (id,
                                  offer_id,
                                  user_id,
                                  content)
VALUES ($1, $2, $3, $4) RETURNING *;