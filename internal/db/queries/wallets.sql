-- name: CreateWallet :exec
INSERT INTO wallets (user_id)
VALUES ($1);
