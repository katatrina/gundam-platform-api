-- name: CreateWalletEntry :one
INSERT INTO wallet_entries (wallet_id,
                            reference_id,
                            reference_type,
                            entry_type,
                            amount,
                            status)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;
