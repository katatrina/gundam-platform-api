-- name: GetSellerByID :one
SELECT *
FROM users
WHERE id = $1
  AND role = 'seller';

-- name: ListSalesOrders :many
SELECT *
FROM orders
WHERE seller_id = $1
  AND type != 'exchange'
  AND status = COALESCE(sqlc.narg('status')::order_status, status)
ORDER BY updated_at DESC, created_at DESC;

-- name: GetSalesOrder :one
SELECT *
FROM orders
WHERE id = sqlc.arg('order_id')
  AND seller_id = sqlc.arg('seller_id')
  AND type != 'exchange';

-- name: UpdateSellerProfileByID :one
UPDATE seller_profiles
SET shop_name  = COALESCE(sqlc.narg('shop_name'), shop_name),
    updated_at = now()
WHERE seller_id = $1 RETURNING *;

-- name: GetSellerDetailByID :one
SELECT sqlc.embed(u),
       sqlc.embed(sp)
FROM users u
         JOIN seller_profiles sp ON u.id = sp.seller_id
WHERE u.id = $1;

-- name: GetSellerProfileByID :one
SELECT *
FROM seller_profiles
WHERE seller_id = $1;

-- name: CreateSellerProfile :one
INSERT INTO seller_profiles (seller_id, shop_name)
VALUES ($1, $2) RETURNING *;
