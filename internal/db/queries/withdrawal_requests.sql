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