-- name: GetSellerByID :one
SELECT *
FROM users
WHERE id = $1 AND role = 'seller';