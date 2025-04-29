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

-- name: GetExchangeByOrderID :one
SELECT *
FROM exchanges
WHERE poster_order_id = $1
   OR offerer_order_id = $1 LIMIT 1;

-- name: ListUserExchanges :many
SELECT *
FROM exchanges
WHERE (poster_id = sqlc.arg(user_id) OR offerer_id = sqlc.arg(user_id))
  AND status = coalesce(sqlc.narg('status'), status)
ORDER BY created_at DESC;

-- name: UpdateExchange :one
UPDATE exchanges
SET poster_order_id                      = COALESCE(sqlc.narg('poster_order_id'), poster_order_id),
    offerer_order_id                     = COALESCE(sqlc.narg('offerer_order_id'), offerer_order_id),
    status                               = COALESCE(sqlc.narg('status'), status),

    poster_from_delivery_id              = COALESCE(sqlc.narg('poster_from_delivery_id'), poster_from_delivery_id),
    poster_to_delivery_id                = COALESCE(sqlc.narg('poster_to_delivery_id'), poster_to_delivery_id),
    offerer_from_delivery_id             = COALESCE(sqlc.narg('offerer_from_delivery_id'), offerer_from_delivery_id),
    offerer_to_delivery_id               = COALESCE(sqlc.narg('offerer_to_delivery_id'), offerer_to_delivery_id),

    poster_delivery_fee                  = COALESCE(sqlc.narg('poster_delivery_fee'), poster_delivery_fee),
    offerer_delivery_fee                 = COALESCE(sqlc.narg('offerer_delivery_fee'), offerer_delivery_fee),

    poster_delivery_fee_paid             = COALESCE(sqlc.narg('poster_delivery_fee_paid'), poster_delivery_fee_paid),
    offerer_delivery_fee_paid            = COALESCE(sqlc.narg('offerer_delivery_fee_paid'), offerer_delivery_fee_paid),

    poster_order_expected_delivery_time  = COALESCE(sqlc.narg('poster_order_expected_delivery_time'),
                                                    poster_order_expected_delivery_time),
    offerer_order_expected_delivery_time = COALESCE(sqlc.narg('offerer_order_expected_delivery_time'),
                                                    offerer_order_expected_delivery_time),

    poster_order_note                    = COALESCE(sqlc.narg('poster_order_note'), poster_order_note),
    offerer_order_note                   = COALESCE(sqlc.narg('offerer_order_note'), offerer_order_note),

    completed_at                         = COALESCE(sqlc.narg('completed_at'), completed_at),

    canceled_by                          = COALESCE(sqlc.narg('canceled_by'), canceled_by),
    canceled_reason                      = COALESCE(sqlc.narg('canceled_reason'), canceled_reason),

    updated_at                           = now()
WHERE id = $1 RETURNING *;