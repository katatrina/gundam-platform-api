-- name: CreateOrderDelivery :one
INSERT INTO order_deliveries (order_id,
                              delivery_tracking_code,
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
SET delivery_tracking_code = COALESCE(sqlc.narg('delivery_tracking_code'), delivery_tracking_code),
    expected_delivery_time = COALESCE(sqlc.narg('expected_delivery_time'), expected_delivery_time),
    status                 = COALESCE(sqlc.narg('status'), status),
    overall_status         = COALESCE(sqlc.narg('overall_status'), overall_status),
    from_delivery_id       = COALESCE(sqlc.narg('from_delivery_id'), from_delivery_id),
    to_delivery_id         = COALESCE(sqlc.narg('to_delivery_id'), to_delivery_id),
    updated_at             = now()
WHERE id = sqlc.arg('id') RETURNING *;

-- name: GetActiveOrderDeliveries :many
SELECT od.id,
       o.id AS order_id,
       od.delivery_tracking_code,
       od.expected_delivery_time,
       od.status,
       od.overall_status,
       od.from_delivery_id,
       od.to_delivery_id,
       od.created_at,
       od.updated_at,
       o.code AS order_code,
       o.buyer_id,
       o.seller_id
FROM order_deliveries od
         JOIN orders o ON od.order_id = o.id::text
WHERE od.overall_status IN ('picking', 'delivering', 'return')
  AND od.delivery_tracking_code IS NOT NULL
ORDER BY od.created_at DESC;
