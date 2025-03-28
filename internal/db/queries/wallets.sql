-- name: CreateWallet :exec
INSERT INTO wallets (user_id)
VALUES ($1);

-- name: GetWalletByUserID :one
SELECT *
FROM wallets
WHERE user_id = $1;

-- name: GetWalletForUpdate :one
SELECT *
FROM wallets
WHERE user_id = $1
    FOR UPDATE;