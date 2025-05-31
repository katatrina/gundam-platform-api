-- name: CreateWithdrawalRequest :one
INSERT INTO withdrawal_requests (id,
                                 user_id,
                                 bank_account_id,
                                 amount,
                                 wallet_entry_id)
VALUES ($1, $2, $3, $4, $5) RETURNING *;

-- name: ListUserWithdrawalRequests :many
SELECT sqlc.embed(wr),
       sqlc.embed(uba)
FROM withdrawal_requests wr
         LEFT JOIN user_bank_accounts uba ON wr.bank_account_id = uba.id
WHERE wr.user_id = sqlc.arg('user_id')
  AND wr.status = COALESCE(sqlc.narg('status'), wr.status)
ORDER BY wr.created_at DESC;

-- name: ListWithdrawalRequests :many
SELECT sqlc.embed(wr),
       sqlc.embed(uba)
FROM withdrawal_requests wr
         LEFT JOIN user_bank_accounts uba ON wr.bank_account_id = uba.id
WHERE wr.status = COALESCE(sqlc.narg('status'), wr.status)
ORDER BY wr.created_at DESC;

-- name: GetWithdrawalRequest :one
SELECT sqlc.embed(wr),
       sqlc.embed(uba)
FROM withdrawal_requests wr
         LEFT JOIN user_bank_accounts uba ON wr.bank_account_id = uba.id
WHERE wr.id = sqlc.arg('id');

-- name: UpdateWithdrawalRequest :one
UPDATE withdrawal_requests
SET status                = COALESCE(sqlc.narg('status'), status),
    processed_by          = COALESCE(sqlc.narg('processed_by'), processed_by),
    processed_at          = COALESCE(sqlc.narg('processed_at'), processed_at),
    rejected_reason       = COALESCE(sqlc.narg('rejected_reason'), rejected_reason),
    transaction_reference = COALESCE(sqlc.narg('transaction_reference'), transaction_reference),
    completed_at          = COALESCE(sqlc.narg('completed_at'), completed_at),
    updated_at            = NOW()
WHERE id = sqlc.arg('id') RETURNING *;