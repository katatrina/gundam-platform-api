-- name: CreateUser :one
INSERT INTO users (hashed_password, email, email_verified)
VALUES ($1, $2, $3) RETURNING *;

-- name: CreateUserWithGoogleAccount :one
INSERT INTO users (id, name, email, email_verified, picture)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;