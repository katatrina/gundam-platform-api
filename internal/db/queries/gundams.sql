-- name: CreateGundam :one
INSERT INTO gundams (owner_id, name, grade_id, condition, manufacturer, scale, description, price, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: ListGundams :many
SELECT *
FROM gundams
ORDER BY created_at DESC;