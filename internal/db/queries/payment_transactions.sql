-- name: CreatePaymentTransaction :one
INSERT INTO payment_transactions (user_id,
                                  amount,
                                  transaction_type,
                                  provider,
                                  provider_transaction_id,
                                  status,
                                  metadata)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING *;

-- name: GetPaymentTransactionByProviderID :one
SELECT *
FROM payment_transactions
WHERE provider_transaction_id = $1
  AND provider = $2
  AND user_id = $3;

-- name: UpdatePaymentTransactionStatus :exec
UPDATE payment_transactions
SET status     = $1,
    updated_at = NOW()
WHERE provider_transaction_id = $2
  AND provider = $3
  AND user_id = $4;