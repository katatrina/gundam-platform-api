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