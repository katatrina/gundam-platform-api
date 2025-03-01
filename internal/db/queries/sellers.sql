-- name: GetSellerByID :one
SELECT *
FROM users
WHERE id = $1 AND role = 'seller';

-- name: ListGundamsBySellerID :many
SELECT *
FROM gundams
WHERE owner_id = $1
ORDER BY created_at DESC;