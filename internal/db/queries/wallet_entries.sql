-- name: CreateWalletEntry :one
INSERT INTO wallet_entries (wallet_id,
                            reference_id,
                            reference_type,
                            entry_type,
                            affected_field,
                            amount,
                            status,
                            completed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetWalletEntryByID :one
SELECT *
FROM wallet_entries
WHERE id = $1;

-- name: UpdateWalletEntryByID :one
UPDATE wallet_entries
SET reference_id = COALESCE(sqlc.narg('reference_id'), reference_id),
    status       = COALESCE(sqlc.narg('status'), status),
    completed_at = COALESCE(sqlc.narg('completed_at'), completed_at),
    updated_at   = now()
WHERE id = sqlc.arg('id') RETURNING *;

-- name: GetPendingExchangeCompensationEntry :one
SELECT *
FROM wallet_entries
WHERE reference_id = $1
  AND reference_type = 'exchange'
  AND entry_type = 'exchange_compensation_transfer'
  AND status = 'pending'
  AND wallet_id = $2 LIMIT 1;

-- name: ListUserWalletEntries :many
SELECT *
FROM wallet_entries
WHERE wallet_id = $1
  AND status = COALESCE(sqlc.narg('status'), status)
ORDER BY created_at DESC;