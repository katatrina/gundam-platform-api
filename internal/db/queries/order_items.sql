-- name: CreateOrderItem :one
INSERT INTO order_items (order_id,
                         gundam_id,
                         price,
                         quantity,
                         weight)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetOrderItems :many
SELECT *
FROM order_items
WHERE order_id = $1;