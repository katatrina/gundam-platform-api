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

-- name: GetGundamsByOrderItems :many
SELECT oi.quantity,
       oi.price,
       oi.weight,
       g.name,
       gg.display_name AS grade,
       g.scale
FROM order_items oi
         JOIN gundams g ON oi.gundam_id = g.id
         JOIN gundam_grades gg ON g.grade_id = gg.id
WHERE oi.order_id = $1;