-- name: CreateOrderTransaction :one
INSERT INTO order_transactions (order_id,
                                amount,
                                status,
                                buyer_entry_id)
VALUES ($1, $2, $3, $4) RETURNING *;
