-- name: CreateUser :one
INSERT INTO users (hashed_password, email, email_verified)
VALUES ($1, $2, $3) RETURNING *;

-- name: CreateUserWithGoogleAccount :one
INSERT INTO users (id, name, email, email_verified, avatar)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: UpdateUser :one
UPDATE users
SET name = COALESCE(sqlc.narg('name'), name),
    avatar = COALESCE(sqlc.narg('avatar'), avatar)
WHERE id = sqlc.arg('user_id') RETURNING *;