-- name: CreateOrderItem :one
INSERT INTO order_items (order_id,
                         gundam_id,
                         price,
                         quantity)
VALUES ($1, $2, $3, $4) RETURNING *;