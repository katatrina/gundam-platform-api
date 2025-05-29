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
ORDER BY created_at DESC;

-- name: GetUserBankAccountByID :one
SELECT *
FROM user_bank_accounts
WHERE id = $1
  AND user_id = $2;