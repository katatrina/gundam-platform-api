-- name: CreateOrderTransaction :one
INSERT INTO order_transactions (order_id,
                                amount,
                                status,
                                buyer_entry_id)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetOrderTransactionByOrderID :one
SELECT *
FROM order_transactions
WHERE order_id = sqlc.arg('order_id');

-- name: UpdateOrderTransaction :one
UPDATE order_transactions
SET amount          = COALESCE(sqlc.narg('amount'), amount),
    status          = COALESCE(sqlc.narg('status'), status),
    buyer_entry_id  = COALESCE(sqlc.narg('buyer_entry_id'), buyer_entry_id),
    seller_entry_id = COALESCE(sqlc.narg('seller_entry_id'), seller_entry_id),
    updated_at      = now()
WHERE order_id = sqlc.arg('order_id') RETURNING *;