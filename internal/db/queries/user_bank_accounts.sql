-- name: CreateUserBankAccount :one
INSERT INTO user_bank_accounts (id,
                                user_id,
                                account_name,
                                account_number,
                                bank_code,
                                bank_name,
                                bank_short_name)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: ListUserBankAccounts :many
SELECT *
FROM user_bank_accounts
WHERE user_id = $1
  AND deleted_at IS NULL
ORDER BY created_at DESC;

-- name: GetUserBankAccount :one
SELECT *
FROM user_bank_accounts
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL;

-- name: UpdateUserBankAccount :one
UPDATE user_bank_accounts
SET deleted_at = COALESCE(sqlc.narg(deleted_at), deleted_at),
    updated_at = now()
WHERE id = $1
  AND user_id = $2
  AND deleted_at IS NULL RETURNING *;