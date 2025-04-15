-- name: CreateOrderItem :one
INSERT INTO order_items (order_id,
                         gundam_id,
                         name,
                         slug,
                         grade,
                         scale,
                         price,
                         quantity,
                         weight,
                         image_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: ListOrderItems :many
SELECT *
FROM order_items
WHERE order_id = $1;
