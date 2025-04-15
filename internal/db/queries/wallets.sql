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

-- name: AddWalletBalance :one
UPDATE wallets
SET balance    = balance + sqlc.arg(amount),
    updated_at = NOW()
WHERE user_id = sqlc.arg(user_id) RETURNING *;

-- name: AddWalletNonWithdrawableAmount :exec
UPDATE wallets
SET non_withdrawable_amount = non_withdrawable_amount + sqlc.arg(amount),
    updated_at              = NOW()
WHERE user_id = sqlc.arg(user_id);

-- name: TransferNonWithdrawableToBalance :one
UPDATE wallets
SET balance = balance + sqlc.arg(amount),
    non_withdrawable_amount = non_withdrawable_amount - sqlc.arg(amount),
    updated_at = now()
WHERE user_id = sqlc.arg(user_id) RETURNING *;