-- name: CreateOrder :one
INSERT INTO orders (id,
                    code,
                    buyer_id,
                    seller_id,
                    items_subtotal,
                    delivery_fee,
                    total_amount,
                    status,
                    payment_method,
                    type,
                    note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: ListMemberOrders :many
SELECT *
FROM orders
WHERE (buyer_id = $1 OR (type = 'exchange' AND seller_id = $1))
  AND status = COALESCE(sqlc.narg('status')::order_status, status)
ORDER BY updated_at DESC, created_at DESC;

-- name: ConfirmOrderByID :one
UPDATE orders
SET status     = 'packaging',
    updated_at = now()
WHERE id = sqlc.arg('order_id')
  AND seller_id = sqlc.arg('seller_id') RETURNING *;

-- name: GetOrderByID :one
SELECT *
FROM orders
WHERE id = $1;

-- name: UpdateOrder :one
UPDATE orders
SET is_packaged          = COALESCE(sqlc.narg('is_packaged'), is_packaged),
    packaging_image_urls = COALESCE(sqlc.narg('packaging_image_urls'), packaging_image_urls),
    status               = COALESCE(sqlc.narg('status'), status),
    canceled_by          = COALESCE(sqlc.narg('canceled_by'), canceled_by),
    canceled_reason      = COALESCE(sqlc.narg('canceled_reason'), canceled_reason),
    completed_at         = COALESCE(sqlc.narg('completed_at'), completed_at),
    updated_at           = now()
WHERE id = sqlc.arg('order_id') RETURNING *;

-- name: GetOrdersToAutoComplete :many
SELECT o.*
FROM orders o
WHERE o.status = 'delivered'
  AND o.updated_at < $1
ORDER BY o.updated_at ASC;

-- name: GetOrderDetails :one
SELECT od.id,
       o.id      AS order_id,
       o.code    AS order_code,
       o.buyer_id,
       o.seller_id,
       o.items_subtotal,
       o.status  AS order_status,
       od.status as delivery_status,
       od.overall_status,
       od.from_delivery_id,
       od.to_delivery_id,
       od.delivery_tracking_code,
       od.expected_delivery_time,
       od.created_at,
       od.updated_at
FROM order_deliveries od
         JOIN orders o ON od.order_id = o.id
WHERE o.id = $1
  AND od.status IS NOT NULL
  AND od.delivery_tracking_code IS NOT NULL
ORDER BY od.created_at DESC LIMIT 1;
