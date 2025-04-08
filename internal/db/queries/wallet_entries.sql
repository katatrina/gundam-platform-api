-- name: CreateWalletEntry :one
INSERT INTO wallet_entries (wallet_id,
                            reference_id,
                            reference_type,
                            entry_type,
                            amount,
                            status,
                            completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetWalletEntryByID :one
SELECT *
FROM wallet_entries
WHERE id = $1;

-- name: UpdateWalletEntryByID :one
UPDATE wallet_entries
SET wallet_id        = COALESCE(sqlc.narg('wallet_id'), wallet_id),
    reference_id      = COALESCE(sqlc.narg('reference_id'), reference_id),
    reference_type    = COALESCE(sqlc.narg('reference_type'), reference_type),
    entry_type        = COALESCE(sqlc.narg('entry_type'), entry_type),
    amount            = COALESCE(sqlc.narg('amount'), amount),
    status            = COALESCE(sqlc.narg('status'), status),
    completed_at      = COALESCE(sqlc.narg('completed_at'), completed_at),
    updated_at        = now()
WHERE id = sqlc.arg('id') RETURNING *;