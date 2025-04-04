-- name: CreateOrderDelivery :one
INSERT INTO order_deliveries (order_id,
                              ghn_order_code,
                              expected_delivery_time,
                              status,
                              overall_status,
                              from_delivery_id,
                              to_delivery_id)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetOrderDelivery :one
SELECT *
FROM order_deliveries
WHERE order_id = $1;

-- name: UpdateOrderDelivery :one
UPDATE order_deliveries
SET ghn_order_code         = COALESCE(sqlc.narg('ghn_order_code'), ghn_order_code),
    expected_delivery_time = COALESCE(sqlc.narg('expected_delivery_time'), expected_delivery_time),
    status                 = COALESCE(sqlc.narg('status'), status),
    overall_status         = COALESCE(sqlc.narg('overall_status'), overall_status),
    from_delivery_id       = COALESCE(sqlc.narg('from_delivery_id'), from_delivery_id),
    to_delivery_id         = COALESCE(sqlc.narg('to_delivery_id'), to_delivery_id),
    updated_at             = now()
WHERE id = sqlc.arg('id') RETURNING *;