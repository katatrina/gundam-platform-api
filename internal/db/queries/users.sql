-- name: CreateUser :one
INSERT INTO users (hashed_password, full_name, email, email_verified, phone_number, phone_number_verified, role,
                   avatar_url)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

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

-- name: CreateUserAddress :one
INSERT INTO user_addresses (user_id, full_name, phone_number, province_name, district_name, ghn_district_id, ward_name,
                            ghn_ward_code,
                            detail, is_primary, is_pickup_address)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: GetUserAddresses :many
SELECT *
FROM user_addresses
WHERE user_id = $1
ORDER BY is_primary DESC, created_at DESC;

-- name: UnsetPrimaryAddress :exec
UPDATE user_addresses
SET is_primary = false
WHERE user_id = $1
  AND is_primary = true;

-- name: UnsetPickupAddress :exec
UPDATE user_addresses
SET is_pickup_address = false
WHERE user_id = $1
  AND is_pickup_address = true;