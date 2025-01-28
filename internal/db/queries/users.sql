-- name: CreateUser :one
INSERT INTO users (hashed_password, email, email_verified)
VALUES ($1, $2, $3) RETURNING *;

-- name: CreateUserWithGoogleAccount :one
INSERT INTO users (id, full_name, email, email_verified, avatar_url)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: GetUserByPhoneNumber :one
SELECT *
FROM users
WHERE phone_number = $1;

-- name: UpdateUser :one
UPDATE users
SET full_name    = COALESCE(sqlc.narg('full_name'), full_name),
    avatar_url   = COALESCE(sqlc.narg('avatar_url'), avatar_url),
    phone_number = COALESCE(sqlc.narg('phone_number'), phone_number),
    updated_at   = now()
WHERE id = sqlc.arg('user_id') RETURNING *;