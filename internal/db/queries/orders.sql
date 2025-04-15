-- name: ValidateGundamBeforeCheckout :one
SELECT sqlc.embed(g),
       CASE
           WHEN g.id IS NOT NULL AND g.status = 'published'
               THEN true
           ELSE false
           END as valid
FROM gundams g
         JOIN users u ON g.owner_id = u.id
WHERE g.id = $1;

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
                    note)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10) RETURNING *;

-- name: ListPurchaseOrders :many
SELECT *
FROM orders
WHERE buyer_id = $1
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
    updated_at           = now()
WHERE id = sqlc.arg('order_id') RETURNING *;